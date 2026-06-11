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
)

// DefaultRequiredTags is the set of tags required when no explicit
// configuration is provided.
var DefaultRequiredTags = []string{TagTitle, TagAlbum, TagArtist, TagYear, TagTrackNum}

var knownTags = map[string]struct{}{
	TagTitle:    {},
	TagAlbum:    {},
	TagArtist:   {},
	TagYear:     {},
	TagTrackNum: {},
}

// Track is the minimal tag set a validator inspects. Zero values are
// treated as "tag absent".
type Track struct {
	Title    string
	Album    string
	Artist   string
	Year     int
	TrackNum int
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
			return nil, fmt.Errorf("unknown required tag %q (valid: %s)", tag, strings.Join(DefaultRequiredTags, ", "))
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
		}
	}
	return missing
}
