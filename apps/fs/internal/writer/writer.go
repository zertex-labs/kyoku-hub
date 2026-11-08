// Package writer performs the job the hub cannot: putting bytes on the user's
// disk and writing the tags that describe them.
//
// The hub decides everything it is good at — which album, which edition, which
// recording ids, what the tags should say — and hands over a manifest. The agent
// fetches, verifies, tags and publishes, then says what happened. Nothing here
// knows what a provider is.
package writer

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/tagwriter"
	"golang.org/x/crypto/blowfish"

	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

// Emit is how progress leaves the agent. The hub turns these into job log lines
// and progress, so the shape has to be enough for both.
type Emit func(*fspb.WriteEvent)

// Executor runs write jobs one at a time.
//
// One at a time, deliberately: these are large sequential writes to the user's
// disk and the provider CDNs are not the bottleneck. Running albums in parallel
// would only make the progress meaningless.
type Executor struct {
	// Root is the agent's library root: manifest paths resolve against it.
	Root string

	// Client is the HTTP client used for provider downloads. Short-lived signed
	// URLs mean no connection reuse across tracks is worth forcing.
	Client *http.Client
}

func New(root string) *Executor {
	return &Executor{Root: root, Client: &http.Client{Timeout: 15 * time.Minute}}
}

// Run executes a manifest. It reports progress through emit and returns the
// first error that stopped it — after emitting a "failed" event, so the hub does
// not have to infer failure from silence.
func (e *Executor) Run(ctx context.Context, album *fspb.WriteAlbum, emit Emit) error {
	if album.GetJobId() == "" {
		return errors.New("manifest has no job id")
	}

	total := int32(len(album.GetTracks()))
	phase := func(p string) {
		emit(&fspb.WriteEvent{JobId: album.GetJobId(), Phase: p, Total: total})
	}
	logf := func(level, msg string) {
		emit(&fspb.WriteEvent{JobId: album.GetJobId(), Message: msg, Level: level, Total: total})
	}

	// Paths in a manifest are relative to this machine's library root, because
	// the hub has no idea what that root is called here.
	libraryDir := filepath.Join(e.Root, filepath.FromSlash(album.GetLibraryRel()))
	workDir := libraryDir
	if album.GetStagingRel() != "" && album.GetKind() == "download" {
		workDir = filepath.Join(e.Root, filepath.FromSlash(album.GetStagingRel()))
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return e.fail(album, emit, fmt.Errorf("cannot create %s: %w", workDir, err))
	}
	if album.GetWriteCoverFile() && len(album.GetCover()) > 0 {
		if err := tagwriter.WriteCoverFile(workDir, album.GetCover()); err != nil {
			logf("warn", "could not write cover.jpg: "+err.Error())
		}
	}

	for i, tr := range album.GetTracks() {
		if err := ctx.Err(); err != nil {
			return err
		}
		dest := filepath.Join(workDir, filepath.FromSlash(tr.GetDestRel()))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return e.fail(album, emit, err)
		}

		if album.GetKind() == "download" {
			phase("downloading")
			if err := e.fetch(ctx, tr, dest); err != nil {
				return e.failTrack(album, emit, tr, err)
			}
		}

		phase("tagging")
		tags := make([]tagwriter.Tag, 0, len(tr.GetTags()))
		for _, t := range tr.GetTags() {
			tags = append(tags, tagwriter.Tag{Key: t.GetKey(), Values: t.GetValues()})
		}
		if len(tags) > 0 {
			if err := tagwriter.WriteTags(dest, tags); err != nil {
				return e.failTrack(album, emit, tr, err)
			}
		}
		if album.GetEmbedCover() && len(album.GetCover()) > 0 {
			if err := tagwriter.WriteCover(dest, album.GetCover()); err != nil {
				// Artwork is worth a warning, never a failed album.
				logf("warn", filepath.Base(dest)+": cover skipped — "+err.Error())
			}
		}

		emit(&fspb.WriteEvent{
			JobId: album.GetJobId(), Phase: "tagging",
			Done: int32(i + 1), Total: total,
			Message: fmt.Sprintf("t%02d done · %s", tr.GetTrack(), filepath.Base(dest)),
		})
	}

	// Publish: move the finished album into the library in one step.
	if workDir != libraryDir {
		phase("publishing")
		if err := publish(workDir, libraryDir); err != nil {
			return e.fail(album, emit, err)
		}
	}

	emit(&fspb.WriteEvent{
		JobId:  album.GetJobId(),
		Phase:  "done",
		Done:   total,
		Total:  total,
		Result: fmt.Sprintf("%d %s", total, plural(total)),
		Dir:    libraryDir,
	})
	return nil
}

// fetch streams one track to dest, checking what arrived against what the hub
// promised. A truncated file that still parses is the worst outcome: it gets
// tagged and looks complete forever.
func (e *Executor) fetch(ctx context.Context, tr *fspb.WriteTrack, dest string) error {
	if tr.GetUrl() == "" {
		return errors.New("no source url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tr.GetUrl(), nil)
	if err != nil {
		return err
	}
	for k, v := range tr.GetHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch: status %d", resp.StatusCode)
	}

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}

	if want := tr.GetExpectedBytes(); want > 0 && n != want {
		_ = os.Remove(tmp)
		return fmt.Errorf("fetched %d bytes, expected %d", n, want)
	}
	if tf := tr.GetTransform(); tf != nil {
		if err := applyTransform(tmp, tf); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}

	// A file that does not start like audio is a signed URL that expired and
	// answered with a page, which otherwise gets tagged happily.
	if err := looksLikeAudio(tmp, tr.GetExpectedExt()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// applyTransform unscrambles the file in place, chunk by chunk, so a 400 MB
// album never needs a second copy on disk.
func applyTransform(path string, tf *fspb.Transform) error {
	if tf.GetKind() != "stripe-cbc" {
		return fmt.Errorf("unknown stream transform %q", tf.GetKind())
	}
	chunk := int(tf.GetChunk())
	every := int(tf.GetEvery())
	if chunk <= 0 || every <= 0 {
		return fmt.Errorf("stream transform has no geometry")
	}

	block, err := blowfish.NewCipher(tf.GetKey())
	if err != nil {
		return fmt.Errorf("stream transform key: %w", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, chunk)
	for i := 0; ; i++ {
		n, readErr := io.ReadFull(f, buf)
		if n == 0 {
			break
		}
		if i%every == 0 && n == chunk {
			dec := cipher.NewCBCDecrypter(block, tf.GetIv())
			dec.CryptBlocks(buf[:n], buf[:n])
			if _, err := f.WriteAt(buf[:n], int64(i*chunk)); err != nil {
				return err
			}
		}
		if readErr != nil {
			break
		}
	}
	return nil
}

// looksLikeAudio checks the container magic rather than trusting the extension.
func looksLikeAudio(path, ext string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("file is %d bytes", len(head))
	}
	switch strings.ToLower(ext) {
	case ".flac":
		if string(head) != "fLaC" {
			return errors.New("the download is not a FLAC file")
		}
	case ".mp3":
		if string(head[:3]) != "ID3" && !(head[0] == 0xFF && head[1]&0xE0 == 0xE0) {
			return errors.New("the download is not an MP3 file")
		}
	}
	return nil
}

// publish moves the finished folder into the library, replacing an older copy.
func publish(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	// Remove the destination first: rename onto a non-empty directory fails, and
	// a half-replaced album is worse than a brief gap.
	if err := os.RemoveAll(to); err != nil {
		return err
	}
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	// Different filesystems: copy, then drop the staging copy.
	if err := copyTree(from, to); err != nil {
		return err
	}
	return os.RemoveAll(from)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

func (e *Executor) fail(album *fspb.WriteAlbum, emit Emit, err error) error {
	emit(&fspb.WriteEvent{JobId: album.GetJobId(), Phase: "failed", Error: err.Error()})
	return err
}

func (e *Executor) failTrack(album *fspb.WriteAlbum, emit Emit, tr *fspb.WriteTrack, err error) error {
	return e.fail(album, emit, fmt.Errorf("track %d: %w", tr.GetTrack(), err))
}

func plural(n int32) string {
	if n == 1 {
		return "file"
	}
	return "files"
}
