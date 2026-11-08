package tagwriter

import (
	"context"
	"errors"
	"testing"
)

type recorder struct {
	name string
	err  error
	seen *[]string
}

func (r recorder) Name() string { return r.name }

func (r recorder) Process(ctx context.Context, a *Artifact) error {
	if r.seen != nil {
		*r.seen = append(*r.seen, r.name)
	}
	return r.err
}

func TestChainRunsInOrderAndSurvivesFailures(t *testing.T) {
	var seen []string
	chain := New(
		recorder{name: "a", seen: &seen},
		recorder{name: "b", seen: &seen, err: errors.New("boom")},
		recorder{name: "c", seen: &seen, err: Skip("not for this format")},
		recorder{name: "d", seen: &seen},
	)

	out := chain.Run(context.Background(), &Artifact{})
	if len(out) != 4 {
		t.Fatalf("outcomes = %d", len(out))
	}
	if len(seen) != 4 {
		t.Fatalf("a failure stopped the chain: ran %v", seen)
	}
	if out[1].Name != "b" || !out[1].Failed() {
		t.Errorf("b should have failed: %+v", out[1])
	}
	if reason, ok := out[2].Skipped(); !ok || reason != "not for this format" {
		t.Errorf("c should have been skipped: %+v", out[2])
	}
	if out[3].Err != nil {
		t.Errorf("d should have run clean: %+v", out[3])
	}
}

func TestChainStopsWhenTheJobIsCancelled(t *testing.T) {
	var seen []string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chain := New(recorder{name: "a", seen: &seen}, recorder{name: "b", seen: &seen})
	out := chain.Run(ctx, &Artifact{})
	if len(out) != 1 {
		t.Fatalf("ran after cancellation: %d outcomes", len(out))
	}
	if len(seen) != 0 {
		t.Fatalf("processors ran after cancellation: %v", seen)
	}
}

func TestDefaultChainIsTagsThenCover(t *testing.T) {
	names := Default().Names()
	if len(names) != 2 || names[0] != "tags" || names[1] != "cover" {
		t.Fatalf("default chain = %v", names)
	}
}

func TestSkipIsNotFailure(t *testing.T) {
	out := Outcome{Name: "x", Err: Skip("nope")}
	if out.Failed() {
		t.Error("skip reported as failure")
	}
	if reason, ok := out.Skipped(); !ok || reason != "nope" {
		t.Errorf("reason = %q, skipped = %v", reason, ok)
	}
}
