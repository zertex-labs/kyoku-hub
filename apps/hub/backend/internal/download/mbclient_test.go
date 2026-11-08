package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func countingMB(t *testing.T, status func(n int32) int) (*mbClient, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if code := status(n); code != http.StatusOK {
			if code == http.StatusServiceUnavailable {
				w.Header().Set("Retry-After", "1")
			}
			w.WriteHeader(code)
			return
		}
		fmt.Fprintf(w, `{"id":"x","title":"hit %d"}`, n)
	}))
	t.Cleanup(srv.Close)
	return &mbClient{http: srv.Client(), base: srv.URL, ua: "test", spacing: 0}, &hits
}

// The same question twice is one request: album pages, re-tags and repeated
// searches all ask for the same release group.
func TestMBClientCachesRepeatedLookups(t *testing.T) {
	c, hits := countingMB(t, func(int32) int { return http.StatusOK })

	var first, second struct{ Title string }
	if err := c.get(context.Background(), "/release-group/abc", &first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := c.get(context.Background(), "/release-group/abc", &second); err != nil {
		t.Fatalf("second: %v", err)
	}

	if n := atomic.LoadInt32(hits); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
	if first.Title != second.Title {
		t.Errorf("cache returned a different answer: %q vs %q", first.Title, second.Title)
	}
}

// A throttle is answered by being slower, not by retrying at the same rate.
func TestMBClientSlowsDownWhenThrottled(t *testing.T) {
	// Fail the first call, succeed after: the retry must happen and the spacing
	// must widen.
	c, hits := countingMB(t, func(n int32) int {
		if n == 1 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})

	var out struct{ Title string }
	if err := c.get(context.Background(), "/isrc/XYZ", &out); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if n := atomic.LoadInt32(hits); n != 2 {
		t.Fatalf("requests = %d, want 2 (one retry)", n)
	}

	c.mu.Lock()
	spacing := c.spacing
	c.mu.Unlock()
	if spacing <= mbMinSpacing {
		t.Errorf("spacing = %v, want more than the base %v", spacing, mbMinSpacing)
	}
}

// A 400 is not a throttle: it must not be retried, and it must not slow anything.
func TestMBClientDoesNotRetryClientErrors(t *testing.T) {
	c, hits := countingMB(t, func(int32) int { return http.StatusBadRequest })

	var out struct{ Title string }
	if err := c.get(context.Background(), "/release/nope", &out); err == nil {
		t.Fatal("expected an error")
	}
	if n := atomic.LoadInt32(hits); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
}

// After the cool-down the client returns to its normal rate.
func TestMBClientRelaxesAfterCooldown(t *testing.T) {
	c, _ := countingMB(t, func(int32) int { return http.StatusOK })
	c.spacing = mbMaxSpacing
	c.throttledUntil = time.Now().Add(-time.Second)

	c.relax()
	if c.spacing != mbMinSpacing {
		t.Fatalf("spacing = %v, want %v", c.spacing, mbMinSpacing)
	}
}
