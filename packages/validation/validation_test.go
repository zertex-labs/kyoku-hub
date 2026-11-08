package validation

import (
	"reflect"
	"testing"
)

func TestParseRequiredTags(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{"empty returns defaults", "", DefaultRequiredTags, false},
		{"whitespace returns defaults", "  ", DefaultRequiredTags, false},
		{"single tag", "title", []string{"title"}, false},
		{"multiple with spaces", " title, album ,artist", []string{"title", "album", "artist"}, false},
		{"uppercase normalized", "TITLE,Album", []string{"title", "album"}, false},
		{"dedup", "title,title,album", []string{"title", "album"}, false},
		{"trailing comma ignored", "title,album,", []string{"title", "album"}, false},
		{"unknown tag errors", "title,bogus", nil, true},
		{"only commas returns defaults", ",,,", DefaultRequiredTags, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRequiredTags(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRequiredTags(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRequiredTags(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMissing(t *testing.T) {
	full := Track{Title: "T", Album: "A", Artist: "X", Year: 2024, TrackNum: 1}

	tests := []struct {
		name     string
		track    Track
		required []string
		want     []string
	}{
		{"complete track", full, DefaultRequiredTags, nil},
		{"empty track", Track{}, DefaultRequiredTags, DefaultRequiredTags},
		{"missing year only", Track{Title: "T", Album: "A", Artist: "X", TrackNum: 1}, DefaultRequiredTags, []string{"year"}},
		{"subset required ignores others", Track{Title: "T"}, []string{"title"}, nil},
		{"order follows required list", Track{}, []string{"year", "title"}, []string{"year", "title"}},
		{"no required tags", Track{}, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Missing(tt.track, tt.required)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Missing(%+v, %v) = %v, want %v", tt.track, tt.required, got, tt.want)
			}
		})
	}
}

// An album tagged by another tool has the core five and no identifiers: it must
// stay valid unless the user asked for the ids, and it must say which are absent
// when they did.
func TestMusicBrainzTagsOnlyCountWhenRequired(t *testing.T) {
	track := Track{Title: "Nukegara", Album: "残夢", Artist: "Ado", Year: 2024, TrackNum: 1}

	if missing := Missing(track, DefaultRequiredTags); len(missing) != 0 {
		t.Fatalf("unidentified file reported missing %v by default", missing)
	}

	missing := Missing(track, MusicBrainzTags)
	if len(missing) != len(MusicBrainzTags) {
		t.Fatalf("missing = %v, want all %d ids", missing, len(MusicBrainzTags))
	}
	for _, want := range []string{TagMBTrackID, TagMBAlbumID, TagMBArtistID, TagMBReleaseGroupID} {
		found := false
		for _, got := range missing {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q not reported missing (got %v)", want, missing)
		}
	}
}

func TestIdentifiedFileHasNothingMissing(t *testing.T) {
	track := Track{
		Title: "Nukegara", Album: "残夢", Artist: "Ado", Year: 2024, TrackNum: 1,
		MBTrackID:        "3bf17fdf-1b0b-4bb1-a680-0a47da719da7",
		MBAlbumID:        "c3bcc159-20da-4e1e-bb9e-53f63dc32280",
		MBArtistID:       "e134b52f-2e9e-4734-9bc3-bea9648d1fa1",
		MBReleaseGroupID: "2f287d56-6736-4091-9b74-8fea69b5def4",
		ISRC:             "JPPO02402979",
	}
	if missing := Missing(track, append(DefaultRequiredTags, MusicBrainzTags...)); len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
}

func TestParseRequiredTagsAcceptsIds(t *testing.T) {
	got, err := ParseRequiredTags("title,musicbrainz_trackid,isrc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}
