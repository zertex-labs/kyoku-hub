package postprocess

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	flac "github.com/go-flac/go-flac"
)

// 1x1 transparent PNG.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func pictures(t *testing.T, path string) (int, []byte) {
	t.Helper()
	f, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	n := 0
	var data []byte
	for _, m := range f.Meta {
		if m.Type == flac.Picture {
			n++
			data = m.Data
		}
	}
	return n, data
}

func TestCoverAddsAndReplaces(t *testing.T) {
	path := fakeFLAC(t, "01 - DND.flac")
	png, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	a := artifactFor(path, ".flac")
	a.Cover = png
	if err := (Cover{}).Process(context.Background(), a); err != nil {
		t.Fatalf("embed: %v", err)
	}
	n, data := pictures(t, path)
	if n != 1 || !bytes.HasSuffix(data, png) {
		t.Fatalf("pictures = %d, embedded correctly = %v", n, bytes.HasSuffix(data, png))
	}

	// A second album pass must not stack a second cover: players take the first.
	other := append(append([]byte{}, png...), 0x00)
	a.Cover = other
	if err := (Cover{}).Process(context.Background(), a); err != nil {
		t.Fatalf("re-embed: %v", err)
	}
	if n, data := pictures(t, path); n != 1 || !bytes.HasSuffix(data, other) {
		t.Fatalf("after replace: pictures = %d, stale = %v", n, !bytes.HasSuffix(data, other))
	}
}

func TestCoverSkipsWhenThereIsNoArtwork(t *testing.T) {
	path := fakeFLAC(t, "01 - DND.flac")
	err := (Cover{}).Process(context.Background(), artifactFor(path, ".flac"))
	if _, skipped := (Outcome{Name: "cover", Err: err}).Skipped(); !skipped {
		t.Fatalf("expected a skip, got %v", err)
	}
}

func TestCoverRejectsNonImages(t *testing.T) {
	path := fakeFLAC(t, "01 - DND.flac")
	a := artifactFor(path, ".flac")
	a.Cover = []byte("not an image at all")
	err := (Cover{}).Process(context.Background(), a)
	out := Outcome{Name: "cover", Err: err}
	if err == nil || !out.Failed() {
		t.Fatalf("expected a failure, got %v", err)
	}
}

func TestImageInfoReadsJPEGSize(t *testing.T) {
	// SOI, APP0, then SOF0 carrying 300x200.
	jpg := []byte{
		0xFF, 0xD8,
		0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0,
		0xFF, 0xC0, 0x00, 0x11, 0x08, 0x00, 0xC8, 0x01, 0x2C, 0x03,
	}
	if mime, w, h := ImageInfo(jpg); mime != "image/jpeg" || w != 300 || h != 200 {
		t.Fatalf("jpeg = %q %dx%d", mime, w, h)
	}

	png, _ := base64.StdEncoding.DecodeString(tinyPNG)
	if mime, w, h := ImageInfo(png); mime != "image/png" || w != 1 || h != 1 {
		t.Fatalf("png = %q %dx%d", mime, w, h)
	}

	if mime, _, _ := ImageInfo([]byte("nope")); mime != "" {
		t.Fatalf("garbage detected as %q", mime)
	}
}
