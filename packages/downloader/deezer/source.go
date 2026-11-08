package deezer

import (
	"context"
	"fmt"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// Name implements downloader.Source.
func (d *Deezer) Name() string { return "deezer" }

// SearchAlbums implements downloader.Source.
func (d *Deezer) SearchAlbums(ctx context.Context, query string) ([]downloader.SearchResult, error) {
	return d.SearchAlbum(query)
}

// Ceiling implements downloader.Source: the best this account can pull.
func (d *Deezer) Ceiling() downloader.AudioQuality { return d.ceiling() }

// DownloadTrack implements downloader.Source: one track, decrypted, on disk.
//
// The returned quality is what the CDN actually served, not what we asked for —
// Deezer downgrades silently when a tier is unavailable, and the caller needs to
// know so it can revoke a fit instead of publishing a 320 as if it were FLAC.
func (d *Deezer) DownloadTrack(ctx context.Context, albumID, trackID, destPath string) (downloader.AudioQuality, error) {
	song, err := d.getSongData(trackID)
	if err != nil {
		return downloader.UnknownQuality, fmt.Errorf("track %s: song data: %w", trackID, err)
	}

	info, err := d.getStreamURL(trackID, deezerQuality(d.config.Quality), song)
	if err != nil {
		return downloader.UnknownQuality, fmt.Errorf("track %s: stream url: %w", trackID, err)
	}

	if err := d.downloadTrack(ctx, info.URL, destPath, trackID); err != nil {
		return downloader.UnknownQuality, fmt.Errorf("track %s: download: %w", trackID, err)
	}

	return actualQuality(info.ActualQuality), nil
}

// TrackTicket implements downloader.Ticketer: the URL, the headers and the
// decryption recipe, without touching the bytes.
func (d *Deezer) TrackTicket(ctx context.Context, albumID, trackID string) (downloader.TrackTicket, error) {
	song, err := d.getSongData(trackID)
	if err != nil {
		return downloader.TrackTicket{}, fmt.Errorf("track %s: song data: %w", trackID, err)
	}

	info, err := d.getStreamURL(trackID, deezerQuality(d.config.Quality), song)
	if err != nil {
		return downloader.TrackTicket{}, fmt.Errorf("track %s: stream url: %w", trackID, err)
	}

	return downloader.TrackTicket{
		URL:     info.URL,
		Headers: map[string]string{"User-Agent": browserUA},
		// Every stream we are handed is encrypted in 2048-byte stripes, with the
		// key derived from the track id and the IV fixed.
		Transform: &downloader.StreamTransform{
			Kind:  "stripe-cbc",
			Chunk: chunkSize,
			Every: 3,
			Key:   DeriveBlowfishKey(trackID),
			IV:    blowfishIV,
		},
		Quality: actualQuality(info.ActualQuality),
	}, nil
}

// actualQuality maps the tier the CDN granted onto the comparable type.
func actualQuality(tier string) downloader.AudioQuality {
	switch tier {
	case "FLAC", "LOSSLESS", "HI_RES_LOSSLESS":
		return downloader.DeezerFLAC
	case "MP3_320", "HIGH":
		return downloader.AudioQuality{Format: "mp3", Bitrate: 320}
	case "MP3_128":
		return downloader.AudioQuality{Format: "mp3", Bitrate: 128}
	default:
		return downloader.UnknownQuality
	}
}
