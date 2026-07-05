package tidal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

const baseURL = "https://api.tidal.com/v1"

type Auth struct {
	Token        string
	RefreshToken string
	CountryCode  string
	UserID       string
	ExpiresAt    int64
}

type Tidal struct {
	auth   Auth
	config downloader.Config
	client *http.Client
}

func New(auth Auth, cfg downloader.Config) *Tidal {
	return &Tidal{
		auth:   auth,
		config: cfg,
		client: &http.Client{},
	}
}

func (t *Tidal) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.auth.Token)
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tidal API %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *Tidal) cc() string {
	if t.auth.CountryCode != "" {
		return t.auth.CountryCode
	}
	return "US"
}

func (t *Tidal) SearchAlbum(q string) ([]downloader.SearchResult, error) {
	path := fmt.Sprintf("/search?query=%s&types=ALBUMS&countryCode=%s&limit=20",
		url.QueryEscape(q), t.cc())

	var resp struct {
		Albums struct {
			Items []struct {
				ID             int    `json:"id"`
				Title          string `json:"title"`
				ReleaseDate    string `json:"releaseDate"`
				NumberOfTracks int    `json:"numberOfTracks"`
				AudioQuality   string `json:"audioQuality"`
				Cover          string `json:"cover"`
				Artist         struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := t.get(path, &resp); err != nil {
		return nil, err
	}

	results := make([]downloader.SearchResult, 0, len(resp.Albums.Items))
	for _, item := range resp.Albums.Items {
		year := ""
		if len(item.ReleaseDate) >= 4 {
			year = item.ReleaseDate[:4]
		}
		quality := item.AudioQuality
		if quality == "" {
			quality = "NORMAL"
		}
		coverURL := ""
		if item.Cover != "" {
			coverURL = fmt.Sprintf("https://resources.tidal.com/images/%s/320x320.jpg",
				strings.ReplaceAll(item.Cover, "-", "/"))
		}
		results = append(results, downloader.SearchResult{
			ID:         fmt.Sprintf("%d", item.ID),
			Title:      item.Title,
			Artist:     item.Artist.Name,
			Year:       year,
			TrackCount: item.NumberOfTracks,
			CoverURL:   coverURL,
			Provider:   "tidal",
			Quality:    quality,
		})
	}
	return results, nil
}

func (t *Tidal) DownloadAlbum(ctx context.Context, albumID, destDir string, onProgress func(phase string, n, total int)) error {
	var tracksResp struct {
		Items []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := t.get(fmt.Sprintf("/albums/%s/tracks?countryCode=%s", albumID, t.cc()), &tracksResp); err != nil {
		return fmt.Errorf("get tracks: %w", err)
	}

	total := len(tracksResp.Items)
	if total == 0 {
		return fmt.Errorf("no tracks found for album %s", albumID)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	for i, track := range tracksResp.Items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		onProgress("downloading", i+1, total)

		var streamResp struct {
			URL string `json:"url"`
		}
		streamPath := fmt.Sprintf("/tracks/%d/streamUrl?soundQuality=LOSSLESS&countryCode=%s", track.ID, t.cc())
		if err := t.get(streamPath, &streamResp); err != nil {
			return fmt.Errorf("get stream url for track %d: %w", track.ID, err)
		}
		if streamResp.URL == "" {
			return fmt.Errorf("empty stream url for track %d", track.ID)
		}

		safeTitle := sanitizeFilename(track.Title)
		destPath := filepath.Join(destDir, fmt.Sprintf("%02d - %s.flac", i+1, safeTitle))
		if err := downloadFile(ctx, t.client, streamResp.URL, destPath); err != nil {
			return fmt.Errorf("download track %d: %w", track.ID, err)
		}
	}

	onProgress("complete", total, total)
	return nil
}

func downloadFile(ctx context.Context, client *http.Client, rawURL, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "", "\"", "", "<", "", ">", "", "|", "-",
	)
	return strings.TrimSpace(replacer.Replace(s))
}
