package postprocess

import (
	"context"
	"strings"
	"testing"

	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

func comments(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*m)
		if err != nil {
			t.Fatalf("parse comments: %v", err)
		}
		out := make(map[string]string, len(cmt.Comments))
		for _, c := range cmt.Comments {
			kv := strings.SplitN(c, "=", 2)
			if len(kv) == 2 {
				out[kv[0]] = kv[1]
			}
		}
		return out
	}
	t.Fatalf("no vorbis comment block")
	return nil
}

func TestTagsWritesTheWholeSet(t *testing.T) {
	path := fakeFLAC(t, "01 - DND.flac")
	if err := (Tags{}).Process(context.Background(), artifactFor(path, ".flac")); err != nil {
		t.Fatalf("process: %v", err)
	}

	want := map[string]string{
		"TITLE":                      "DND",
		"ARTIST":                     "Ado",
		"ALBUMARTIST":                "Ado",
		"ALBUM":                      "DND",
		"TRACKNUMBER":                "1",
		"TRACKTOTAL":                 "12",
		"DISCNUMBER":                 "1",
		"DISCTOTAL":                  "2",
		"DATE":                       "2026-11-04",
		"GENRE":                      "J-Pop",
		"ISRC":                       "JPPO02601234",
		"LABEL":                      "Virgin Music",
		"RELEASETYPE":                "album",
		"MUSICBRAINZ_TRACKID":        "662ec667-2102-41a2-a9e0-3b46da7af447",
		"MUSICBRAINZ_ARTISTID":       "e134b52f-2e9e-4734-9bc3-bea9648d1fa1",
		"MUSICBRAINZ_ALBUMID":        "99cc17f3-3d4e-4cf7-9569-1a84ac1f9658",
		"MUSICBRAINZ_RELEASEGROUPID": "cbc7a5a2-3298-4f59-bddc-16e4a2d36311",
	}
	got := comments(t, path)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// Running twice (a resumed download, a retry) must leave one set of values, not
// duplicates, and must not touch comments this writer does not own.
func TestTagsAreIdempotentAndPreserveForeignComments(t *testing.T) {
	path := fakeFLAC(t, "01 - DND.flac")
	a := artifactFor(path, ".flac")
	if err := (Tags{}).Process(context.Background(), a); err != nil {
		t.Fatalf("first: %v", err)
	}

	f, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, _ := flacvorbis.ParseFromMetaDataBlock(*m)
		cmt.Comments = append(cmt.Comments, "REPLAYGAIN_TRACK_GAIN=-6.20 dB", "CUSTOM=keep me")
		block := cmt.Marshal()
		f.Meta[i] = &block
	}
	if err := f.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	a.Title = "DND (Remastered)"
	if err := (Tags{}).Process(context.Background(), a); err != nil {
		t.Fatalf("second: %v", err)
	}

	got := comments(t, path)
	if got["TITLE"] != "DND (Remastered)" {
		t.Errorf("TITLE = %q", got["TITLE"])
	}
	if got["REPLAYGAIN_TRACK_GAIN"] != "-6.20 dB" || got["CUSTOM"] != "keep me" {
		t.Errorf("foreign comments dropped: %v", got)
	}

	f, _ = flac.ParseFile(path)
	n := 0
	for _, m := range f.Meta {
		if m.Type == flac.VorbisComment {
			n++
		}
	}
	if n != 1 {
		t.Errorf("comment blocks = %d, want 1", n)
	}
}

// A format without a writer must say so instead of failing the download.
func TestTagsSkipFormatsWithoutAWriter(t *testing.T) {
	err := (Tags{}).Process(context.Background(), artifactFor(fakeFLAC(t, "01 - DND.mp3"), ".mp3"))
	if err == nil {
		t.Fatal("expected a skip")
	}
	out := Outcome{Name: "tags", Err: err}
	reason, skipped := out.Skipped()
	if !skipped {
		t.Fatalf("not reported as a skip: %v", err)
	}
	if !strings.Contains(reason, "mp3") {
		t.Errorf("skip reason = %q", reason)
	}
	if out.Failed() {
		t.Error("a skip is not a failure")
	}
}

func TestTagsHonourCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (Tags{}).Process(ctx, artifactFor(fakeFLAC(t, "a.flac"), ".flac")); err == nil {
		t.Fatal("expected ctx error")
	}
}
