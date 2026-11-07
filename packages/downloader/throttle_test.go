package downloader

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestThrottleSpacesSequentialCalls(t *testing.T) {
	th := NewThrottle(20) // 50ms apart
	ctx := context.Background()

	start := time.Now()
	if err := th.Wait(ctx); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	if err := th.Wait(ctx); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("two calls went out %v apart, want >= ~50ms", elapsed)
	}
}

func TestThrottleGivesConcurrentCallersDifferentSlots(t *testing.T) {
	// The whole point: two goroutines arriving together must not both compute
	// "no wait" and fire in the same instant.
	th := NewThrottle(20)
	ctx := context.Background()

	var (
		mu    sync.Mutex
		times []time.Time
		wg    sync.WaitGroup
	)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = th.Wait(ctx)
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(times) != 3 {
		t.Fatalf("got %d slots, want 3", len(times))
	}
	earliest, latest := times[0], times[0]
	for _, ts := range times {
		if ts.Before(earliest) {
			earliest = ts
		}
		if ts.After(latest) {
			latest = ts
		}
	}
	if spread := latest.Sub(earliest); spread < 80*time.Millisecond {
		t.Errorf("3 concurrent calls spanned %v, want >= ~100ms (they shared a slot)", spread)
	}
}

func TestThrottleCooldownParksTheBudget(t *testing.T) {
	th := NewThrottle(1000) // effectively no interval
	if err := th.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	th.CoolDown(80 * time.Millisecond)

	start := time.Now()
	if err := th.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("call after cooldown went out after %v, want >= ~80ms", elapsed)
	}
}

func TestThrottleHonoursContextCancellation(t *testing.T) {
	th := NewThrottle(1) // 1s apart
	_ = th.Wait(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := th.Wait(ctx); err == nil {
		t.Fatal("expected context error while parked behind a slot")
	}
}

func TestThrottleReady(t *testing.T) {
	th := NewThrottle(2) // 500ms apart
	if !th.Ready() {
		t.Error("fresh throttle should be ready")
	}
	_ = th.Wait(context.Background())
	if th.Ready() {
		t.Error("throttle should not be ready right after taking a slot")
	}
}
