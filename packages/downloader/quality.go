package downloader

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AudioQuality is the comparable form of "what you actually get".
//
// Every provider speaks its own dialect ("FLAC", "LOSSLESS", "HI_RES_LOSSLESS",
// "MP3_320"), and none of them can be ordered against each other. Mapping each
// into this struct is what makes source choice, upgrade detection and "prefer
// hi-res" one function instead of three features.
type AudioQuality struct {
	Format     string // "flac", "mp3", "aac", "alac", "wav", "dsf", "opus", "unknown"
	Bitrate    int    // kbps; 0 when unknown or meaningless (lossless)
	SampleRate int    // Hz; 0 when unknown
	BitDepth   int    // bits per sample; 0 when unknown
}

// UnknownQuality is the zero-provenance value. It never satisfies a target and
// never over-claims: a missing probe result must not read as "CD quality".
var UnknownQuality = AudioQuality{Format: "unknown"}

// Known reports whether the format is actually identified.
func (q AudioQuality) Known() bool {
	f := strings.ToLower(q.Format)
	return f != "" && f != "unknown"
}

// Lossless reports whether the format carries the full signal.
func (q AudioQuality) Lossless() bool {
	switch strings.ToLower(q.Format) {
	case "flac", "alac", "wav", "dsf", "aiff":
		return true
	}
	return false
}

// TierScore orders two qualities that satisfy the same target. It is a
// tiebreaker only — cross-format priority comes from the user's target list.
func (q AudioQuality) TierScore() float64 {
	base := map[string]float64{
		"dsf": 102, "flac": 100, "alac": 98, "wav": 95,
		"ogg": 70, "opus": 65, "aac": 60, "mp3": 50,
	}
	b, ok := base[strings.ToLower(q.Format)]
	if !ok {
		b = 10
	}
	if q.Lossless() {
		sr := q.SampleRate
		if sr == 0 {
			sr = 44100
		}
		bd := q.BitDepth
		if bd == 0 {
			bd = 16
		}
		srScore := math.Min(float64(sr)/192000, 1) * 20 // 44.1k = 0, 192k = +20
		bdScore := math.Max(float64(bd-16), 0) / 8 * 10 // 16-bit = 0, 24-bit = +10
		return b + srScore + bdScore
	}
	br := q.Bitrate
	if br > 320 {
		br = 320
	}
	return b + float64(br)/320*10
}

// ShortLabel is the compact form used in summaries: "FLAC 16/44.1", "MP3 320".
func (q AudioQuality) ShortLabel() string {
	f := strings.ToUpper(q.Format)
	if !q.Known() {
		return "unknown"
	}
	if q.Lossless() {
		parts := make([]string, 0, 2)
		if q.BitDepth > 0 {
			parts = append(parts, strconv.Itoa(q.BitDepth))
		}
		if q.SampleRate > 0 {
			parts = append(parts, trimKhz(q.SampleRate))
		}
		if len(parts) == 0 {
			return f
		}
		return f + " " + strings.Join(parts, "/")
	}
	if q.Bitrate > 0 {
		return fmt.Sprintf("%s %d", f, q.Bitrate)
	}
	return f
}

// Label is the long form for tooltips and the per-track expansion:
// "FLAC 16-bit/44.1kHz", "MP3 320kbps".
func (q AudioQuality) Label() string {
	f := strings.ToUpper(q.Format)
	if !q.Known() {
		return "unknown"
	}
	if q.Lossless() {
		var b strings.Builder
		b.WriteString(f)
		if q.BitDepth > 0 || q.SampleRate > 0 {
			b.WriteString(" ")
			if q.BitDepth > 0 {
				b.WriteString(strconv.Itoa(q.BitDepth))
				b.WriteString("-bit")
			}
			if q.SampleRate > 0 {
				if q.BitDepth > 0 {
					b.WriteString("/")
				}
				b.WriteString(trimKhz(q.SampleRate))
				b.WriteString("kHz")
			}
		}
		return b.String()
	}
	if q.Bitrate > 0 {
		return fmt.Sprintf("%s %dkbps", f, q.Bitrate)
	}
	return f
}

func trimKhz(hz int) string {
	v := float64(hz) / 1000
	if v == math.Trunc(v) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// QualityTarget is one rung of the user's ranked preference ladder. Rungs are
// ordered: index 0 is the most wanted.
type QualityTarget struct {
	Label         string // human name, e.g. "FLAC 24-bit/96kHz"
	Format        string // "flac", "mp3", "" = any
	MinBitrate    int    // kbps floor for lossy rungs
	MinSampleRate int    // Hz floor for hi-res rungs
	BitDepth      int    // required bit depth for hi-res rungs
}

func (t QualityTarget) String() string {
	if t.Label != "" {
		return t.Label
	}
	return AudioQuality{Format: t.Format, Bitrate: t.MinBitrate, SampleRate: t.MinSampleRate, BitDepth: t.BitDepth}.Label()
}

// Meets reports whether q satisfies t.
//
// Unknown fields fail a rung that demands them, unless the rung can be inferred
// from a bitrate: an unprobed FLAC must never out-rank a known 24/96 one, and
// must never be thrown out of a plain 16-bit rung either.
func (q AudioQuality) Meets(t QualityTarget) bool {
	if !q.Known() {
		return false
	}
	if t.Format != "" && strings.ToLower(t.Format) != strings.ToLower(q.Format) {
		return false
	}
	if t.MinBitrate > 0 && q.Bitrate < t.MinBitrate {
		return false
	}
	if t.MinSampleRate > 0 {
		switch {
		case q.SampleRate > 0:
			if q.SampleRate < t.MinSampleRate {
				return false
			}
		case q.Lossless() && q.Bitrate > 0:
			// 16/44.1 ~= 1411 kbps; 24/96 ~= 4608; 24/192 ~= 9216.
			if q.Bitrate < minKbpsFor(t.MinSampleRate, defaultDepth(t.BitDepth)) {
				return false
			}
		default:
			return false
		}
	}
	if t.BitDepth > 0 {
		switch {
		case q.BitDepth > 0:
			if q.BitDepth < t.BitDepth {
				return false
			}
		case q.Lossless() && t.BitDepth >= 24:
			// A hi-res rung needs proof: a bare "flac" with no spec is CD at best.
			if q.Bitrate < 1450 {
				return false
			}
		}
	}
	return true
}

func defaultDepth(d int) int {
	if d > 0 {
		return d
	}
	return 16
}

func minKbpsFor(sampleRate, bitDepth int) int {
	// sampleRate * bitDepth * 2 channels / 1000, with a little headroom for
	// FLAC's variable block sizes.
	return sampleRate * bitDepth * 2 / 1000
}

// Rank returns (targetIndex, tierScore) for sorting. Lower index wins; a
// candidate that satisfies no rung sorts last but is not discarded — the caller
// decides whether to offer it.
func Rank(q AudioQuality, targets []QualityTarget) (int, float64) {
	for i, t := range targets {
		if q.Meets(t) {
			return i, q.TierScore()
		}
	}
	return len(targets), q.TierScore()
}

// DefaultTargets is the ladder a new user starts on. It ends with a lossy rung
// on purpose: without a bottom rung nothing below FLAC is ever offerable, and
// a "try the fallback source last" rule becomes unreachable.
func DefaultTargets() []QualityTarget {
	return []QualityTarget{
		{Label: "FLAC 24-bit/192kHz", Format: "flac", BitDepth: 24, MinSampleRate: 192000},
		{Label: "FLAC 24-bit/96kHz", Format: "flac", BitDepth: 24, MinSampleRate: 96000},
		{Label: "FLAC 24-bit/48kHz", Format: "flac", BitDepth: 24, MinSampleRate: 48000},
		{Label: "FLAC 16-bit", Format: "flac", BitDepth: 16},
		{Label: "MP3 320kbps", Format: "mp3", MinBitrate: 320},
		{Label: "MP3 192kbps", Format: "mp3", MinBitrate: 192},
	}
}

// DeezerFLAC is Deezer's only lossless tier: always CD quality.
var DeezerFLAC = AudioQuality{Format: "flac", SampleRate: 44100, BitDepth: 16}
