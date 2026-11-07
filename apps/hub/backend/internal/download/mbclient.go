package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MusicBrainz allows one request per second per client, averaged. Everything
// here is best-effort metadata enrichment: a slow or unavailable MB must never
// fail a download, it only means fewer tags.
const (
	mbBaseURL    = "https://musicbrainz.org/ws/2"
	mbUserAgent  = "kyoku-hub/0.1 (https://github.com/zertex-labs/kyoku-hub)"
	mbMinSpacing = 1100 * time.Millisecond
)

type mbClient struct {
	http *http.Client
	base string
	ua   string

	// spacing is the per-call budget; zero in tests so a fake server is fast.
	spacing time.Duration

	mu   sync.Mutex
	last time.Time
}

func newMBClient() *mbClient {
	return &mbClient{
		http:    &http.Client{Timeout: 15 * time.Second},
		base:    mbBaseURL,
		ua:      mbUserAgent,
		spacing: mbMinSpacing,
	}
}

// get serialises calls at one per second. Holding the lock across the wait is
// deliberate: two concurrent lookups would otherwise both see an empty window
// and fire together.
func (c *mbClient) get(ctx context.Context, path string, out any) error {
	c.mu.Lock()
	if wait := c.spacing - time.Since(c.last); wait > 0 {
		select {
		case <-ctx.Done():
			c.mu.Unlock()
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	c.last = time.Now()
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("musicbrainz: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
