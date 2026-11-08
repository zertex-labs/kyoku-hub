// Package validation contains shared tag validation logic
// services. It checks file metadata (reported by fs agents) against a
// configurable list of required tags.
package validation

import (
	"fmt"
	"strings"
)

// Tag names accepted in a required-tags configuration.
const (
	TagTitle    = "title"
	TagAlbum    = "album"
	TagArtist   = "artist"
	TagYear     = "year"
	TagTrackNum = "track_num"

	// The MusicBrainz identifiers, named as Picard names them. A file without
	// them still plays, but nothing else can recognise it: matching falls back to
	// text, which is what fails across scripts and licensing renames.
	TagMBTrackID        = "musicbrainz_trackid"
	TagMBAlbumID        = "musicbrainz_albumid"
	TagMBArtistID       = "musicbrainz_artistid"
	TagMBReleaseGroupID = "musicbrainz_releasegroupid"
	TagISRC             = "isrc"
)

// MusicBrainzTags is what a fully identified file carries. Requiring it is a
// choice, not a default: most libraries are tagged by tools that write the ids
// and many that do not.
var MusicBrainzTags = []string{TagMBArtistID, TagMBReleaseGroupID, TagMBAlbumID, TagMBTrackID}

// DefaultRequiredTags is the set of tags required when no explicit
// configuration is provided.
var DefaultRequiredTags = []string{TagTitle, TagAlbum, TagArtist, TagYear, TagTrackNum}

var knownTags = map[string]struct{}{
	TagTitle:    {},
	TagAlbum:    {},
	TagArtist:   {},
	TagYear:     {},
	TagTrackNum: {},

	TagMBTrackID:        {},
	TagMBAlbumID:        {},
	TagMBArtistID:       {},
	TagMBReleaseGroupID: {},
	TagISRC:             {},
}

// Track is the minimal tag set a validator inspects. Zero values are
// treated as "tag absent".
type Track struct {
	Title    string
	Album    string
	Artist   string
	Year     int
	TrackNum int

	// MusicBrainz identifiers and ISRC, empty when the file has none.
	MBTrackID        string
	MBAlbumID        string
	MBArtistID       string
	MBReleaseGroupID string
	ISRC             string
}

// ParseRequiredTags parses a comma-separated list of tag names (e.g. from an
// environment variable). An empty input returns DefaultRequiredTags. Unknown
// tag names produce an error so misconfiguration fails fast.
func ParseRequiredTags(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultRequiredTags, nil
	}

	parts := strings.Split(s, ",")
	tags := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		tag := strings.ToLower(strings.TrimSpace(p))
		if tag == "" {
			continue
		}
		if _, ok := knownTags[tag]; !ok {
			return nil, fmt.Errorf("unknown required tag %q (valid: %s)", tag, strings.Join(allTagNames(), ", "))
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return DefaultRequiredTags, nil
	}
	return tags, nil
}

// allTagNames lists every accepted tag, for error messages.
func allTagNames() []string {
	names := append([]string{}, DefaultRequiredTags...)
	names = append(names, MusicBrainzTags...)
	return append(names, TagISRC)
}

// Missing returns the names of required tags absent from t, in the order
// they appear in required.
func Missing(t Track, required []string) []string {
	var missing []string
	for _, tag := range required {
		switch tag {
		case TagTitle:
			if t.Title == "" {
				missing = append(missing, tag)
			}
		case TagAlbum:
			if t.Album == "" {
				missing = append(missing, tag)
			}
		case TagArtist:
			if t.Artist == "" {
				missing = append(missing, tag)
			}
		case TagYear:
			if t.Year == 0 {
				missing = append(missing, tag)
			}
		case TagTrackNum:
			if t.TrackNum == 0 {
				missing = append(missing, tag)
			}
		case TagMBTrackID:
			if t.MBTrackID == "" {
				missing = append(missing, tag)
			}
		case TagMBAlbumID:
			if t.MBAlbumID == "" {
				missing = append(missing, tag)
			}
		case TagMBArtistID:
			if t.MBArtistID == "" {
				missing = append(missing, tag)
			}
		case TagMBReleaseGroupID:
			if t.MBReleaseGroupID == "" {
				missing = append(missing, tag)
			}
		case TagISRC:
			if t.ISRC == "" {
				missing = append(missing, tag)
			}
		}
	}
	return missing
}
