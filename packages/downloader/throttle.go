package downloader

import (
	"context"
	"sync"
	"time"
)

// Throttle paces outbound calls to one provider account.
//
// One budget per account, shared by every caller — search, probe, download,
// background refresh. A per-call-site sleep is arithmetic that never adds up:
// ten goroutines each politely waiting 100ms still hit the provider ten times in
// the same second. Here a caller reserves the *next* slot under a lock, so two
// callers arriving together get two different times instead of the same one.
type Throttle struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	cooldown time.Time
}

// NewThrottle builds a budget of perSecond sustained calls/s.
// Deezer publishes no limit; the community figure is 50 per 5s, so asking for
// ~8/s leaves most of it as margin for being wrong.
func NewThrottle(perSecond float64) *Throttle {
	if perSecond <= 0 {
		perSecond = 1
	}
	return &Throttle{interval: time.Duration(float64(time.Second) / perSecond)}
}

// Wait blocks until the caller's reserved slot, honouring ctx and any cooldown.
func (t *Throttle) Wait(ctx context.Context) error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	now := time.Now()
	slot := t.next
	if slot.Before(now) {
		slot = now
	}
	if t.cooldown.After(slot) {
		slot = t.cooldown
	}
	t.next = slot.Add(t.interval)
	t.mu.Unlock()

	d := time.Until(slot)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// CoolDown parks the whole budget for d. Used after a provider reports an
// exceeded quota in the response body (Deezer: HTTP 200, error code 4).
func (t *Throttle) CoolDown(d time.Duration) {
	if t == nil || d <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	until := time.Now().Add(d)
	if until.After(t.cooldown) {
		t.cooldown = until
	}
}

// Ready reports whether a call could go out right now, without consuming a slot.
func (t *Throttle) Ready() bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	return !t.next.After(now) && !t.cooldown.After(now)
}
