package downloader

import "testing"

func TestRankOrdersByTargetList(t *testing.T) {
	targets := DefaultTargets()

	cases := []struct {
		name string
		q    AudioQuality
		want int
	}{
		{"flac 24/192", AudioQuality{Format: "flac", SampleRate: 192000, BitDepth: 24}, 0},
		{"flac 24/96", AudioQuality{Format: "flac", SampleRate: 96000, BitDepth: 24}, 1},
		{"flac 16/44.1", DeezerFLAC, 3},
		{"bare flac (no spec)", AudioQuality{Format: "flac"}, 3},
		{"flac by bitrate only", AudioQuality{Format: "flac", Bitrate: 1411}, 3},
		{"mp3 320", AudioQuality{Format: "mp3", Bitrate: 320}, 4},
		{"mp3 128 satisfies nothing", AudioQuality{Format: "mp3", Bitrate: 128}, len(targets)},
		{"unknown satisfies nothing", UnknownQuality, len(targets)},
	}

	for _, tc := range cases {
		got, _ := Rank(tc.q, targets)
		if got != tc.want {
			t.Errorf("%s: target index = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestHiResTargetNeedsProof(t *testing.T) {
	// A FLAC with no sample rate/bit depth must not be handed a 24-bit rung:
	// unknown spec is CD at best, and claiming otherwise makes "prefer hi-res"
	// a lie the download cannot keep.
	bare := AudioQuality{Format: "flac"}
	hiRes := QualityTarget{Format: "flac", BitDepth: 24, MinSampleRate: 96000}
	if bare.Meets(hiRes) {
		t.Fatal("bare FLAC satisfied a 24/96 target")
	}

	// With only a bitrate, the kbps heuristic decides.
	byRate := AudioQuality{Format: "flac", Bitrate: 4608} // ~24/96 in FLAC
	if !byRate.Meets(hiRes) {
		t.Error("24/96-rate FLAC did not satisfy a 24/96 target")
	}
	if (AudioQuality{Format: "flac", Bitrate: 1411}).Meets(hiRes) {
		t.Error("CD-rate FLAC satisfied a hi-res target")
	}
}

func TestFormatMismatchNeverSatisfies(t *testing.T) {
	flacOnly := QualityTarget{Format: "flac", BitDepth: 16}
	if (AudioQuality{Format: "mp3", Bitrate: 320}).Meets(flacOnly) {
		t.Fatal("mp3 320 satisfied a flac-only rung")
	}
}

func TestTierScoreOrdersWithinABucket(t *testing.T) {
	cd := DeezerFLAC
	hi := AudioQuality{Format: "flac", SampleRate: 96000, BitDepth: 24}
	mp3 := AudioQuality{Format: "mp3", Bitrate: 320}
	if !(hi.TierScore() > cd.TierScore()) {
		t.Error("24/96 did not outrank 16/44.1")
	}
	if !(cd.TierScore() > mp3.TierScore()) {
		t.Error("FLAC did not outrank MP3 320")
	}
}

func TestShortLabel(t *testing.T) {
	cases := map[string]AudioQuality{
		"FLAC 16/44.1": DeezerFLAC,
		"FLAC 24/96":   {Format: "flac", SampleRate: 96000, BitDepth: 24},
		"MP3 320":      {Format: "mp3", Bitrate: 320},
		"unknown":      UnknownQuality,
	}
	for want, q := range cases {
		if got := q.ShortLabel(); got != want {
			t.Errorf("ShortLabel(%+v) = %q, want %q", q, got, want)
		}
	}
}
