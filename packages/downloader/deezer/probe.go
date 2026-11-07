package deezer

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// ProbeAlbum answers "what would I actually get if I downloaded this album?".
//
// Deezer has no album-level quality anywhere: the public API returns nb_tracks,
// record_type and a release date but no bitrate or filesize, and the track
// endpoint has none either. The only source of truth is the private gw-light
// `song.getData`, one call per track — so this costs N authenticated calls and
// must only run for an album the user is about to download, never per search
// result.
//
// The account tier caps the answer on purpose: if the ARL cannot stream FLAC,
// the probe reports what the download would really deliver (320), not what the
// album contains.
func (d *Deezer) ProbeAlbum(
	ctx context.Context,
	albumID string,
	expected downloader.AlbumRef,
	targets []downloader.QualityTarget,
) (downloader.AlbumCandidate, error) {
	tracks, err := d.getTracksForAlbum(albumID)
	if err != nil {
		return downloader.AlbumCandidate{}, fmt.Errorf("probe %s: list tracks: %w", albumID, err)
	}

	meta := d.getAlbumMeta(albumID, tracks)
	year, _ := strconv.Atoi(meta.Year)

	cand := downloader.AlbumCandidate{
		Provider:    "deezer",
		ProviderID:  albumID,
		Format:      firstNonEmptyString(meta.RecordType, "album"),
		Title:       meta.Title,
		Artist:      meta.AlbumArtist,
		Year:        year,
		ReleaseDate: meta.ReleaseDate,
		Genre:       meta.Genre,
		Label:       meta.Label,
		Ceiling:     d.ceiling(),
		Tracks:      make([]downloader.TrackQuality, 0, len(tracks)),
	}

	for _, tr := range tracks {
		if err := ctx.Err(); err != nil {
			return cand, err
		}

		song, err := d.getSongData(tr.ID)
		if err != nil {
			if cand.ProbeError == "" {
				cand.ProbeError = err.Error()
			}
			// A failed probe is unknown, never "no quality": one flaky call must
			// not turn a perfect album yellow.
			cand.Tracks = append(cand.Tracks, downloader.TrackQuality{
				ProviderID: tr.ID,
				Number:     tr.TrackNumber,
				Disc:       tr.DiscNumber,
				Title:      tr.Title,
				Quality:    downloader.UnknownQuality,
				Known:      false,
				ReasonCode: "probe_failed: " + err.Error(),
			})
			continue
		}

		q, known, note := d.probeQuality(song)
		cand.Tracks = append(cand.Tracks, downloader.TrackQuality{
			ProviderID: tr.ID,
			Number:     tr.TrackNumber,
			Disc:       tr.DiscNumber,
			Title:      tr.Title,
			Quality:    q,
			Known:      known,
			ReasonCode: note,
			ISRC:       song.ISRC,
		})
	}

	cand.ApplyFloor(targets)
	verdict, _ := cand.Classify(expected, targets)
	cand.Confidence = confidenceFor(verdict, cand)
	return cand, nil
}

// ceiling is the best quality this account can pull from Deezer at all.
// Classification is measured against it, so a FLAC album on a Free account reads
// as "this is the best this account can do" instead of failing a hi-res rung it
// could never reach.
func (d *Deezer) ceiling() downloader.AudioQuality {
	switch {
	case d.auth.CanStreamLossless:
		return downloader.DeezerFLAC
	case d.auth.CanStreamHQ:
		return downloader.AudioQuality{Format: "mp3", Bitrate: 320}
	default:
		return downloader.AudioQuality{Format: "mp3", Bitrate: 128}
	}
}

// probeQuality maps one song's filesize fields onto a comparable quality,
// capped by what this account may actually stream.
func (d *Deezer) probeQuality(song GWSongData) (downloader.AudioQuality, bool, string) {
	has := func(size string) bool { return size != "" && size != "0" }
	flac := has(song.FILESIZE_FLAC)
	hq := has(song.FILESIZE_MP3_320)
	lq := has(song.FILESIZE_MP3_128)

	switch {
	case flac && d.auth.CanStreamLossless:
		return downloader.DeezerFLAC, true, ""
	case hq && d.auth.CanStreamHQ:
		note := ""
		if flac {
			note = "FLAC exists but this account cannot stream it"
		}
		return downloader.AudioQuality{Format: "mp3", Bitrate: 320}, true, note
	case lq:
		note := ""
		if flac || hq {
			note = "only the 128 kbps tier is available on this account"
		}
		return downloader.AudioQuality{Format: "mp3", Bitrate: 128}, true, note
	default:
		return downloader.UnknownQuality, false, "no playable format on this account"
	}
}

// confidenceFor turns a verdict into the 0..1 number the UI uses to decide
// between "offer it" and "just take it". Kept deliberately blunt for now: the
// verdict is already the product decision, and a fake-precise score would only
// invite tuning that cannot be validated.
func confidenceFor(v downloader.Verdict, c downloader.AlbumCandidate) float64 {
	switch v {
	case downloader.VerdictFit:
		return 1
	case downloader.VerdictPartial:
		if c.Floor.Known() {
			return 0.6
		}
		return 0.4
	default:
		return 0
	}
}

// firstNonEmptyString picks the first non-empty value; providers omit fields
// freely, and a zero value is never the answer we want in a tag.
func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
