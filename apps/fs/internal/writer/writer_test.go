package writer

import (
	"bytes"
	"context"
	"crypto/cipher"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/blowfish"

	"github.com/zertex-labs/kyoku-hub/packages/tagwriter"
	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

// fakeFLACBytes is a file the FLAC parser accepts: magic, STREAMINFO, a frame
// sync and padding.
func fakeFLACBytes(size int) []byte {
	var buf bytes.Buffer
	buf.WriteString("fLaC")
	buf.WriteByte(0x80)
	buf.Write([]byte{0x00, 0x00, 0x22})
	buf.Write(make([]byte, 34))
	buf.Write([]byte{0xFF, 0xF8, 0x69, 0x18, 0x00, 0x00})
	buf.Write(make([]byte, size))
	return buf.Bytes()
}

func TestRunDownloadsTagsAndPublishes(t *testing.T) {
	audio := fakeFLACBytes(2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "yes" {
			t.Errorf("headers were not forwarded: %v", r.Header)
		}
		w.Write(audio)
	}))
	defer srv.Close()

	root := t.TempDir()
	album := &fspb.WriteAlbum{
		JobId:      "job-1",
		Kind:       "download",
		StagingRel: ".staging/job-1",
		LibraryRel: "Ado/残夢",
		EmbedCover: false,
		Tracks: []*fspb.WriteTrack{{
			DestRel:       "01 - Nukegara.flac",
			Url:           srv.URL,
			Headers:       map[string]string{"X-Test": "yes"},
			Track:         1,
			ExpectedExt:   ".flac",
			ExpectedBytes: int64(len(audio)),
			Tags: []*fspb.Tag{
				{Key: "TITLE", Values: []string{"Nukegara"}},
				{Key: "ARTIST", Values: []string{"Ado"}},
				{Key: "ALBUM", Values: []string{"残夢"}},
				{Key: "TRACKNUMBER", Values: []string{"1"}},
				{Key: "MUSICBRAINZ_TRACKID", Values: []string{"3bf17fdf-1b0b-4bb1-a680-0a47da719da7"}},
				{Key: "GENRE", Values: []string{"j-pop", "pop"}},
			},
		}},
	}

	var events []*fspb.WriteEvent
	err := New(root).Run(context.Background(), album, func(e *fspb.WriteEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The file is in the library, not in staging, and staging is gone.
	libDir := filepath.Join(root, "Ado", "残夢")
	dest := filepath.Join(libDir, "01 - Nukegara.flac")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".staging", "job-1")); !os.IsNotExist(err) {
		t.Errorf("staging dir still exists")
	}

	// And it carries the tags the hub asked for.
	vals, err := tagwriter.ReadTags(dest)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if vals.Title != "Nukegara" || vals.Album != "残夢" {
		t.Errorf("tags = %+v", vals)
	}
	if vals.MBTrackID != "3bf17fdf-1b0b-4bb1-a680-0a47da719da7" {
		t.Errorf("recording id = %q", vals.MBTrackID)
	}

	// The last event is the one the hub turns into the job's result.
	last := events[len(events)-1]
	if last.GetPhase() != "done" || last.GetDir() != libDir {
		t.Fatalf("last event = %+v", last)
	}
	if last.GetDone() != 1 || last.GetTotal() != 1 {
		t.Errorf("progress = %d/%d", last.GetDone(), last.GetTotal())
	}
}

// A truncated download must fail the job instead of producing a tagged stub.
func TestRunRejectsShortFiles(t *testing.T) {
	audio := fakeFLACBytes(64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(audio[:len(audio)-10])
	}))
	defer srv.Close()

	root := t.TempDir()
	album := &fspb.WriteAlbum{
		JobId:      "job-2",
		Kind:       "download",
		StagingRel: "staging",
		LibraryRel: "lib",
		Tracks: []*fspb.WriteTrack{{
			DestRel:       "01.flac",
			Url:           srv.URL,
			Track:         1,
			ExpectedExt:   ".flac",
			ExpectedBytes: int64(len(audio)),
		}},
	}

	err := New(root).Run(context.Background(), album, func(*fspb.WriteEvent) {})
	if err == nil {
		t.Fatal("expected an error for a short file")
	}
	if _, statErr := os.Stat(filepath.Join(root, "lib", "01.flac")); !os.IsNotExist(statErr) {
		t.Error("a short file reached the library")
	}
}

// A signed URL that expired answers with HTML; that must not be tagged as audio.
func TestRunRejectsNonAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>url expired</html>"))
	}))
	defer srv.Close()

	root := t.TempDir()
	album := &fspb.WriteAlbum{
		JobId:      "job-3",
		Kind:       "download",
		StagingRel: "staging",
		LibraryRel: "lib",
		Tracks: []*fspb.WriteTrack{{
			DestRel:     "01.flac",
			Url:         srv.URL,
			Track:       1,
			ExpectedExt: ".flac",
		}},
	}
	if err := New(root).Run(context.Background(), album, func(*fspb.WriteEvent) {}); err == nil {
		t.Fatal("expected an error for a non-audio download")
	}
}

// A retag edits the files where they are; it must not move or duplicate them.
func TestRetagWritesInPlace(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(lib, "01.flac")
	if err := os.WriteFile(dest, fakeFLACBytes(128), 0o644); err != nil {
		t.Fatal(err)
	}

	album := &fspb.WriteAlbum{
		JobId:      "job-4",
		Kind:       "retag",
		LibraryRel: "lib",
		Tracks: []*fspb.WriteTrack{{
			DestRel: "01.flac",
			Track:   1,
			Tags:    []*fspb.Tag{{Key: "TITLE", Values: []string{"New Title"}}},
		}},
	}
	if err := New(root).Run(context.Background(), album, func(*fspb.WriteEvent) {}); err != nil {
		t.Fatalf("run: %v", err)
	}

	vals, err := tagwriter.ReadTags(dest)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if vals.Title != "New Title" {
		t.Errorf("title = %q", vals.Title)
	}
}

// The provider hands out encrypted bytes; the ticket says how to unscramble them.
// Round trip: scramble a real file the same way, and expect it back byte for byte.
func TestRunDecryptsTheStream(t *testing.T) {
	plain := fakeFLACBytes(2048 * 3)
	key := []byte("0123456789abcdef")
	iv := []byte{0, 1, 2, 3, 4, 5, 6, 7}

	encrypted := make([]byte, len(plain))
	copy(encrypted, plain)
	block, err := blowfish.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i*2048 < len(encrypted); i++ {
		if i%3 != 0 {
			continue
		}
		off := i * 2048
		if off+2048 > len(encrypted) {
			break
		}
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted[off:off+2048], encrypted[off:off+2048])
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(encrypted)
	}))
	defer srv.Close()

	root := t.TempDir()
	album := &fspb.WriteAlbum{
		JobId:      "job-5",
		Kind:       "download",
		StagingRel: "staging",
		LibraryRel: "lib",
		Tracks: []*fspb.WriteTrack{{
			DestRel:       "01.flac",
			Url:           srv.URL,
			Track:         1,
			ExpectedExt:   ".flac",
			ExpectedBytes: int64(len(plain)),
			Transform: &fspb.Transform{
				Kind: "stripe-cbc", Chunk: 2048, Every: 3, Key: key, Iv: iv,
			},
		}},
	}
	if err := New(root).Run(context.Background(), album, func(*fspb.WriteEvent) {}); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "lib", "01.flac"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted file differs from the original (%d vs %d bytes)", len(got), len(plain))
	}
}

var _ = binary.BigEndian
