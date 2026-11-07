package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// ---- fake source -----------------------------------------------------------

type fakeSource struct {
	cand      downloader.AlbumCandidate
	search    []downloader.SearchResult
	probeErr  error
	trackErrs map[string]error
	written   []string
}

func (f *fakeSource) Name() string                     { return "deezer" }
func (f *fakeSource) Ceiling() downloader.AudioQuality { return downloader.DeezerFLAC }

func (f *fakeSource) SearchAlbums(ctx context.Context, q string) ([]downloader.SearchResult, error) {
	return f.search, nil
}

func (f *fakeSource) ProbeAlbum(ctx context.Context, albumID string, ref downloader.AlbumRef, targets []downloader.QualityTarget) (downloader.AlbumCandidate, error) {
	if f.probeErr != nil {
		return downloader.AlbumCandidate{}, f.probeErr
	}
	return f.cand, nil
}

func (f *fakeSource) DownloadTrack(ctx context.Context, albumID, trackID, dest string) (downloader.AudioQuality, error) {
	if err := ctx.Err(); err != nil {
		return downloader.UnknownQuality, err
	}
	if err := f.trackErrs[trackID]; err != nil {
		return downloader.UnknownQuality, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return downloader.UnknownQuality, err
	}
	if err := writeFakeAudio(dest); err != nil {
		return downloader.UnknownQuality, err
	}
	f.written = append(f.written, dest)
	return downloader.DeezerFLAC, nil
}

// writeFakeAudio writes a minimal file that the real verification accepts for
// the requested container, so tests exercise the actual checks rather than a stub.
func writeFakeAudio(path string) error {
	if strings.HasSuffix(strings.ToLower(path), ".mp3") {
		buf := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), make([]byte, MinTrackBytes+1024)...)
		return os.WriteFile(path, buf, 0o644)
	}
	hdr := []byte{'f', 'L', 'a', 'C', 0x80, 0x00, 0x00, 0x22} // magic + last STREAMINFO block
	body := make([]byte, 34)
	frames := []byte{0xFF, 0xF8, 0x69, 0x18, 0x00, 0x00} // frame sync + a plausible header
	pad := make([]byte, MinTrackBytes+1024)
	buf := append(append(append(hdr, body...), frames...), pad...)
	return os.WriteFile(path, buf, 0o644)
}

func fakeCandidate(tracks ...downloader.TrackQuality) downloader.AlbumCandidate {
	c := downloader.AlbumCandidate{
		Provider:    "deezer",
		ProviderID:  "302127",
		Title:       "Discovery",
		Artist:      "Daft Punk",
		Year:        2001,
		ReleaseDate: "2001-03-12",
		Genre:       "Electronic",
		Label:       "Virgin",
		Ceiling:     downloader.DeezerFLAC,
		Tracks:      tracks,
	}
	c.ApplyFloor(downloader.DefaultTargets())
	return c
}

func flacTrack(id string, n int, title string) downloader.TrackQuality {
	return downloader.TrackQuality{
		ProviderID: id, Number: n, Disc: 1, Title: title,
		Quality: downloader.DeezerFLAC, Known: true,
		ISRC: "GBDUW0000059",
	}
}

// ---- helpers ---------------------------------------------------------------

func newTestManager(t *testing.T, src *fakeSource) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	m := NewManager(root, func(ctx context.Context, userID string) map[string]downloader.Source {
		return map[string]downloader.Source{"deezer": src}
	}, nil, nil, nil)
	// Unit tests must never touch musicbrainz.org: tags are asserted from the
	// candidate the fake source returns.
	m.mb = nil
	return m, root
}

func startJob(t *testing.T, m *Manager, tracks int) JobView {
	t.Helper()
	job, err := m.Start(context.Background(), "user-1", "rg-1", StartRequest{
		Album: AlbumView{Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: tracks},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return job
}

func waitFor(t *testing.T, m *Manager, id string, cond func(JobView) bool) JobView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		live, ok := m.Get("user-1", id)
		if !ok {
			t.Fatal("job disappeared")
		}
		if snap := live.Snapshot(); cond(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	live, _ := m.Get("user-1", id)
	t.Fatalf("timed out waiting for job state; last snapshot: %+v", live.Snapshot())
	return JobView{}
}

func logTexts(v JobView) []string {
	out := make([]string, 0, len(v.Log))
	for _, l := range v.Log {
		out = append(out, l.Text)
	}
	return out
}

func logHas(v JobView, substr string) bool {
	for _, l := range v.Log {
		if strings.Contains(l.Text, substr) {
			return true
		}
	}
	return false
}

// ---- tests -----------------------------------------------------------------

func TestFitDownloadsSilentlyAndPublishes(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "302127", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 2}},
		cand: fakeCandidate(
			flacTrack("t1", 1, "One More Time"),
			flacTrack("t2", 2, "Aerodynamic"),
		),
	}
	m, root := newTestManager(t, src)

	job := startJob(t, m, 2)
	done := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseDone })

	for _, s := range done.Sources {
		if s.Status != "fit" {
			t.Errorf("source %s status = %s, want fit", s.ID, s.Status)
		}
	}
	if len(done.Candidates) != 0 {
		t.Errorf("a fit must not produce candidates, got %d", len(done.Candidates))
	}

	albumDir := filepath.Join(root, "Daft Punk", "Discovery (2001)")
	for i, name := range []string{"01 - One More Time.flac", "02 - Aerodynamic.flac"} {
		path := filepath.Join(albumDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected published file %s: %v", name, err)
		}
		// The release-group tag is what the fs agent matches on; without it the
		// file lands in the right folder and the library still says "missing".
		tags := readVorbis(t, path)
		if tags["MUSICBRAINZ_RELEASEGROUPID"] != "rg-1" {
			t.Errorf("%s: release group tag = %q, want rg-1", name, tags["MUSICBRAINZ_RELEASEGROUPID"])
		}
		if tags["ALBUM"] != "Discovery" || tags["TRACKNUMBER"] != strconv.Itoa(i+1) {
			t.Errorf("%s: unexpected tags %v", name, tags)
		}
		// Metadata that used to be dropped on the floor: the provider knows it at
		// probe time, so a published file should carry it without a second lookup.
		if tags["GENRE"] != "Electronic" {
			t.Errorf("%s: genre = %q", name, tags["GENRE"])
		}
		if tags["ISRC"] != "GBDUW0000059" {
			t.Errorf("%s: isrc = %q", name, tags["ISRC"])
		}
		if tags["DATE"] == "" {
			t.Errorf("%s: no date", name)
		}
		if tags["LABEL"] != "Virgin" {
			t.Errorf("%s: label = %q", name, tags["LABEL"])
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".staging", job.ID)); !os.IsNotExist(err) {
		t.Error("staging dir should be gone after publish")
	}

	// The log is the only place a user sees what happened to the tags and that
	// the run is over, so both must be in it.
	if !logHas(done, "Done.") {
		t.Errorf("no final Done. line in %v", logTexts(done))
	}
	if !logHas(done, "tags ·") {
		t.Errorf("no tag report in %v", logTexts(done))
	}
	if !logHas(done, "t01 tagged · ISRC GBDUW0000059") {
		t.Errorf("per-track tag report missing: %v", logTexts(done))
	}
	if !logHas(done, "cover · none") {
		t.Errorf("cover line missing: %v", logTexts(done))
	}
}

func TestPartialEndsInResultsAndPickDownloads(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "1", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 2}},
		cand: fakeCandidate(
			flacTrack("t1", 1, "One More Time"),
			downloader.TrackQuality{
				ProviderID: "t2", Number: 2, Disc: 1, Title: "Aerodynamic",
				Quality: downloader.AudioQuality{Format: "mp3", Bitrate: 320}, Known: true,
			},
		),
	}
	m, root := newTestManager(t, src)

	job := startJob(t, m, 2)
	res := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseResults })

	if len(res.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.Summary != "2/2 songs · floor MP3 320" {
		t.Errorf("summary = %q", c.Summary)
	}
	if len(c.Tracks) != 2 || c.Tracks[1].OK {
		t.Errorf("per-track detail wrong: %+v", c.Tracks)
	}
	if len(c.Deltas) == 0 {
		t.Error("partial candidate should carry deltas")
	}
	for _, s := range res.Sources {
		if s.Status != "partial" {
			t.Errorf("source status = %s, want partial", s.Status)
		}
	}

	// Picking resumes the download.
	if err := m.Decide("user-1", job.ID, "use:deezer"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	done := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseDone })
	if !strings.Contains(done.Result, "2/2") {
		t.Errorf("result = %q, want 2/2 tracks", done.Result)
	}
	// The extension follows the quality actually delivered per track, not the
	// album's headline: this one was a 320, so it lands as .mp3 beside the FLACs.
	if _, err := os.Stat(filepath.Join(root, "Daft Punk", "Discovery (2001)", "02 - Aerodynamic.mp3")); err != nil {
		t.Errorf("second track missing: %v", err)
	}
}

func TestMissSkipsTheSource(t *testing.T) {
	src := &fakeSource{search: nil} // nothing found
	m, _ := newTestManager(t, src)

	job := startJob(t, m, 2)
	res := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseResults })

	if len(res.Candidates) != 0 {
		t.Errorf("miss must not create candidates, got %d", len(res.Candidates))
	}
	for _, s := range res.Sources {
		if s.Status != "miss" {
			t.Errorf("source status = %s, want miss", s.Status)
		}
		if !strings.Contains(s.Reason, "no matching album") {
			t.Errorf("reason = %q", s.Reason)
		}
	}
}

func TestTrackFailureAsksThenSkips(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "1", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 2}},
		cand: fakeCandidate(
			flacTrack("t1", 1, "One More Time"),
			flacTrack("t2", 2, "Aerodynamic"),
		),
		trackErrs: map[string]error{"t1": errors.New("404 from cdn")},
	}
	m, _ := newTestManager(t, src)

	job := startJob(t, m, 2)
	waiting := waitFor(t, m, job.ID, func(j JobView) bool { return j.Decision != nil })

	if waiting.Decision.Kind != "track_failed" {
		t.Fatalf("decision kind = %s", waiting.Decision.Kind)
	}
	if len(waiting.Decision.Options) < 2 {
		t.Fatalf("expected skip/retry/cancel, got %+v", waiting.Decision.Options)
	}

	if err := m.Decide("user-1", job.ID, "skip"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	done := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseDone })
	if !strings.Contains(done.Result, "1/2") {
		t.Errorf("result = %q, want 1/2 tracks (one skipped)", done.Result)
	}
}

func TestDedupeReturnsTheRunningJob(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "1", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 1}},
		cand:   fakeCandidate(flacTrack("t1", 1, "One More Time")),
	}
	m, _ := newTestManager(t, src)

	first := startJob(t, m, 1)
	second := startJob(t, m, 1)
	if first.ID != second.ID {
		t.Errorf("second start created a new job (%s vs %s); same user + same rg must dedupe", first.ID, second.ID)
	}
	// Let the background run finish before TempDir cleanup deletes its staging.
	waitFor(t, m, first.ID, func(j JobView) bool { return j.Phase == PhaseDone || j.Phase == PhaseFailed })
}

func readVorbis(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]string{}
	for _, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*m)
		if err != nil {
			t.Fatalf("comments %s: %v", path, err)
		}
		for _, c := range cmt.Comments {
			k, v, ok := strings.Cut(c, "=")
			if ok {
				out[strings.ToUpper(k)] = v
			}
		}
	}
	return out
}

// ---- plugins ---------------------------------------------------------------

// fakeDispatcher stands in for the plugin registry: the pipeline's contract is
// "call the dispatcher, publish whatever files it added, block only when a
// required plugin failed". The real registry (processes, timeouts, payload JSON)
// is covered by its own package.
type fakeDispatcher struct {
	add      func(dir string)
	err      bool
	required bool
	code     int
	called   []Hook
}

func (f *fakeDispatcher) Dispatch(_ context.Context, hook Hook, p Payload) []Outcome {
	f.called = append(f.called, hook)
	// Only on the album hook: files written during a track hook are already in
	// staging when publish snapshots it, so they would not register as "added".
	if f.add != nil && hook == HookAlbumBeforePublish {
		f.add(p.Album.Dir)
	}
	if !f.err {
		return []Outcome{{Plugin: "fake", Hook: string(hook)}}
	}
	return []Outcome{{
		Plugin: "fake", Hook: string(hook), Err: errors.New("boom"), Stderr: "storage unreachable",
	}}
}

func (f *fakeDispatcher) Required(string) bool { return f.required }
func (f *fakeDispatcher) Names() []string      { return []string{"fake"} }
func (f *fakeDispatcher) Count() int           { return 1 }

// A file a plugin writes during album.beforePublish must reach the library, not
// die with the staging directory.
func TestPluginAddedFilesArePublished(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "302127", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 1}},
		cand:   fakeCandidate(flacTrack("t1", 1, "One More Time")),
	}
	m, root := newTestManager(t, src)
	m.SetDispatcher(&fakeDispatcher{add: func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "lyrics.lrc"), []byte("hi"), 0o644)
	}})

	job := startJob(t, m, 1)
	done := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseDone })

	if _, err := os.Stat(filepath.Join(root, "Daft Punk", "Discovery (2001)", "lyrics.lrc")); err != nil {
		t.Fatalf("plugin file was not published: %v", err)
	}
	if !logHas(done, "plugins added · lyrics.lrc") {
		t.Errorf("added file not reported: %v", logTexts(done))
	}
}

// required: true turns a plugin failure into a blocked publish, so a library is
// never published without the step the user declared mandatory.
func TestRequiredPluginFailureBlocksPublish(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "302127", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 1}},
		cand:   fakeCandidate(flacTrack("t1", 1, "One More Time")),
	}
	m, root := newTestManager(t, src)
	m.SetDispatcher(&fakeDispatcher{err: true, required: true})

	job := startJob(t, m, 1)
	done := waitFor(t, m, job.ID, func(j JobView) bool {
		return j.Phase == PhaseFailed || j.Phase == PhaseDone
	})

	if done.Phase != PhaseFailed {
		t.Fatalf("phase = %s, want failed", done.Phase)
	}
	if _, err := os.Stat(filepath.Join(root, "Daft Punk", "Discovery (2001)")); !os.IsNotExist(err) {
		t.Error("album was published despite a required plugin failing")
	}
	if !logHas(done, "storage unreachable") {
		t.Errorf("plugin stderr missing from log: %v", logTexts(done))
	}
}

// A failing optional plugin is a warning, and the album still lands.
func TestOptionalPluginFailureStillPublishes(t *testing.T) {
	src := &fakeSource{
		search: []downloader.SearchResult{{ID: "302127", Title: "Discovery", Artist: "Daft Punk", Year: "2001", TrackCount: 1}},
		cand:   fakeCandidate(flacTrack("t1", 1, "One More Time")),
	}
	m, root := newTestManager(t, src)
	m.SetDispatcher(&fakeDispatcher{err: true})

	job := startJob(t, m, 1)
	done := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseDone })

	if _, err := os.Stat(filepath.Join(root, "Daft Punk", "Discovery (2001)", "01 - One More Time.flac")); err != nil {
		t.Fatalf("album not published: %v", err)
	}
	if !logHas(done, "fake · track.tagged failed") {
		t.Errorf("failure not reported: %v", logTexts(done))
	}
}
