package writer

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
		StagingDir: filepath.Join(root, ".staging", "job-1"),
		LibraryDir: filepath.Join(root, "Ado", "残夢"),
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
	err := New().Run(context.Background(), album, func(e *fspb.WriteEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The file is in the library, not in staging, and staging is gone.
	dest := filepath.Join(album.LibraryDir, "01 - Nukegara.flac")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if _, err := os.Stat(album.StagingDir); !os.IsNotExist(err) {
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
	if last.GetPhase() != "done" || last.GetDir() != album.LibraryDir {
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
		StagingDir: filepath.Join(root, "staging"),
		LibraryDir: filepath.Join(root, "lib"),
		Tracks: []*fspb.WriteTrack{{
			DestRel:       "01.flac",
			Url:           srv.URL,
			Track:         1,
			ExpectedExt:   ".flac",
			ExpectedBytes: int64(len(audio)),
		}},
	}

	err := New().Run(context.Background(), album, func(*fspb.WriteEvent) {})
	if err == nil {
		t.Fatal("expected an error for a short file")
	}
	if _, statErr := os.Stat(filepath.Join(album.LibraryDir, "01.flac")); !os.IsNotExist(statErr) {
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
		StagingDir: filepath.Join(root, "staging"),
		LibraryDir: filepath.Join(root, "lib"),
		Tracks: []*fspb.WriteTrack{{
			DestRel:     "01.flac",
			Url:         srv.URL,
			Track:       1,
			ExpectedExt: ".flac",
		}},
	}
	if err := New().Run(context.Background(), album, func(*fspb.WriteEvent) {}); err == nil {
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
		LibraryDir: lib,
		Tracks: []*fspb.WriteTrack{{
			DestRel: "01.flac",
			Track:   1,
			Tags:    []*fspb.Tag{{Key: "TITLE", Values: []string{"New Title"}}},
		}},
	}
	if err := New().Run(context.Background(), album, func(*fspb.WriteEvent) {}); err != nil {
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

var _ = binary.BigEndian
