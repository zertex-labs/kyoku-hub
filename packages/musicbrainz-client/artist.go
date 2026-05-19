package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SearchArtist searches for artists by name.
// Limit defaults to 25, offset defaults to 0.
func (c *Client) SearchArtist(ctx context.Context, query string, limit, offset int) (*ArtistSearchResult, error) {
	if limit <= 0 {
		limit = 25
	}

	u := fmt.Sprintf("%s/artist?query=%s&fmt=json&limit=%d&offset=%d",
		c.baseURL, url.QueryEscape(query), limit, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: artist search returned %d", resp.StatusCode)
	}

	var result ArtistSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LookupArtist fetches a single artist by MBID.
// Pass inc values like "release-groups" to include additional data.
func (c *Client) LookupArtist(ctx context.Context, mbid string, inc ...string) (*Artist, error) {
	u := fmt.Sprintf("%s/artist/%s?fmt=json", c.baseURL, url.PathEscape(mbid))
	if len(inc) > 0 {
		incStr := ""
		for i, v := range inc {
			if i > 0 {
				incStr += "+"
			}
			incStr += v
		}
		u += "&inc=" + url.QueryEscape(incStr)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: artist lookup returned %d", resp.StatusCode)
	}

	var artist Artist
	if err := json.NewDecoder(resp.Body).Decode(&artist); err != nil {
		return nil, err
	}
	return &artist, nil
}
