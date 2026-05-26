package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const lastfmPlaceholderMD5 = "2a96cbd8b46e442fc41c2b86b821562f"

// LastFMInfo contains artist image and bio fetched from Last.fm.
type LastFMInfo struct {
	ImageURL    string // Best available image URL (extralarge or mega)
	Description string // Bio summary (plain text, stripped of HTML)
}

// FetchLastFM fetches artist image and bio from Last.fm API using MBID.
// Returns nil (no error) if the entity has no image or description.
func FetchLastFM(ctx context.Context, apiKey, mbid string) (*LastFMInfo, error) {
	if apiKey == "" || mbid == "" {
		return &LastFMInfo{}, nil
	}

	u := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=artist.getinfo&mbid=%s&api_key=%s&format=json",
		url.QueryEscape(mbid), url.QueryEscape(apiKey))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lastfm: returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[lastfm] response: %s\n", string(body))

	var result struct {
		Artist struct {
			Image []struct {
				Text string `json:"#text"`
				Size string `json:"size"`
			} `json:"image"`
			Bio struct {
				Summary string `json:"summary"`
			} `json:"bio"`
		} `json:"artist"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	info := &LastFMInfo{}

	// Get best image (prefer mega > extralarge > large)
	sizeOrder := []string{"mega", "extralarge", "large"}
	for _, preferred := range sizeOrder {
		for _, img := range result.Artist.Image {
			if img.Size == preferred && img.Text != "" && isRealImage(img.Text) {
				info.ImageURL = img.Text
				break
			}
		}
		if info.ImageURL != "" {
			break
		}
	}

	// Bio summary — strip HTML tags and Last.fm attribution link
	if result.Artist.Bio.Summary != "" {
		info.Description = stripHTML(result.Artist.Bio.Summary)
	}

	return info, nil
}

// isRealImage filters out Last.fm's "no image" placeholder.
func isRealImage(url string) bool {
	return !strings.Contains(url, lastfmPlaceholderMD5)
}

// stripHTML removes HTML tags and trims the Last.fm attribution link.
func stripHTML(s string) string {
	// Remove <a href="...">...</a> attribution at the end
	if idx := strings.Index(s, "<a href"); idx != -1 {
		s = s[:idx]
	}
	// Strip remaining tags
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}
