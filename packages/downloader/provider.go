// Package downloader provides a library interface for downloading music from
// streaming services (Deezer, Tidal). It is designed to be used as a dependency
// by the hub backend, without any CLI or TUI dependencies.
package downloader

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Track represents a generic track from any music service.
type Track struct {
	ID        string
	Title     string
	Artist    string
	Album     string
	Quality   string
	Duration  int // seconds
	StreamURL string
	MimeType  string

	// Metadata for tagging
	AlbumArtist string
	TrackNumber int
	TotalTracks int
	DiscNumber  int
	TotalDiscs  int
	Date        string // release date (YYYY or YYYY-MM-DD)
	Copyright   string
	ISRC        string
	CoverURL    string // URL to album cover art

	// MusicBrainz enrichment
	Genre                  string
	MusicBrainzRecordingID string
	MusicBrainzReleaseID   string
	MusicBrainzArtistID    string
}

// Album represents a generic album from any music service.
type Album struct {
	ID          string
	Title       string
	Artist      string
	Tracks      []Track
	Date        string // release date
	TotalTracks int
	TotalDiscs  int
	CoverURL    string
}

// SearchResult represents a search result from a provider.
type SearchResult struct {
	Provider    string
	ID          string
	Title       string
	Artist      string
	CoverURL    string
	TrackCount  int
	ReleaseDate string
	Quality     string // best available quality
}

// Config holds common download configuration.
type Config struct {
	Format  string `json:"format"`
	Quality string `json:"quality,omitempty"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Format:  "%artist%/%album%/%track% - %title%.%extension%",
		Quality: "LOSSLESS",
	}
}

// Provider is the interface that all music service providers must implement.
// Unlike the CLI version, this interface is designed for library use:
// no interactive prompts, no TUI, all operations return data.
type Provider interface {
	// Name returns the service name (e.g., "deezer", "tidal").
	Name() string

	// IsAuthenticated checks if the provider has valid credentials.
	IsAuthenticated() bool

	// GetAlbum fetches album metadata and tracks by provider-specific ID.
	GetAlbum(id string) (Album, error)

	// GetTrack fetches track metadata by provider-specific ID.
	GetTrack(id string) (Track, error)

	// GetStreamURL returns the direct stream URL for a track.
	// For Deezer, streams are encrypted and need decryption.
	GetStreamURL(trackID string) (StreamInfo, error)

	// SearchAlbum searches for albums matching the query.
	SearchAlbum(query string) ([]SearchResult, error)

	// GetAlbumByURL fetches album info given a provider URL.
	// Returns the album ID extracted from the URL.
	GetAlbumByURL(url string) (string, error)
}

// StreamInfo holds stream URL and metadata for downloading.
type StreamInfo struct {
	URL              string
	URLs             []string // multiple URLs for DASH segment streams
	MimeType         string
	IsEncrypted      bool   // true for Deezer Blowfish-encrypted streams
	TrackID          string // needed for Deezer decryption key derivation
	RequestedQuality string
	ActualQuality    string
}

// DownloadProgress reports download progress for a single track.
type DownloadProgress struct {
	TrackIndex int
	TrackID    string
	Progress   float64 // 0-100
	Downloaded int64   // bytes
	TotalSize  int64   // bytes (0 if unknown)
	Speed      float64 // bytes/sec
	Phase      string  // "downloading", "decrypting", "tagging", "writing", "complete", "error"
	Error      string  // set when Phase == "error"
}

// --- Shared Utilities ---

// SanitizeFilename removes characters that are problematic in filenames.
func SanitizeFilename(s string) string {
	replacements := map[string]string{
		"/":  "-",
		"\\": "-",
		":":  " - ",
		"*":  "",
		"?":  "",
		"\"": "'",
		"<":  "",
		">":  "",
		"|":  "-",
	}

	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
	}

	return strings.TrimSpace(s)
}

// MimeToExtension converts a MIME type to a file extension.
func MimeToExtension(mimeType string) string {
	switch {
	case strings.Contains(mimeType, "flac"):
		return "flac"
	case strings.Contains(mimeType, "mp4"), strings.Contains(mimeType, "aac"):
		return "m4a"
	case strings.Contains(mimeType, "mpeg"):
		return "mp3"
	case strings.Contains(mimeType, "ogg"):
		return "ogg"
	case strings.Contains(mimeType, "wav"):
		return "wav"
	default:
		return "m4a"
	}
}

// FormatFilename applies a format template to generate a filename.
func FormatFilename(format string, track Track, extension string) string {
	filename := format

	artist := track.AlbumArtist
	if artist == "" {
		artist = track.Artist
	}
	if artist == "" {
		artist = "Unknown Artist"
	}

	title := track.Title
	if title == "" {
		title = "Unknown Title"
	}

	album := track.Album
	if album == "" {
		album = "Unknown Album"
	}

	filename = strings.ReplaceAll(filename, "%id%", track.ID)
	filename = strings.ReplaceAll(filename, "%albumartist%", SanitizeFilename(artist))
	filename = strings.ReplaceAll(filename, "%artist%", SanitizeFilename(artist))
	filename = strings.ReplaceAll(filename, "%title%", SanitizeFilename(title))
	filename = strings.ReplaceAll(filename, "%album%", SanitizeFilename(album))
	filename = strings.ReplaceAll(filename, "%quality%", track.Quality)
	filename = strings.ReplaceAll(filename, "%extension%", extension)

	if track.TrackNumber > 0 {
		pad := 2
		if track.TotalTracks >= 100 {
			pad = 3
		}
		filename = strings.ReplaceAll(filename, "%track%", fmt.Sprintf("%0*d", pad, track.TrackNumber))
	} else {
		filename = strings.ReplaceAll(filename, "%track%", "00")
	}

	if track.DiscNumber > 0 {
		filename = strings.ReplaceAll(filename, "%disc%", fmt.Sprintf("%d", track.DiscNumber))
	} else {
		filename = strings.ReplaceAll(filename, "%disc%", "1")
	}

	if track.Date != "" {
		filename = strings.ReplaceAll(filename, "%date%", track.Date)
		year := track.Date
		if len(track.Date) >= 4 {
			year = track.Date[:4]
		}
		filename = strings.ReplaceAll(filename, "%year%", year)
	} else {
		filename = strings.ReplaceAll(filename, "%date%", "Unknown")
		filename = strings.ReplaceAll(filename, "%year%", "Unknown")
	}

	return filepath.ToSlash(filename)
}
