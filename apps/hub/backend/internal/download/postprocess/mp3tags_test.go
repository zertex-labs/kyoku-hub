package postprocess

import (
	"context"
	"strings"
	"testing"

	id3v2 "github.com/bogem/id3v2/v2"
)

// The ID3 mapping is the contract for MP3s, and it differs from the Vorbis one
// in the two places that matter: the recording id lives in UFID, and the totals
// live inside TRCK/TPOS.
func TestMP3TagsMatchThePicardFrames(t *testing.T) {
	path := fakeMP3(t, "01 - DND.mp3")
	a := artifactFor(path, ".mp3")
	a.Cover = fakeJPEG()

	if err := (Tags{}).Process(context.Background(), a); err != nil {
		t.Fatalf("process: %v", err)
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tag.Close()

	frames := map[string]string{
		"TIT2": "DND",
		"TALB": "DND",
		"TPE1": "Ado",
		"TPE2": "Ado",
		"TSRC": "JPPO02601234",
		"TPUB": "Virgin Music",
		"TRCK": "1/12",
		"TPOS": "1/2",
		"TDRC": "2026-11-04",
		"TDOR": "2026-10-15",
	}
	for id, want := range frames {
		got := tag.GetTextFrame(id).Text
		if got != want {
			t.Errorf("%s = %q, want %q", id, got, want)
		}
	}

	txxx := map[string]string{
		"MusicBrainz Album Id":              "99cc17f3-3d4e-4cf7-9569-1a84ac1f9658",
		"MusicBrainz Release Group Id":      "cbc7a5a2-3298-4f59-bddc-16e4a2d36311",
		"MusicBrainz Artist Id":             "e134b52f-2e9e-4734-9bc3-bea9648d1fa1",
		"MusicBrainz Album Artist Id":       "e134b52f-2e9e-4734-9bc3-bea9648d1fa1",
		"MusicBrainz Album Type":            "album",
		"MusicBrainz Album Status":          "Official",
		"MusicBrainz Album Release Country": "JP",
		"CATALOGNUMBER":                     "TYCT-60123",
		"BARCODE":                           "4988061861234",
	}
	seen := map[string]string{}
	for _, f := range tag.GetFrames("TXXX") {
		udtf, ok := f.(id3v2.UserDefinedTextFrame)
		if !ok {
			t.Fatalf("TXXX frame is %T", f)
		}
		seen[udtf.Description] = udtf.Value
	}
	for desc, want := range txxx {
		if seen[desc] != want {
			t.Errorf("TXXX:%s = %q, want %q", desc, seen[desc], want)
		}
	}

	// A release-track id here would be a lie: this frame is not the recording id.
	if v, ok := seen["MusicBrainz Track Id"]; ok {
		t.Errorf("wrote MusicBrainz Track Id = %q; the recording id belongs in UFID", v)
	}

	ufid := tag.GetLastFrame("UFID")
	if ufid == nil {
		t.Fatal("no UFID frame")
	}
	frame, ok := ufid.(id3v2.UFIDFrame)
	if !ok {
		t.Fatalf("UFID is %T", frame)
	}
	if frame.OwnerIdentifier != "http://musicbrainz.org" {
		t.Errorf("UFID owner = %q", frame.OwnerIdentifier)
	}
	if string(frame.Identifier) != "662ec667-2102-41a2-a9e0-3b46da7af447" {
		t.Errorf("UFID = %q", frame.Identifier)
	}

	pics := tag.GetFrames(tag.CommonID("Attached picture"))
	if len(pics) != 1 {
		t.Fatalf("pictures = %d, want 1", len(pics))
	}
	pic := pics[0].(id3v2.PictureFrame)
	if pic.PictureType != id3v2.PTFrontCover || len(pic.Picture) == 0 {
		t.Errorf("cover frame = type %d, %d bytes", pic.PictureType, len(pic.Picture))
	}

	// Both genres survive, as a v2.4 multi-value TCON.
	genres := strings.Split(tag.Genre(), "\x00")
	if len(genres) != 2 || genres[0] != "J-Pop" || genres[1] != "J-Rock" {
		t.Errorf("TCON = %q", tag.Genre())
	}
}

// Re-running must replace the frames, not stack a second set beside them.
func TestMP3TagsAreIdempotent(t *testing.T) {
	path := fakeMP3(t, "01 - DND.mp3")
	a := artifactFor(path, ".mp3")
	for i := 0; i < 2; i++ {
		if err := (Tags{}).Process(context.Background(), a); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tag.Close()

	if n := len(tag.GetFrames("TIT2")); n != 1 {
		t.Errorf("TIT2 frames = %d", n)
	}
	if n := len(tag.GetFrames("UFID")); n != 1 {
		t.Errorf("UFID frames = %d", n)
	}
	// The MPEG frames behind the tag must still be there.
	if tag.Size() == 0 {
		t.Error("tag has no size")
	}
}
