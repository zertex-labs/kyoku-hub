package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

const testRGID = "8b8ed50b-ab91-4847-842b-6cc0a7b15659"

func fakeMB(t *testing.T, hits *int32) *mbClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/release-group/"):
			fmt.Fprint(w, `{
			  "id":"8b8ed50b-ab91-4847-842b-6cc0a7b15659",
			  "title":"Raven Sky","primary-type":"Single","first-release-date":"2026-10-15",
			  "artist-credit":[{"name":"Ado","artist":{"id":"e134b52f-2e9e-4734-9bc3-bea9648d1fa1"}}],
			  "releases":[
			    {"id":"99cc17f3-3d4e-4cf7-9569-1a84ac1f9658","date":"2026-10-15","status":"Official",
			     "media":[{"track-count":2}]}
			  ]}`)
		case strings.HasPrefix(r.URL.Path, "/release/782b43b0-e017-45bf-9eca-d1d7f87e9073"):
			fmt.Fprint(w, `{
			  "id":"782b43b0-e017-45bf-9eca-d1d7f87e9073","title":"放生会","date":"2024-05-28","country":"XW","status":"Official",
			  "label-info":[{"label":{"name":"EMI Records"},"catalog-number":"00602465697216"}],
			  "media":[{"track-count":13}]}`)
		case strings.HasPrefix(r.URL.Path, "/isrc/JPPO02603857"):
			fmt.Fprint(w, `{"isrc":"JPPO02603857","recordings":[{"id":"662ec667-2102-41a2-a9e0-3b46da7af447","title":"Raven Sky"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &mbClient{http: srv.Client(), base: srv.URL, ua: "test", spacing: 0}
}

func mbTracks() []downloader.TrackQuality {
	return []downloader.TrackQuality{
		{Number: 1, ISRC: "JPPO02603857"},
		{Number: 2, ISRC: "JPPO02603857"}, // same ISRC twice must cost one lookup
	}
}

func TestResolveMBFillsArtistReleaseAndRecording(t *testing.T) {
	var hits int32
	m := &Manager{mb: fakeMB(t, &hits)}

	info := m.resolveMB(context.Background(), testRGID, "", downloader.AlbumCandidate{Tracks: mbTracks()})

	id := info.recording(context.Background(), "JPPO02603857", 3*time.Second)
	if id != "662ec667-2102-41a2-a9e0-3b46da7af447" {
		t.Fatalf("recording = %q", id)
	}
	if got := info.artistID(); got != "e134b52f-2e9e-4734-9bc3-bea9648d1fa1" {
		t.Errorf("artist id = %q", got)
	}
	if got := info.releaseID(); got != "99cc17f3-3d4e-4cf7-9569-1a84ac1f9658" {
		t.Errorf("release id = %q", got)
	}
	// One group lookup plus one ISRC lookup, not one per track.
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
}

// The budget must be a budget: an album with no ISRCs, or a disabled client,
// must not sit out the per-track wait before every tag.
func TestRecordingReturnsImmediatelyWhenNothingIsPending(t *testing.T) {
	info := &mbInfo{rgid: testRGID, rec: map[string]string{}}
	start := time.Now()
	if got := info.recording(context.Background(), "JPPO00000000", 2*time.Second); got != "" {
		t.Fatalf("got %q", got)
	}
	if d := time.Since(start); d > 150*time.Millisecond {
		t.Fatalf("waited %s with nothing pending", d)
	}
}

func TestResolveMBSkipsIdsThatAreNotUUIDs(t *testing.T) {
	var hits int32
	m := &Manager{mb: fakeMB(t, &hits)}

	info := m.resolveMB(context.Background(), "rg-1", "", downloader.AlbumCandidate{Tracks: mbTracks()})
	info.recording(context.Background(), "JPPO02603857", 50*time.Millisecond)

	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("made %d requests for a non-uuid group id", n)
	}
}

func TestResolveMBWithoutAClientIsInert(t *testing.T) {
	m := &Manager{}
	info := m.resolveMB(context.Background(), testRGID, "", downloader.AlbumCandidate{Tracks: mbTracks()})
	start := time.Now()
	if got := info.recording(context.Background(), "JPPO02603857", 2*time.Second); got != "" {
		t.Fatalf("got %q with no client", got)
	}
	if d := time.Since(start); d > 150*time.Millisecond {
		t.Fatalf("waited %s with no client", d)
	}
}

func TestPickReleaseOnlyWhenUnambiguous(t *testing.T) {
	one := []mbRelease{{
		ID: "a", Status: "Official", Media: []mbMediaOpt{{TrackCount: 12}},
	}}
	if got := pickRelease(one, 12); got != "a" {
		t.Errorf("single match = %q", got)
	}

	two := []mbRelease{
		{ID: "a", Status: "Official", Media: []mbMediaOpt{{TrackCount: 12}}},
		{ID: "b", Status: "Official", Media: []mbMediaOpt{{TrackCount: 12}}},
	}
	if got := pickRelease(two, 12); got != "" {
		t.Errorf("two candidates should not pick: %q", got)
	}

	wrongCount := []mbRelease{{ID: "a", Status: "Official", Media: []mbMediaOpt{{TrackCount: 9}}}}
	if got := pickRelease(wrongCount, 12); got != "" {
		t.Errorf("track count mismatch picked %q", got)
	}

	bootleg := []mbRelease{{ID: "a", Status: "Bootleg", Media: []mbMediaOpt{{TrackCount: 12}}}}
	if got := pickRelease(bootleg, 12); got != "" {
		t.Errorf("bootleg picked %q", got)
	}

	multiDisc := []mbRelease{{ID: "a", Status: "Official", Media: []mbMediaOpt{
		{TrackCount: 6}, {TrackCount: 6},
	}}}
	if got := pickRelease(multiDisc, 12); got != "a" {
		t.Errorf("multi-disc total = %q", got)
	}
}

// A chosen release is not a guess: it supplies the title, the date and the label,
// and the provider's licensed title is exactly what it overrides - Deezer calls
// 放生会 "Carnival".
func TestChosenReleaseOwnsTheMetadata(t *testing.T) {
	var hits int32
	m := &Manager{mb: fakeMB(t, &hits)}

	info := m.resolveMB(context.Background(),
		testRGID, "782b43b0-e017-45bf-9eca-d1d7f87e9073",
		downloader.AlbumCandidate{Tracks: mbTracks()})

	// The lookups run in the background; this blocks until they finish.
	info.recording(context.Background(), "JPPO02603857", 3*time.Second)

	if got := info.releaseID(); got != "782b43b0-e017-45bf-9eca-d1d7f87e9073" {
		t.Fatalf("release id = %q, want the chosen one", got)
	}
	if got := info.albumTitle(); got != "放生会" {
		t.Errorf("title = %q", got)
	}
	if got := info.albumDate(); got != "2024-05-28" {
		t.Errorf("date = %q", got)
	}
	if got := info.albumLabel(); got != "EMI Records" {
		t.Errorf("label = %q", got)
	}
}

// Without a choice, the old behaviour stands: claim a release only when the
// track count singles one out.
func TestNoChosenReleaseStillGuesses(t *testing.T) {
	var hits int32
	m := &Manager{mb: fakeMB(t, &hits)}

	info := m.resolveMB(context.Background(), testRGID, "",
		downloader.AlbumCandidate{Tracks: mbTracks()})

	// Blocks until the background group/ISRC lookups are done.
	info.recording(context.Background(), "JPPO02603857", 3*time.Second)

	if got := info.releaseID(); got != "99cc17f3-3d4e-4cf7-9569-1a84ac1f9658" {
		t.Fatalf("release id = %q", got)
	}
	if got := info.albumTitle(); got != "" {
		t.Errorf("title = %q, want empty (nothing was chosen)", got)
	}
}
