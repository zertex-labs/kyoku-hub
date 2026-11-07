package downloader

import "strings"

// ArtistMatches reports whether a provider's artist credit names the artist we
// asked for.
//
// Search is loose by design — Deezer matches any word in the query, so "Ado DND"
// also returns adonä, Noir and You'll Never Get To Heaven — and a picker full of
// different artists is worse than a short one. Matching is done on normalised
// text, whole words only, so "Ado" matches "Ado & Hatsune Miku" but never
// "adonä" or "Shadow".
func ArtistMatches(want, got string) bool {
	w := FoldArtist(want)
	g := FoldArtist(got)
	if w == "" || g == "" {
		return false
	}
	if w == g {
		return true
	}
	// A collaboration credit is the same artist with more people on it; either
	// side may carry the extra names.
	if containsWord(g, w) || containsWord(w, g) {
		return true
	}
	// Different spellings of one credit: "Simon & Garfunkel" vs "Simon and
	// Garfunkel" fold to the same words once punctuation is stripped.
	dropAnd := func(s string) string {
		s = " " + s + " "
		s = strings.ReplaceAll(s, " and ", " ")
		return strings.TrimSpace(s)
	}
	return dropAnd(w) == dropAnd(g)
}

// FoldArtist normalises an artist credit for comparison: lowercase, without
// parenthesised disambiguation, without a featured-artist tail, without
// punctuation, and without Latin accents.
func FoldArtist(s string) string {
	s = strings.ToLower(s)
	s = stripParenthetical(s)
	// "Ado feat. X" is Ado; the featured credit is not the album artist.
	for _, sep := range []string{" feat.", " feat ", " ft.", " ft ", " featuring "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	s = foldAccents(s)

	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSpace = false
		case r > 127:
			// Keep scripts without word separators (Japanese, Korean, Cyrillic)
			// intact: dropping them would collapse every such name to "".
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func stripParenthetical(s string) string {
	for {
		open := strings.Index(s, "(")
		if open < 0 {
			return s
		}
		close := strings.Index(s[open:], ")")
		if close < 0 {
			return s[:open]
		}
		s = s[:open] + s[open+close+1:]
	}
}

// accentFold maps the Latin-1 accents that show up in artist names onto ASCII,
// so "Björk" and "Bjork" are one artist. A map, not two parallel strings: the
// strings were one rune apart and silently folded nothing at all.
var accentFold = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'ç': 'c', 'ć': 'c', 'č': 'c',
	'ď': 'd',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ė': 'e', 'ę': 'e', 'ě': 'e',
	'ğ': 'g',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i', 'į': 'i',
	'ñ': 'n', 'ń': 'n', 'ň': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ő': 'o',
	'ř': 'r', 'ś': 's', 'š': 's', 'ş': 's', 'ť': 't',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ū': 'u', 'ů': 'u', 'ű': 'u',
	'ý': 'y', 'ÿ': 'y', 'ż': 'z', 'ź': 'z', 'ž': 'z',
}

func foldAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if f, ok := accentFold[r]; ok {
			b.WriteRune(f)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func containsWord(haystack, needle string) bool {
	if needle == "" || haystack == "" {
		return false
	}
	for _, token := range strings.Split(haystack, " & ") {
		if strings.TrimSpace(token) == needle {
			return true
		}
	}
	return strings.HasPrefix(haystack, needle+" ") || strings.HasSuffix(haystack, " "+needle) ||
		strings.Contains(haystack, " "+needle+" ")
}

// TitleRank scores how close a provider title is to the release group's. It is
// used to order results, never to drop them: a release with a different title is
// still the right album far more often than a release by a different artist.
//
// Version markers are the exception — a remix or an instrumental is a different
// release of the same song, and it should not sit above the album just because
// the words match.
func TitleRank(want, got string) int {
	w := foldTitle(want)
	g := foldTitle(got)
	if w == "" || g == "" {
		return 0
	}

	switch {
	case w == g:
		return 100
	case strings.HasPrefix(g, w) && extraIsVersionSuffix(g[len(w):]):
		return 80 // "dnd" vs "dnd (deluxe edition)"
	case strings.Contains(g, w):
		return 60
	case strings.Contains(w, g):
		return 50
	}
	return 0
}

// versionSuffixWords mark a tail as another edition of the same release rather
// than a different title. "DND (Deluxe Edition)" is the same album; "LEMONADE -
// The 2nd Album" is a different one.
var versionSuffixWords = []string{
	"deluxe", "remaster", "edition", "version", "expanded", "anniversary",
	"bonus", "special", "explicit", "clean", "mono", "stereo",
}

// extraIsVersionSuffix reports whether the tail is a bracketed or dashed suffix
// naming another edition of the same release.
func extraIsVersionSuffix(tail string) bool {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return true
	}
	if !strings.HasPrefix(tail, "(") && !strings.HasPrefix(tail, "-") && !strings.HasPrefix(tail, "[") {
		return false
	}
	for _, w := range versionSuffixWords {
		if strings.Contains(tail, w) {
			return true
		}
	}
	return false
}

// releaseNoise are words that name a different version of the same release.
var releaseNoise = []string{
	"remix", "remixed", "instrumental", "karaoke", "sped up", "slowed",
	"nightcore", "cover", "live", "acoustic",
}

// NoisyRelease reports whether a title carries a version marker the reference
// title does not — the difference between the album and the remix single.
func NoisyRelease(want, got string) bool {
	w := foldTitle(want)
	g := foldTitle(got)
	for _, marker := range releaseNoise {
		if strings.Contains(g, marker) && !strings.Contains(w, marker) {
			return true
		}
	}
	return false
}

// foldTitle is FoldArtist without the featured-artist and parenthesis handling:
// in a title, parentheses are part of the name.
func foldTitle(s string) string {
	return strings.TrimSpace(foldAccents(strings.ToLower(s)))
}
