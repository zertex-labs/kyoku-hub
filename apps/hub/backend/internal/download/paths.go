package download

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
)

// Disk layout — the album folder is a template the user owns:
//
//	<root>/<rendered album path>/<NN> - <title>.<ext>
//	                            /cover.jpg
//
// The default is "Artist/[Year] Album", which is what Picard produces and
// therefore what an existing library already looks like. Anything else is a
// setting, not a fork of the code: a second naming scheme in one library is what
// makes re-tagging and de-duplication miserable.
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
//
// Kept as the untemplated form so callers that only need "artist/album (year)"
// stay readable; AlbumDirFor is what the download path uses.
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

// AlbumDirFor renders an album path template:
//
//	{artist}      album artist, falling back to the track artist
//	{albumartist} the same value, for people who write it out
//	{album}       album title
//	{year}        four-digit year, empty when unknown
//
// A segment that renders empty takes its decoration with it: with no year,
// "[{year}] {album}" becomes "{album}", not "[] Album". Every segment is
// sanitised separately so a "/" in a title cannot invent a directory.
func AlbumDirFor(template, albumArtist, artist, album string, year int) string {
	if strings.TrimSpace(template) == "" {
		template = settings.DefaultAlbumPath
	}

	artistName := albumArtist
	if artistName == "" {
		artistName = artist
	}
	albumName := album
	if albumName == "" {
		albumName = "Unknown Album"
	}

	yearText := ""
	if year > 0 {
		yearText = fmt.Sprintf("%d", year)
	}

	// Substitute per segment, not over the whole path: an artist called "AC/DC"
	// must not become two directories.
	out := make([]string, 0, 3)
	for _, seg := range strings.Split(template, "/") {
		if year == 0 {
			// A missing year takes its decoration with it.
			seg = strings.ReplaceAll(seg, "[{year}]", "")
			seg = strings.ReplaceAll(seg, "({year})", "")
		}
		seg = strings.NewReplacer(
			"{artist}", artistName,
			"{albumartist}", artistName,
			"{album}", albumName,
			"{year}", yearText,
		).Replace(seg)

		seg = strings.Join(strings.Fields(seg), " ")
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		out = append(out, Sanitize(seg))
	}
	if len(out) == 0 {
		return filepath.Join("Unknown Artist", Sanitize(albumName))
	}
	return filepath.Join(out...)
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
