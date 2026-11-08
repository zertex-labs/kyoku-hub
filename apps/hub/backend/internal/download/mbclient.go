package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MusicBrainz allows one request per second per client, averaged, and enforces it
// by IP across everything that talks to it. Being polite is not a nicety: a
// throttled lookup used to leave a track without a recording id and nothing in
// the log explained why.
//
// Three rules follow from that, in order of how much they save:
//
//  1. Ask once. Responses are cached for a day; a release group, a release and
//     its tracklist are the same answer every time. Re-tagging an album, or
//     opening the same release page twice, now costs nothing.
//  2. Ask once for the whole album. `inc=recordings` on the release returns every
//     track's recording id in one response, which is what replaced the per-ISRC
//     lookups.
//  3. When told to slow down, slow down. A 429/503 widens the spacing for a
//     cool-down window instead of retrying at the same rate.
const (
	mbBaseURL    = "https://musicbrainz.org/ws/2"
	mbUserAgent  = "kyoku-hub/0.1 (https://github.com/zertex-labs/kyoku-hub)"
	mbMinSpacing = 1100 * time.Millisecond
	mbMaxSpacing = 6 * time.Second

	// mbCacheTTL is how long a response is reused. Metadata changes on the scale
	// of months; a day is short enough to pick up a merge and long enough that
	// repeated work is free.
	mbCacheTTL = 24 * time.Hour
	mbCacheMax = 1024
)

type mbCacheEntry struct {
	body    []byte
	expires time.Time
}

type mbClient struct {
	http *http.Client
	base string
	ua   string

	// spacing is the per-call budget; zero in tests so a fake server is fast.
	spacing time.Duration
	// throttledUntil keeps the widened spacing after MusicBrainz pushed back.
	throttledUntil time.Time

	mu    sync.Mutex
	last  time.Time
	cache map[string]mbCacheEntry
}

func newMBClient() *mbClient {
	return &mbClient{
		http:    &http.Client{Timeout: 15 * time.Second},
		base:    mbBaseURL,
		ua:      mbUserAgent,
		spacing: mbMinSpacing,
		cache:   make(map[string]mbCacheEntry),
	}
}

// get returns a cached response when it has one, otherwise fetches it under the
// shared rate budget.
func (c *mbClient) get(ctx context.Context, path string, out any) error {
	if body, ok := c.cached(path); ok {
		return json.Unmarshal(body, out)
	}

	body, err := c.fetch(ctx, path)
	if err != nil {
		return err
	}
	c.store(path, body)
	return json.Unmarshal(body, out)
}

func (c *mbClient) cached(path string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[path]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(c.cache, path)
		}
		return nil, false
	}
	return e.body, true
}

func (c *mbClient) store(path string, body []byte) {
	if len(body) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]mbCacheEntry)
	}
	// Crude bound: an unbounded cache on a long-running hub is a leak, and the
	// entries are independent, so clearing everything is cheap and rare.
	if len(c.cache) >= mbCacheMax {
		c.cache = make(map[string]mbCacheEntry)
	}
	c.cache[path] = mbCacheEntry{body: body, expires: time.Now().Add(mbCacheTTL)}
}

// fetch is the retry loop: four attempts at 1s, 4s and 9s, never faster than the
// current spacing. A "server busy" from MusicBrainz is usually a minute of
// congestion, not a permanent no — and giving up turns into missing tags.
func (c *mbClient) fetch(ctx context.Context, path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt*attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		body, retryAfter, err := c.getOnce(ctx, path)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !isTransient(err) {
			return nil, err
		}
		c.slowDown(retryAfter)
	}
	return nil, lastErr
}

// slowDown widens the spacing after a throttle, for a minute. MusicBrainz's limit
// is averaged, so the answer to "too fast" is to be slower for a while, not to
// retry the same request immediately.
func (c *mbClient) slowDown(retryAfter time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next := c.spacing * 2
	if next < mbMaxSpacing/2 {
		next = mbMinSpacing * 2
	}
	if next > mbMaxSpacing {
		next = mbMaxSpacing
	}
	c.spacing = next
	c.throttledUntil = time.Now().Add(time.Minute)

	if retryAfter > 0 {
		c.last = time.Now().Add(retryAfter - c.spacing)
	}
}

// relax returns to the base spacing once the cool-down has passed, so one bad
// minute does not slow the client down for the rest of the process.
func (c *mbClient) relax() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.spacing > mbMinSpacing && time.Now().After(c.throttledUntil) {
		c.spacing = mbMinSpacing
	}
}

// waitTurn blocks until the next request is allowed. Holding the lock across the
// wait is deliberate: two concurrent lookups would otherwise both see an empty
// window and fire together.
func (c *mbClient) waitTurn(ctx context.Context) error {
	c.mu.Lock()
	if wait := c.spacing - time.Since(c.last); wait > 0 {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		c.mu.Lock()
	}
	c.last = time.Now()
	c.mu.Unlock()
	return nil
}

// getOnce performs one request and returns the body plus any Retry-After hint.
func (c *mbClient) getOnce(ctx context.Context, path string) ([]byte, time.Duration, error) {
	if err := c.waitTurn(ctx); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var retry time.Duration
		if s := resp.Header.Get("Retry-After"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 120 {
				retry = time.Duration(n) * time.Second
			}
		}
		return nil, retry, fmt.Errorf("musicbrainz: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, err
	}
	c.relax()
	return body, 0, nil
}

// isTransient reports whether MusicBrainz asked us to come back later.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 502")
}
