package postprocess

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeFLAC writes the smallest file the FLAC parser accepts: magic, a
// STREAMINFO block, a frame sync and padding.
func fakeFLAC(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	hdr := []byte{'f', 'L', 'a', 'C', 0x80, 0x00, 0x00, 0x22}
	body := make([]byte, 34)
	frames := []byte{0xFF, 0xF8, 0x69, 0x18, 0x00, 0x00}
	buf := append(append(append(hdr, body...), frames...), make([]byte, 4096)...)
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func artifactFor(path, ext string) *Artifact {
	return &Artifact{
		Path:           path,
		Ext:            ext,
		Title:          "DND",
		Artist:         "Ado",
		AlbumArtist:    "Ado",
		Album:          "DND",
		Track:          1,
		TrackTotal:     12,
		Disc:           1,
		DiscTotal:      2,
		Year:           "2026-11-04",
		Genre:          "J-Pop",
		ISRC:           "JPPO02601234",
		Label:          "Virgin Music",
		ReleaseType:    "album",
		ReleaseGroupID: "cbc7a5a2-3298-4f59-bddc-16e4a2d36311",
		MBTrackID:      "662ec667-2102-41a2-a9e0-3b46da7af447",
		MBArtistID:     "e134b52f-2e9e-4734-9bc3-bea9648d1fa1",
		MBAlbumID:      "99cc17f3-3d4e-4cf7-9569-1a84ac1f9658",
	}
}
