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
