package fsagent

import (
	"context"
	"testing"
	"time"

	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
)

// A reply that carries a request id belongs to the caller waiting for it, and
// must not be treated as another unsolicited event.
func TestResolveDeliversToTheCaller(t *testing.T) {
	ch := make(chan *fspb.Envelope, 1)
	c := &Conn{pending: map[string]chan *fspb.Envelope{"req-1": ch}}

	reply := &fspb.Envelope{RequestId: "req-1"}
	if !c.resolve(reply) {
		t.Fatal("resolve reported no caller for a pending request")
	}
	select {
	case got := <-ch:
		if got.GetRequestId() != "req-1" {
			t.Errorf("delivered %q", got.GetRequestId())
		}
	default:
		t.Fatal("nothing was delivered")
	}

	// The entry is consumed: a duplicate reply must not panic on a closed channel.
	if c.resolve(reply) {
		t.Error("the same request was resolved twice")
	}
}

// One-way messages have no id and nobody waiting.
func TestResolveIgnoresUnsolicitedMessages(t *testing.T) {
	c := &Conn{pending: map[string]chan *fspb.Envelope{}}
	if c.resolve(&fspb.Envelope{}) {
		t.Error("an envelope without a request id was treated as a reply")
	}
}

// A reply whose caller already timed out is dropped rather than blocking the
// reader for the rest of the connection's life.
func TestResolveDropsRepliesNobodyWaitsFor(t *testing.T) {
	c := &Conn{pending: map[string]chan *fspb.Envelope{}}
	if c.resolve(&fspb.Envelope{RequestId: "gone"}) {
		t.Error("an unmatched reply was claimed")
	}
}

// A slow agent must fail the call, not hang the page that asked.
func TestWaitReplyTimesOut(t *testing.T) {
	_, err := waitReply(t.Context(), make(chan *fspb.Envelope, 1), 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout")
	}
}

// And a cancelled page must stop waiting immediately.
func TestWaitReplyHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := waitReply(ctx, make(chan *fspb.Envelope, 1), time.Second); err == nil {
		t.Fatal("expected cancellation")
	}
}
