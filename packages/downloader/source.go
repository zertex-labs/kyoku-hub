package downloader

import "context"

// Source is the contract a download provider implements.
//
// Deliberately narrower than the old Provider interface: the manager owns the
// walk, the classification and the per-track loop, because those rules must be
// identical for every source. A source only knows how to describe and fetch its
// own albums.
type Source interface {
	// Name is the stable id used in the chain, the credentials table and the UI.
	Name() string

	// Ceiling is the best quality this source can deliver for this account. Used
	// as the yardstick for fit/partial: an album that reaches the ceiling is a
	// FIT even when the ceiling is below rung 0 of the user's ladder.
	Ceiling() AudioQuality

	// SearchAlbums resolves the release group to this source's album ids. The
	// manager walks the chain by searching per source: the user picks a release
	// group, not a provider album.
	SearchAlbums(ctx context.Context, query string) ([]SearchResult, error)

	// ProbeAlbum answers "what would I get", without downloading. It must return
	// a candidate with per-track quality so the album floor is computable, and
	// must not treat a failed single-track probe as a low-quality track.
	ProbeAlbum(ctx context.Context, albumID string, ref AlbumRef, targets []QualityTarget) (AlbumCandidate, error)

	// DownloadTrack fetches one track to destPath and reports what was actually
	// delivered — which may be below the probe's claim. The caller verifies the
	// bytes and decides what to do about a downgrade.
	DownloadTrack(ctx context.Context, albumID, trackID, destPath string) (AudioQuality, error)
}

// Prober is the optional half of a Source, kept separate only so a source can be
// brought up for search-only use before its download path exists.
type Prober interface {
	ProbeAlbum(ctx context.Context, albumID string, ref AlbumRef, targets []QualityTarget) (AlbumCandidate, error)
}

// ArtistSource is an optional half of a Source: a provider that can list an
// artist's albums.
//
// Provider search is one blob of text. When a catalogue stores an artist under a
// different script, or an album under its original title — Deezer files 椎名林檎
// as "Sheena Ringo" and 三毒史 as "triviṣa-itihāsa" — the query finds nothing
// even though the release is right there. Walking the artist's discography is
// the way back, and it is only needed when the direct search came up empty.
type ArtistSource interface {
	ArtistAlbums(ctx context.Context, artist, title string) ([]SearchResult, error)
}
