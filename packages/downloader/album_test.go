package downloader

import (
	"strings"
	"testing"
)

func album(tracks ...TrackQuality) AlbumCandidate {
	c := AlbumCandidate{
		Provider: "deezer",
		Title:    "Discovery",
		Artist:   "Daft Punk",
		Year:     2001,
		Ceiling:  DeezerFLAC, // best Deezer can ever do: CD-quality FLAC
		Tracks:   tracks,
	}
	c.ApplyFloor(DefaultTargets())
	return c
}

func flacTrack(n int) TrackQuality {
	return TrackQuality{Number: n, Title: "T", Quality: DeezerFLAC, Known: true}
}

func mp3Track(n int, kbps int) TrackQuality {
	return TrackQuality{Number: n, Title: "T", Quality: AudioQuality{Format: "mp3", Bitrate: kbps}, Known: true}
}

func unknownTrack(n int) TrackQuality {
	return TrackQuality{Number: n, Title: "T", Quality: UnknownQuality, Known: false}
}

var discovery = AlbumRef{Title: "Discovery", Artist: "Daft Punk", Year: 2001, TrackCount: 4}

func TestClassifyFit(t *testing.T) {
	c := album(flacTrack(1), flacTrack(2), flacTrack(3), flacTrack(4))
	got, reasons := c.Classify(discovery, DefaultTargets())
	if got != VerdictFit {
		t.Fatalf("verdict = %s, want fit (reasons: %v)", got, reasons)
	}
	if len(reasons) != 0 {
		t.Errorf("fit should carry no reasons, got %v", reasons)
	}
	if want := "4/4 songs · FLAC 16/44.1"; c.Summary(DefaultTargets()) != want {
		t.Errorf("summary = %q, want %q", c.Summary(DefaultTargets()), want)
	}
}

func TestClassifyPartialNamesTheBadTrack(t *testing.T) {
	c := album(flacTrack(1), flacTrack(2), mp3Track(3, 320), flacTrack(4))
	got, reasons := c.Classify(discovery, DefaultTargets())
	if got != VerdictPartial {
		t.Fatalf("verdict = %s, want partial", got)
	}
	if c.OkTracks != 3 {
		t.Errorf("OkTracks = %d, want 3", c.OkTracks)
	}
	joined := strings.Join(reasons, " | ")
	if !strings.Contains(joined, "t03") || !strings.Contains(joined, "MP3 320") {
		t.Errorf("reasons should name the track and its quality, got %q", joined)
	}
	// Summary leads with completeness, never "3/4 FLAC".
	if want := "4/4 songs · floor MP3 320"; c.Summary(DefaultTargets()) != want {
		t.Errorf("summary = %q, want %q", c.Summary(DefaultTargets()), want)
	}
}

func TestSingleTrackFloor(t *testing.T) {
	c := AlbumCandidate{
		Provider: "deezer", Title: "One More Time", Artist: "Daft Punk", Year: 2000,
		Tracks: []TrackQuality{mp3Track(1, 320)},
	}
	c.ApplyFloor(DefaultTargets())
	ref := AlbumRef{Title: "One More Time", Artist: "Daft Punk", Year: 2000, TrackCount: 1}
	if got, _ := c.Classify(ref, DefaultTargets()); got != VerdictPartial {
		t.Fatalf("single with one 320 track = %s, want partial", got)
	}
	if want := "1/1 songs · MP3 320"; c.Summary(DefaultTargets()) != want {
		t.Errorf("summary = %q, want %q (never %q)", c.Summary(DefaultTargets()), want, "0/1 FLAC")
	}
}

func TestUnknownTrackDoesNotDemoteTheFloor(t *testing.T) {
	c := album(flacTrack(1), flacTrack(2), unknownTrack(3), flacTrack(4))
	if c.Floor != DeezerFLAC {
		t.Fatalf("floor = %s, want FLAC: one failed probe must not drag the album down", c.Floor.ShortLabel())
	}
	if c.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", c.Unknown)
	}
	// Still not a silent FIT — an unproven track cannot be claimed as good.
	got, reasons := c.Classify(discovery, DefaultTargets())
	if got != VerdictPartial {
		t.Fatalf("verdict = %s, want partial", got)
	}
	if !strings.Contains(strings.Join(reasons, " "), "could not be probed") {
		t.Errorf("reasons should mention the unprobed track, got %v", reasons)
	}
}

func TestIdentityMismatchIsAMiss(t *testing.T) {
	// Right quality, wrong release: a 2019 remaster offered for a 2001 group.
	c := album(flacTrack(1), flacTrack(2), flacTrack(3), flacTrack(4))
	c.Year = 2019
	got, reasons := c.Classify(discovery, DefaultTargets())
	if got != VerdictMiss {
		t.Fatalf("verdict = %s, want miss", got)
	}
	if !strings.Contains(strings.Join(reasons, " "), "year") {
		t.Errorf("reasons should explain the year mismatch, got %v", reasons)
	}

	// Wrong track count is also a miss.
	c2 := album(flacTrack(1), flacTrack(2))
	if got, _ := c2.Classify(discovery, DefaultTargets()); got != VerdictMiss {
		t.Errorf("short album = %s, want miss", got)
	}
}

func TestRemasterSuffixStillMatches(t *testing.T) {
	c := album(flacTrack(1), flacTrack(2), flacTrack(3), flacTrack(4))
	c.Title = "Discovery (Remastered)"
	if got, _ := c.Classify(discovery, DefaultTargets()); got == VerdictMiss {
		t.Fatal("a remaster suffix should not make the release a miss by itself")
	}
}

func TestNormalizeName(t *testing.T) {
	cases := [][2]string{
		{"Don't Stop Me Now", "dont stop me now"},
		{"AC/DC", "ac dc"},
		{"  Discovery  ", "discovery"},
		{"Sgt. Pepper's", "sgt peppers"},
	}
	for _, c := range cases {
		if got, want := normalizeName(c[0]), normalizeName(c[1]); got != want {
			t.Errorf("normalizeName(%q) = %q, want %q", c[0], got, want)
		}
	}
}

func TestEmptyAlbumIsAMiss(t *testing.T) {
	var c AlbumCandidate
	c.ApplyFloor(DefaultTargets())
	if got, _ := c.Classify(AlbumRef{}, DefaultTargets()); got != VerdictMiss {
		t.Errorf("empty candidate = %s, want miss", got)
	}
	if got := c.Summary(DefaultTargets()); got != "no tracks" {
		t.Errorf("summary = %q, want %q", got, "no tracks")
	}
}
