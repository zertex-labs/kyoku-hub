package downloader

import (
	"fmt"
	"strings"
)

// Verdict is the classification of one provider album against the user's ladder.
//
//	Fit      every track present, every track at/above the top target, identity
//	         matches -> download silently, no prompt.
//	Partial  present and usable, but at least one track is below the top target
//	         (or identity is merely plausible) -> kept as a candidate for the
//	         results view; the walk continues.
//	Miss     identity wrong, or nothing on the album can reach any acceptable
//	         rung -> skipped, reason logged, never offered.
type Verdict string

const (
	VerdictFit     Verdict = "fit"
	VerdictPartial Verdict = "partial"
	VerdictMiss    Verdict = "miss"
)

// TrackQuality is one probed track.
type TrackQuality struct {
	// ProviderID is the source's own track id — required to download this track
	// after the user has picked the album.
	ProviderID string
	Number     int
	Disc       int
	Title      string
	Quality    AudioQuality
	// Known is false when the probe failed (network, geo, provider error). An
	// unknown track is NOT a bad track: it must not drag the album floor down,
	// it just cannot be claimed as good either.
	Known      bool
	ReasonCode string // provider error code when Known is false, or the cap reason

	// ISRC is the recording's ISRC when the probe exposed it. It is the only
	// per-track identifier a provider gives us for free, and the one tag that
	// makes a file findable by any other tool later.
	ISRC string
}

// AlbumRef is what we expect the release to be — normally filled from the
// MusicBrainz release group.
type AlbumRef struct {
	Title      string
	Artist     string
	Year       int
	TrackCount int
}

// AlbumCandidate is one provider's answer for one release group: the probed
// tracks, the derived floor, and how well the identity matches.
type AlbumCandidate struct {
	Provider   string
	ProviderID string
	Format     string // provider record type: album | ep | single, when known

	Title  string
	Artist string
	Year   int
	// ReleaseDate is the provider's full date (YYYY-MM-DD) when it has one.
	// DATE tag preference is: MusicBrainz year, then this, then the caller's.
	ReleaseDate string
	Genre       string
	Label       string

	// Ceiling is the best this source can ever deliver for this album on this
	// account (Deezer: FLAC 16/44.1, or 320 when the ARL cannot stream lossless).
	// Classification is judged against the best ladder rung the ceiling reaches,
	// not against rung 0: otherwise Deezer can never be a FIT and every download
	// asks the user a question it already knows the answer to.
	Ceiling AudioQuality

	Tracks     []TrackQuality
	OkTracks   int          // tracks at/above the effective target
	Floor      AudioQuality // weakest KNOWN track; UnknownQuality when nothing probed
	Unknown    int          // tracks whose probe failed
	Confidence float64      // 0..1 identity confidence

	// ProbeError is the first per-track probe failure, verbatim. Without it a
	// "could not probe any track" miss is undebuggable: an expired ARL, a quota
	// block and a geo restriction all look identical from the outside.
	ProbeError string

	IdentityNotes []string
}

// Classify returns the verdict for c against ref and targets, plus the human
// lines that explain a Partial/Miss (they become the deltas in the UI).
func (c AlbumCandidate) Classify(ref AlbumRef, targets []QualityTarget) (Verdict, []string) {
	var reasons []string

	// 1. Identity. Wrong master at perfect quality is worse than the right
	// master at 320: it is silently wrong, and the user cannot tell.
	idOK, idReasons := identityMatch(c, ref)
	reasons = append(reasons, idReasons...)
	if !idOK {
		return VerdictMiss, reasons
	}

	// 2. Nothing probed at all: not a candidate, just a failure.
	if len(c.Tracks) == 0 {
		return VerdictMiss, append(reasons, "no tracks returned")
	}

	// 3. Quality, judged against the best rung this source can reach.
	top := EffectiveTarget(c.Ceiling, targets)
	okCount := 0
	var belowTarget []TrackQuality
	for _, t := range c.Tracks {
		if !t.Known {
			continue
		}
		if t.Quality.Meets(top) {
			okCount++
			continue
		}
		belowTarget = append(belowTarget, t)
	}
	known := len(c.Tracks) - c.Unknown
	if known == 0 {
		msg := "could not probe any track"
		if c.ProbeError != "" {
			msg += ": " + c.ProbeError
		}
		return VerdictMiss, append(reasons, msg)
	}

	if okCount == known && c.Unknown == 0 {
		return VerdictFit, nil
	}

	// Partial: report what fell short, per track, so the user can judge.
	for _, t := range belowTarget {
		reasons = append(reasons, fmt.Sprintf("t%02d %q → %s instead of %s",
			t.Number, t.Title, t.Quality.ShortLabel(), top.String()))
	}
	if c.Unknown > 0 {
		reasons = append(reasons, fmt.Sprintf("%d track(s) could not be probed — verified at transfer", c.Unknown))
	}
	if !c.Floor.Meets(top) && len(belowTarget) == 0 {
		reasons = append(reasons, fmt.Sprintf("album floor %s is below %s", c.Floor.ShortLabel(), top.String()))
	}
	return VerdictPartial, reasons
}

// Summary is the headline used in the UI. Completeness leads, quality follows:
// "14/14 songs · FLAC 16/44.1", "14/14 songs · floor MP3 320", "1/1 songs · MP3 320".
// Never "0/1 FLAC" — on a single that reads as a contradiction.
func (c AlbumCandidate) Summary(targets []QualityTarget) string {
	total := len(c.Tracks)
	if total == 0 {
		return "no tracks"
	}
	present := 0
	for _, t := range c.Tracks {
		if t.Quality.Known() || t.Known {
			present++
		}
	}
	q := c.Floor
	if !q.Known() {
		// Nothing probed: say so rather than inventing a number.
		return fmt.Sprintf("%d/%d songs · unprobed", present, total)
	}
	if q.Meets(EffectiveTarget(c.Ceiling, targets)) {
		return fmt.Sprintf("%d/%d songs · %s", present, total, q.ShortLabel())
	}
	if total == 1 {
		// "floor" on a one-track release is noise: the floor is the only track.
		return fmt.Sprintf("%d/%d songs · %s", present, total, q.ShortLabel())
	}
	return fmt.Sprintf("%d/%d songs · floor %s", present, total, q.ShortLabel())
}

// ApplyFloor recomputes OkTracks/Floor/Unknown from Tracks against the top
// target. Callers build the track list, then call this once.
func (c *AlbumCandidate) ApplyFloor(targets []QualityTarget) {
	ok := 0
	unknown := 0
	target := EffectiveTarget(c.Ceiling, targets)
	floor := UnknownQuality
	for _, t := range c.Tracks {
		if !t.Known || !t.Quality.Known() {
			unknown++
			continue
		}
		if floor == UnknownQuality {
			floor = t.Quality
		} else if t.Quality.TierScore() < floor.TierScore() {
			floor = t.Quality
		}
		if t.Quality.Meets(target) {
			ok++
		}
	}
	c.OkTracks, c.Unknown, c.Floor = ok, unknown, floor
}

// EffectiveTarget is the rung classification actually measures against: the
// highest rung the source's ceiling can satisfy.
//
// The ladder is a preference order, not a set of requirements. A Deezer album at
// FLAC 16/44.1 satisfies rung 3 and is the best Deezer can ever do, so it is a
// FIT — it should download without asking. Judging against rung 0 would make
// every Deezer album PARTIAL and turn the results view into a toll booth.
func EffectiveTarget(ceiling AudioQuality, targets []QualityTarget) QualityTarget {
	if len(targets) == 0 {
		return QualityTarget{}
	}
	if ceiling.Known() {
		for _, t := range targets {
			if ceiling.Meets(t) {
				return t
			}
		}
	}
	// Ceiling unknown: be strict. Falling back to the lowest rung would let an
	// unclassified album be "fit" and download silently at 320 — the one failure
	// mode the whole FIT/PARTIAL split exists to prevent.
	return targets[0]
}

// identityMatch decides whether c is the release we asked for.
//
// Deliberately conservative: exact track count, normalized title, artist
// containment, year within a year. A loose threshold turns "asks the user" into
// "silently downloads the wrong master", which is the failure nobody notices.
func identityMatch(c AlbumCandidate, ref AlbumRef) (bool, []string) {
	var notes []string
	ok := true

	if ref.TrackCount > 0 && len(c.Tracks) != ref.TrackCount {
		ok = false
		notes = append(notes, fmt.Sprintf("track count %d vs release group %d", len(c.Tracks), ref.TrackCount))
	}

	if ref.Title != "" && normalizeName(c.Title) != normalizeName(ref.Title) {
		// Not fatal by itself: providers append "(Remastered)", "(Deluxe)" and
		// different punctuation. The user should still see it.
		if !strings.Contains(normalizeName(c.Title), normalizeName(ref.Title)) {
			ok = false
			notes = append(notes, fmt.Sprintf("title %q does not match %q", c.Title, ref.Title))
		} else {
			notes = append(notes, fmt.Sprintf("title %q vs %q", c.Title, ref.Title))
		}
	}

	if ref.Artist != "" && c.Artist != "" {
		a, b := normalizeName(c.Artist), normalizeName(ref.Artist)
		if !strings.Contains(a, b) && !strings.Contains(b, a) {
			ok = false
			notes = append(notes, fmt.Sprintf("artist %q vs %q", c.Artist, ref.Artist))
		}
	}

	if ref.Year > 0 && c.Year > 0 {
		diff := ref.Year - c.Year
		if diff < 0 {
			diff = -diff
		}
		if diff > 1 {
			ok = false
			notes = append(notes, fmt.Sprintf("year %d vs %d — likely a different master", c.Year, ref.Year))
		}
	}

	if ok {
		return true, nil
	}
	return false, notes
}

// normalizeName lowercases and strips everything that differs between
// providers without changing meaning: punctuation, brackets, spacing.
func normalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSpace = false
		case r == ' ':
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			// apostrophes vanish ("don't" == "dont"), everything else separates.
			if r == '\'' || r == '’' || r == '`' {
				continue
			}
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
