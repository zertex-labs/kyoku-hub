package musicbrainz

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

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
