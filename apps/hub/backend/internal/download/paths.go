package download

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Disk layout — pinned in docs/downloader-plan.md §7:
//
//	<root>/<albumartist>/<album> (<year>)/<NN> - <title>.<ext>
//	                                          /cover.jpg
//
// Disc prefix only when the release has more than one disc ("1-01 Title.flac"):
// the two-digit form is what every other tool in this space expects, and adding
// a disc number to single-disc albums breaks that for no gain.

// AlnumFold is the comparison form used for matching: lowercased, punctuation
// and spacing collapsed. Kept next to Sanitize because both exist to make
// provider strings comparable.
func alnumFold(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case r == '\'' || r == '’' || r == '`':
			continue
		default:
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Sanitize makes a provider string safe as one path element.
func Sanitize(s string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", " -", "*", "_", "?", "_",
		`"`, "'", "<", "_", ">", "_", "|", "_", "\x00", "",
	)
	out := strings.TrimSpace(r.Replace(s))
	out = strings.Trim(out, ". ")
	// Newlines and tabs would break the tree in surprising ways.
	out = strings.Join(strings.Fields(out), " ")
	if out == "" {
		out = "Unknown"
	}
	if len(out) > 120 {
		out = strings.TrimSpace(out[:120])
	}
	return out
}

// AlbumDir is the publish directory for a release, relative to the library root.
func AlbumDir(albumArtist, album string, year int) string {
	artist := Sanitize(albumArtist)
	if artist == "" {
		artist = "Unknown Artist"
	}
	name := Sanitize(album)
	if name == "" {
		name = "Unknown Album"
	}
	if year > 0 {
		name = fmt.Sprintf("%s (%d)", name, year)
	}
	return filepath.Join(artist, name)
}

// TrackFileName is "NN - Title.ext", or "D-NN - Title.ext" when disc > 1.
func TrackFileName(disc, track int, title, ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	name := Sanitize(title)
	if name == "" {
		name = "Untitled"
	}
	if disc > 1 {
		return fmt.Sprintf("%d-%02d - %s%s", disc, track, name, ext)
	}
	return fmt.Sprintf("%02d - %s%s", track, name, ext)
}
