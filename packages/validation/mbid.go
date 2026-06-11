package validation

import "regexp"

// MBIDs are UUIDs, canonically lowercase in MusicBrainz.
var mbidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsMBID reports whether s is a well-formed MusicBrainz identifier.
func IsMBID(s string) bool {
	return mbidRegex.MatchString(s)
}
