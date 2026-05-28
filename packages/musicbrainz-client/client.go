package musicbrainz

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// ErrNotFound is returned when MusicBrainz returns a 404 for a lookup.
var ErrNotFound = errors.New("musicbrainz: not found")

// APIError represents a non-OK response from MusicBrainz.
type APIError struct {
	StatusCode int
	Endpoint   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("musicbrainz: %s returned %d", e.Endpoint, e.StatusCode)
}

type Client struct {
	http      *http.Client
	limiter   *rate.Limiter
	userAgent string
	baseURL   string
}

func NewClient(userAgent string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		limiter:   rate.NewLimiter(rate.Every(time.Second), 1),
		userAgent: userAgent,
		baseURL:   "https://musicbrainz.org/ws/2",
	}
}

func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	var resp *http.Response
	var err error
	for attempt := range 3 {
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusServiceUnavailable {
			return resp, nil
		}
		resp.Body.Close()
		backoff := time.Duration(1<<attempt) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return resp, nil
}
