package downloader

import "context"

type SearchResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Year       string `json:"year"`
	TrackCount int    `json:"track_count"`
	CoverURL   string `json:"cover_url"`
	Provider   string `json:"provider"`
	Quality    string `json:"quality"`
}

type Config struct {
	Quality string // "FLAC", "MP3_320", "MP3_128"
}

func DefaultConfig() Config { return Config{Quality: "FLAC"} }

type Provider interface {
	SearchAlbum(q string) ([]SearchResult, error)
	DownloadAlbum(ctx context.Context, albumID, destDir string, onProgress func(phase string, n, total int)) error
}
