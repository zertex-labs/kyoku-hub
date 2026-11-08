package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/tagwriter"
	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"

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

// TrackTicket is what the hub now asks for: where the bytes are, not the bytes.
func (f *fakeSource) TrackTicket(ctx context.Context, albumID, trackID string) (downloader.TrackTicket, error) {
	if err := ctx.Err(); err != nil {
		return downloader.TrackTicket{}, err
	}
	if err := f.trackErrs[trackID]; err != nil {
		return downloader.TrackTicket{}, err
	}
	return downloader.TrackTicket{
		URL:     "https://cdn.invalid/" + trackID,
		Headers: map[string]string{"User-Agent": "test"},
		Quality: downloader.DeezerFLAC,
	}, nil
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

// ---- fake agent ------------------------------------------------------------

// fakeFiler stands in for an fs agent: it records the manifest the hub produced
// and then does what an agent would do with it, so the assertions below are about
// the hub's decisions — tags, paths, tickets — rather than about file handling,
// which the agent has its own tests for.
type fakeFiler struct {
	album  *fspb.WriteAlbum
	events []*fspb.WriteEvent
	fail   error
	root   string
}

func (f *fakeFiler) MakeAlbum(ctx context.Context, userID, agentID string, album *fspb.WriteAlbum, onEvent func(*fspb.WriteEvent)) error {
	f.album = album
	if f.fail != nil {
		return f.fail
	}

	dir := filepath.Join(f.root, filepath.FromSlash(album.LibraryRel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, tr := range album.Tracks {
		dest := filepath.Join(dir, tr.DestRel)
		if err := writeFakeAudio(dest); err != nil {
			return err
		}
		tags := make([]tagwriter.Tag, 0, len(tr.Tags))
		for _, tag := range tr.Tags {
			tags = append(tags, tagwriter.Tag{Key: tag.Key, Values: tag.Values})
		}
		if len(tags) > 0 {
			if err := tagwriter.WriteTags(dest, tags); err != nil {
				return err
			}
		}
		ev := &fspb.WriteEvent{
			JobId: album.JobId, Phase: "tagging",
			Done: int32(i + 1), Total: int32(len(album.Tracks)),
			Message: fmt.Sprintf("t%02d done · %s", tr.Track, tr.DestRel),
		}
		f.events = append(f.events, ev)
		onEvent(ev)
	}

	done := &fspb.WriteEvent{
		JobId: album.JobId, Phase: "done",
		Done: int32(len(album.Tracks)), Total: int32(len(album.Tracks)),
		Dir: dir, Result: "ok",
	}
	f.events = append(f.events, done)
	onEvent(done)
	return nil
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
	m.SetFiler(&fakeFiler{root: root})
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
	if !logHas(done, "t01 done") {
		t.Errorf("per-track report missing: %v", logTexts(done))
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

// A ticket the provider will not hand out ends the job with the reason. There is
// no per-track decision any more: the hub cannot tell a dead track from a dead
// account without the agent having tried, and a half-written album is worse than
// a clear failure.
func TestTrackTicketFailureEndsTheJob(t *testing.T) {
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
	failed := waitFor(t, m, job.ID, func(j JobView) bool { return j.Phase == PhaseFailed })

	if !strings.Contains(failed.Result, "404 from cdn") {
		t.Errorf("result = %q, want the provider's reason", failed.Result)
	}
	if !strings.Contains(failed.Result, "One More Time") {
		t.Errorf("result = %q, want the track named", failed.Result)
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

// A download started from a search result often carries no year; the folder must
// still be dated, because the edition knows when it came out.
func TestAlbumYearFallsBackToMusicBrainz(t *testing.T) {
	m := NewManager(t.TempDir(), nil, nil, nil, nil)

	fromRequest := &Job{JobView: JobView{Album: AlbumView{Year: "2021"}}}
	if y := m.albumYear(fromRequest, &mbInfo{}); y != 2021 {
		t.Errorf("request year = %d", y)
	}

	edition := &mbInfo{relDate: "2023-05-02"}
	empty := &Job{}
	if y := m.albumYear(empty, edition); y != 2023 {
		t.Errorf("edition year = %d", y)
	}

	groupOnly := &mbInfo{groupFirst: "1999-08-10"}
	if y := m.albumYear(empty, groupOnly); y != 1999 {
		t.Errorf("group year = %d", y)
	}

	// Nothing known: no year, rather than a made-up one.
	if y := m.albumYear(empty, &mbInfo{}); y != 0 {
		t.Errorf("unknown year = %d, want 0", y)
	}
}
