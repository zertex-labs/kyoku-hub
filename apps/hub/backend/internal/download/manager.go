package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download/postprocess"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// SourceResolver is how the manager gets the caller's configured sources. It is
// a function, not a map, because credentials are per user and a source instance
// carries that user's session.
type SourceResolver func(ctx context.Context, userID string) map[string]downloader.Source

// JobStore persists finished/running job snapshots. Optional: the in-memory
// registry is authoritative for a running job, the store is the record.
type JobStore interface {
	Save(ctx context.Context, userID string, job JobView) error
}

// Manager owns every download job: the walk, the decisions, the per-track loop,
// staging and publish.
type Manager struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	byRG     map[string]string // userID \x00 rgID -> job id, for dedupe
	sse      *sse.Manager
	logger   *zap.Logger
	root     string // library root; staging lives inside it so publish is a rename
	sources  SourceResolver
	store    JobStore
	client   *http.Client
	mb       *mbClient          // MusicBrainz lookups, rate limited process-wide
	post     *postprocess.Chain // post-download steps, in order
	dispatch Dispatcher         // user-installed plugins; nil means none

	// OnPublish is called after a batch lands in the library, so the caller can
	// ask the right fs agent to rescan. Optional; nil-safe.
	OnPublish func(userID, agentID string)
}

func NewManager(
	root string,
	sources SourceResolver,
	sseManager *sse.Manager,
	store JobStore,
	logger *zap.Logger,
) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		jobs:    make(map[string]*Job),
		byRG:    make(map[string]string),
		sse:     sseManager,
		logger:  logger,
		root:    root,
		sources: sources,
		store:   store,
		client:  &http.Client{Timeout: 20 * time.Second},
		mb:      newMBClient(),
		post:    postprocess.Default(),
	}
}

// StartRequest is what the download page posts after the user picks a release.
type StartRequest struct {
	Album   AlbumView `json:"album"`
	AgentID string    `json:"agent_id,omitempty"`
	Mode    string    `json:"mode,omitempty"`  // order | best
	Chain   []string  `json:"chain,omitempty"` // user's source order; rest appended

	// Preferred* is the album the user actually clicked on the download page.
	// When the walk reaches that provider it probes this id directly instead of
	// searching first — the click already answered the question.
	PreferredProvider string `json:"preferred_provider,omitempty"`
	PreferredAlbumID  string `json:"preferred_album_id,omitempty"`
}

// Start creates a job and begins the walk. The returned job is a snapshot.
func (m *Manager) Start(ctx context.Context, userID, rgID string, req StartRequest) (JobView, error) {
	if strings.TrimSpace(req.Album.Title) == "" {
		return JobView{}, errors.New("missing album title")
	}
	sources := m.sources(ctx, userID)
	if len(sources) == 0 {
		return JobView{}, errors.New("no download sources configured")
	}

	key := userID + "\x00" + rgID
	m.mu.Lock()
	if id, ok := m.byRG[key]; ok {
		if existing := m.jobs[id]; existing != nil {
			phase := existing.Snapshot().Phase
			if phase == PhaseWalking || phase == PhaseDownloading || phase == PhaseResults {
				m.mu.Unlock()
				return existing.Snapshot(), nil // dedupe: same user, same release
			}
		}
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &Job{
		JobView: JobView{
			ID:      uuid.NewString(),
			RGID:    rgID,
			Phase:   PhaseWalking,
			Mode:    orDefault(req.Mode, "order"),
			Album:   req.Album,
			Sources: make([]SourceView, 0, len(sources)),
		},
		bySource:  make(map[string]sourcePick),
		decide:    make(chan string, 1),
		cancel:    cancel,
		ctx:       jobCtx,
		userID:    userID,
		agentID:   req.AgentID,
		prefer:    sourceHint{provider: req.PreferredProvider, albumID: req.PreferredAlbumID},
		targets:   downloader.DefaultTargets(),
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}
	job.ref = downloader.AlbumRef{
		Title:      req.Album.Title,
		Artist:     req.Album.Artist,
		Year:       atoi(req.Album.Year),
		TrackCount: req.Album.TrackCount,
	}

	chain := orderedChain(req.Chain, sources)
	for _, name := range chain {
		job.Sources = append(job.Sources, SourceView{
			ID:      name,
			Name:    displayName(name),
			Status:  "pending",
			Ceiling: sources[name].Ceiling().ShortLabel(),
		})
	}

	m.jobs[job.ID] = job
	m.byRG[key] = job.ID
	m.mu.Unlock()

	m.broadcast(job)
	m.persist(job)

	go m.run(jobCtx, job, chain, sources)
	return job.Snapshot(), nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func displayName(id string) string {
	if id == "" {
		return id
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func orderedChain(preferred []string, sources map[string]downloader.Source) []string {
	seen := make(map[string]bool, len(sources))
	out := make([]string, 0, len(sources))
	for _, name := range preferred {
		if _, ok := sources[name]; ok && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(sources))
	for name := range sources {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// run walks the chain: search per source, probe the album it returns, classify,
// and either take a FIT or collect PARTIALs for the results view.
func (m *Manager) run(ctx context.Context, job *Job, chain []string, sources map[string]downloader.Source) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("download job panicked", zap.Any("panic", r), zap.String("job", job.ID))
			m.fail(job, "internal error")
		}
	}()

	job.log("info", fmt.Sprintf("walk started · %s · %d source(s)", modeLabel(job.Mode), len(chain)))
	job.log("info", fmt.Sprintf("album: %s — %s (%s), %d tracks",
		job.Album.Title, job.Album.Artist, job.Album.Year, job.Album.TrackCount))
	m.broadcast(job)

	query := strings.TrimSpace(job.ref.Title + " " + job.ref.Artist)

	for _, name := range chain {
		if ctx.Err() != nil {
			m.fail(job, "cancelled")
			return
		}
		src := sources[name]
		if src == nil {
			continue
		}

		job.setSource(name, "running", "", "")
		m.broadcast(job)

		pick, verdict, reasons, failed := m.probeSource(ctx, job, name, src, query)
		if failed {
			job.setSource(name, "failed", firstReason(reasons), "")
			job.log("error", fmt.Sprintf("%s: %s", name, firstReason(reasons)))
			m.broadcast(job)
			continue
		}

		switch verdict {
		case downloader.VerdictFit:
			summary := pick.cand.Summary(job.targets)
			job.setSource(name, "fit", "", summary)
			job.log("ok", fmt.Sprintf("%s: fit — %s", name, summary))
			job.log("info", fmt.Sprintf("walk stopped at %s (first fit)", name))
			m.broadcast(job)
			m.download(ctx, job, pick)
			return

		case downloader.VerdictPartial:
			summary := pick.cand.Summary(job.targets)
			job.setSource(name, "partial", strings.Join(reasons, "; "), summary)
			job.log("warn", fmt.Sprintf("%s: partial — %s", name, summary))
			for _, r := range reasons {
				job.log("warn", "  · "+r)
			}
			m.bySourceAdd(job, name, pick, reasons)
			m.broadcast(job)
			continue

		default: // miss
			job.setSource(name, "miss", firstReason(reasons), "")
			job.log("warn", fmt.Sprintf("%s: skipped — %s", name, firstReason(reasons)))
			m.broadcast(job)
			continue
		}
	}

	m.results(job)
}

// probeSource resolves the release group on one source and classifies it.
func (m *Manager) probeSource(
	ctx context.Context,
	job *Job,
	name string,
	src downloader.Source,
	query string,
) (sourcePick, downloader.Verdict, []string, bool) {
	albumID := ""
	if job.prefer.provider == name && job.prefer.albumID != "" {
		albumID = job.prefer.albumID // the user already picked this one
	}
	if albumID == "" {
		results, err := src.SearchAlbums(ctx, query)
		if err != nil {
			return sourcePick{}, downloader.VerdictMiss,
				[]string{fmt.Sprintf("search failed: %v", err)}, true
		}
		albumID = bestMatch(results, job.ref)
		if albumID == "" {
			return sourcePick{}, downloader.VerdictMiss,
				[]string{"no matching album (searched: " + query + ")"}, false
		}
	}

	cand, err := src.ProbeAlbum(ctx, albumID, job.ref, job.targets)
	if err != nil {
		return sourcePick{}, downloader.VerdictMiss,
			[]string{fmt.Sprintf("probe failed: %v", err)}, true
	}

	verdict, reasons := cand.Classify(job.ref, job.targets)
	return sourcePick{source: src, albumID: albumID, cand: cand}, verdict, reasons, false
}

// bestMatch picks the most likely album for a release group, or "" when nothing
// is close enough. Title containment is required: a same-artist album with a
// different name is not a candidate at all.
func bestMatch(results []downloader.SearchResult, ref downloader.AlbumRef) string {
	wantTitle := alnumFold(ref.Title)
	wantArtist := alnumFold(ref.Artist)

	best, bestScore := "", 0
	for _, r := range results {
		title := alnumFold(r.Title)
		if title == "" {
			continue
		}
		score := 0
		switch {
		case title == wantTitle:
			score += 100
		case strings.Contains(title, wantTitle) || strings.Contains(wantTitle, title):
			score += 60
		default:
			continue
		}
		if a := alnumFold(r.Artist); a != "" && wantArtist != "" {
			if strings.Contains(a, wantArtist) || strings.Contains(wantArtist, a) {
				score += 40
			}
		}
		if ref.TrackCount > 0 && r.TrackCount == ref.TrackCount {
			score += 10
		}
		if ref.Year > 0 && strings.HasPrefix(r.Year, strconv.Itoa(ref.Year)) {
			score += 10
		}
		if score > bestScore {
			best, bestScore = r.ID, score
		}
	}
	return best
}

// results ends a walk that produced no FIT.
func (m *Manager) results(job *Job) {
	job.mu.Lock()
	job.Phase = PhaseResults
	n := len(job.Candidates)
	job.mu.Unlock()

	if n == 0 {
		job.log("error", "nothing could satisfy the target")
	} else {
		job.log("info", fmt.Sprintf("waiting for you to pick a release (%d candidate(s))", n))
	}
	m.broadcast(job)
	m.persist(job)
}

// download fetches every track of a picked album into staging, then publishes.
func (m *Manager) download(ctx context.Context, job *Job, pick sourcePick) {
	staging := filepath.Join(m.root, ".staging", job.ID)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		m.fail(job, "cannot create staging dir: "+err.Error())
		return
	}

	// MusicBrainz artwork of the tags, resolved in the background at 1 req/s
	// while the audio downloads. Best-effort: no network, no MB tags.
	mb := m.resolveMB(ctx, job.RGID, job.Album.ReleaseID, pick.cand)
	if err := mb.failures(); err != "" {
		job.log("warn", "musicbrainz: "+err)
	}

	// One cover download for the whole album: embedded in every track and written
	// beside them. Best-effort — missing art must not fail a download.
	cover, err := m.fetchCover(job.Album.Cover)
	if err != nil {
		job.log("warn", "cover art: "+err.Error())
	}
	if err := WriteCover(staging, cover); err != nil {
		job.log("warn", "could not write cover.jpg: "+err.Error())
	}

	for _, line := range metadataLines(job, pick.cand, mb, cover, m.post) {
		job.log("info", line)
	}
	if names := m.pluginNames(); len(names) > 0 {
		job.log("info", "plugins · "+strings.Join(names, ", "))
	}

	tracks := pick.cand.Tracks
	job.mu.Lock()
	job.Phase = PhaseDownloading
	job.Progress = &Progress{Total: len(tracks)}
	job.mu.Unlock()
	m.broadcast(job)

	var (
		written  []string
		totalLen int64
		skips    = map[string]bool{}
	)
	for i := 0; i < len(tracks); i++ {
		tr := tracks[i]
		if ctx.Err() != nil {
			m.fail(job, "cancelled")
			return
		}

		ext := extFor(tr.Quality)
		name := TrackFileName(tr.Disc, tr.Number, tr.Title, ext)
		dest := filepath.Join(staging, name)

		job.setProgress(i, fmt.Sprintf("t%02d %s", tr.Number, tr.Title))
		m.broadcast(job)

		delivered, err := pick.source.DownloadTrack(ctx, pick.albumID, tr.ProviderID, dest)
		if err == nil {
			err = VerifyAudio(dest)
		}
		if err != nil {
			job.log("error", fmt.Sprintf("t%02d %q: %v", tr.Number, tr.Title, err))
			m.broadcast(job)

			switch m.awaitDecision(ctx, job, Decision{
				Kind:  "track_failed",
				Title: fmt.Sprintf("t%02d %q failed", tr.Number, tr.Title),
				Body:  err.Error() + ". Skipping leaves the album incomplete; retrying asks the source again.",
				Options: []DecisionOption{
					{ID: "skip", Label: "Skip track", Tone: "primary"},
					{ID: "retry", Label: "Retry", Tone: "ghost"},
					{ID: "cancel", Label: "Cancel", Tone: "danger"},
				},
			}) {
			case "skip":
				_ = os.Remove(dest)
				job.log("warn", fmt.Sprintf("skipping t%02d — album will be incomplete", tr.Number))
				continue
			case "retry":
				_ = os.Remove(dest)
				i--
				continue
			default:
				m.fail(job, "cancelled")
				return
			}
		}

		// The probe promised something; the transfer is the truth.
		if delivered.Known() && tr.Quality.Known() && delivered.TierScore() < tr.Quality.TierScore() {
			job.log("warn", fmt.Sprintf("t%02d delivered %s, probe said %s",
				tr.Number, delivered.ShortLabel(), tr.Quality.ShortLabel()))
		}

		if m.runHook(ctx, job, HookTrackDownloaded,
			m.payloadFor(job, pick.cand, staging, coverPathFor(staging), &tr, nil), skips) {
			m.fail(job, "a required plugin failed")
			return
		}

		art := m.artifact(job, tr, pick.cand, mb, dest, ext, cover)
		for _, out := range m.post.Run(ctx, &art) {
			switch {
			case out.Err == nil:
			case out.Failed():
				job.log("warn", fmt.Sprintf("%s t%02d: %v", out.Name, tr.Number, out.Err))
			default:
				// A skip is album-wide, not track-wide: say it once.
				if reason, ok := out.Skipped(); ok && !skips[out.Name] {
					skips[out.Name] = true
					job.log("info", fmt.Sprintf("%s · %s (applies to every file here)", out.Name, reason))
				}
			}
		}
		job.log("info", fmt.Sprintf("t%02d tagged · %s", tr.Number, tagSummary(art)))

		if m.runHook(ctx, job, HookTrackTagged,
			m.payloadFor(job, pick.cand, staging, coverPathFor(staging), &tr, nil), skips) {
			m.fail(job, "a required plugin failed")
			return
		}

		if st, err := os.Stat(dest); err == nil {
			totalLen += st.Size()
		}
		written = append(written, name)
		job.setProgress(i+1, fmt.Sprintf("t%02d %s", tr.Number, tr.Title))
		m.broadcast(job)
	}

	if line := mb.summary(); line != "" {
		job.log("info", line)
	}

	m.publish(ctx, job, staging, written, len(tracks), totalLen, pick.cand, skips)
}

func extFor(q downloader.AudioQuality) string {
	if strings.EqualFold(q.Format, "mp3") {
		return ".mp3"
	}
	if strings.EqualFold(q.Format, "aac") || strings.EqualFold(q.Format, "alac") {
		return ".m4a"
	}
	return ".flac"
}

// tagsFor builds the tag set for one track.
// dateFor resolves the DATE tag: what the caller knows (MusicBrainz), then the
// provider's own release date, then its year. Deezer's search results carry no
// release_date at all, which is how a file used to end up with no DATE.
func dateFor(job *Job, cand downloader.AlbumCandidate) string {
	date := job.Album.Year
	if date == "" {
		date = cand.ReleaseDate
	}
	if date == "" && cand.Year > 0 {
		date = strconv.Itoa(cand.Year)
	}
	return date
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// publish moves a complete staged album into the library. A batch that did not
// finish never reaches the library: it stays in .staging so a retry can decide
// what to do with it, and media servers never see half an album.
func (m *Manager) publish(
	ctx context.Context,
	job *Job,
	staging string,
	audio []string,
	total int,
	bytes int64,
	cand downloader.AlbumCandidate,
	skips map[string]bool,
) {
	if len(audio) == 0 {
		m.fail(job, "no tracks were written")
		return
	}

	// The album is complete and still in staging: last chance for a plugin to
	// look at the whole set at once (album gain, cue sheets, lyrics).
	before := stagedFiles(staging)
	if m.runHook(ctx, job, HookAlbumBeforePublish, m.payloadFor(
		job, cand, staging, coverPathFor(staging), nil, before), skips) {
		m.fail(job, "a required plugin failed")
		return
	}
	added := diffFiles(before, stagedFiles(staging))

	destDir := filepath.Join(m.root, AlbumDir(job.Album.Artist, job.Album.Title, atoi(job.Album.Year)))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		m.fail(job, "cannot create album dir: "+err.Error())
		return
	}

	// Move everything that is staged, not just the files we downloaded: artwork
	// and anything a plugin wrote here belongs to the album and would otherwise
	// be left behind when staging is removed.
	isAudio := make(map[string]bool, len(audio))
	for _, name := range audio {
		isAudio[name] = true
	}

	moved := 0
	for _, f := range stagedFiles(staging) {
		name := filepath.Base(f.Path)
		if err := moveFile(f.Path, filepath.Join(destDir, name)); err != nil {
			job.log("error", fmt.Sprintf("publish %s: %v", name, err))
			continue
		}
		if isAudio[name] {
			moved++
		}
	}
	_ = os.Remove(staging)

	job.mu.Lock()
	job.Phase = PhaseDone
	job.Result = fmt.Sprintf("%d/%d tracks · %s", moved, total, humanBytes(bytes))
	job.mu.Unlock()

	if len(added) > 0 {
		names := make([]string, 0, len(added))
		for _, f := range added {
			names = append(names, filepath.Base(f.Path))
		}
		job.log("info", "plugins added · "+strings.Join(names, ", "))
	}

	job.log("ok", fmt.Sprintf("Done. %d/%d files · %s · %s → %s",
		moved, total, cand.Floor.ShortLabel(), humanBytes(bytes), destDir))
	m.broadcast(job)
	m.persist(job)

	m.runHook(ctx, job, HookAlbumPublished, m.payloadFor(
		job, cand, destDir, coverPathFor(destDir), nil, stagedFiles(destDir)), skips)

	if m.OnPublish != nil {
		m.OnPublish(job.userID, job.agentID)
	}
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback: the staging tree is inside the root, but a bind
	// mount can still split them.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Remove(src)
}

func (m *Manager) fetchCover(url string) ([]byte, error) {
	if url == "" {
		return nil, nil
	}
	resp, err := m.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// awaitDecision parks the run until the user answers. A closed tab must not
// leak the goroutine, so it has a hard ceiling; the id is not reused.
func (m *Manager) awaitDecision(ctx context.Context, job *Job, d Decision) string {
	job.mu.Lock()
	job.Decision = &d
	job.updatedAt = time.Now()
	job.mu.Unlock()
	m.broadcast(job)

	select {
	case choice := <-job.decide:
		job.mu.Lock()
		job.Decision = nil
		job.mu.Unlock()
		job.log("info", "you chose: "+choiceLabel(d, choice))
		m.broadcast(job)
		return choice
	case <-ctx.Done():
		return "cancel"
	case <-time.After(30 * time.Minute):
		job.log("warn", "no answer for 30 minutes — cancelling")
		return "cancel"
	}
}

func choiceLabel(d Decision, id string) string {
	for _, o := range d.Options {
		if o.ID == id {
			return o.Label
		}
	}
	return id
}

// Decide answers a pending decision, or picks a candidate from the results view.
func (m *Manager) Decide(userID, jobID, choice string) error {
	job, ok := m.Get(userID, jobID)
	if !ok {
		return errors.New("job not found")
	}

	job.mu.Lock()
	pending := job.Decision
	phase := job.Phase
	job.mu.Unlock()

	if pending != nil {
		select {
		case job.decide <- choice:
			return nil
		default:
			return errors.New("decision already answered")
		}
	}

	if phase != PhaseResults {
		return errors.New("nothing to decide")
	}

	switch {
	case choice == "quit":
		job.log("info", "cancelled — nothing published")
		m.fail(job, "cancelled")
		return nil
	case strings.HasPrefix(choice, "use:"):
		name := strings.TrimPrefix(choice, "use:")
		pick, ok := m.pick(job, name)
		if !ok {
			return errors.New("candidate not found")
		}
		job.mu.Lock()
		job.Decision = nil
		job.mu.Unlock()
		job.log("info", "picked "+displayName(name)+" — "+pick.cand.Summary(job.targets))
		m.broadcast(job)
		go m.download(job.ctx, job, pick)
		return nil
	default:
		return errors.New("unknown choice")
	}
}

func (m *Manager) bySourceAdd(job *Job, name string, pick sourcePick, deltas []string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.bySource[name] = pick
	job.Candidates = append(job.Candidates, candidateView(name, pick.cand, job.targets, job.Album.Cover, deltas))
	job.updatedAt = time.Now()
}

func (m *Manager) pick(job *Job, name string) (sourcePick, bool) {
	job.mu.Lock()
	defer job.mu.Unlock()
	p, ok := job.bySource[name]
	return p, ok
}

// candidateView renders one album for the results list.
func candidateView(name string, cand downloader.AlbumCandidate, targets []downloader.QualityTarget, cover string, deltas []string) Candidate {
	out := Candidate{
		ID:         name,
		Provider:   displayName(name),
		Title:      cand.Title,
		Artist:     cand.Artist,
		Year:       strconv.Itoa(cand.Year),
		TrackCount: len(cand.Tracks),
		OkTracks:   cand.OkTracks,
		Summary:    cand.Summary(targets),
		Deltas:     deltas,
		Tone:       "partial",
		Cover:      cover,
		Tracks:     make([]CandidateTrack, 0, len(cand.Tracks)),
	}
	for _, t := range cand.Tracks {
		facts := t.Quality.Label()
		if !t.Known || !t.Quality.Known() {
			facts = "not probed — verified at transfer"
		}
		out.Tracks = append(out.Tracks, CandidateTrack{
			N:     t.Number,
			Title: t.Title,
			OK:    t.Known && t.Quality.Known() && t.Quality.Meets(downloader.EffectiveTarget(cand.Ceiling, targets)),
			Facts: facts,
		})
	}
	return out
}

// Get returns the live job, checking ownership.
func (m *Manager) Get(userID, jobID string) (*Job, bool) {
	m.mu.RLock()
	job := m.jobs[jobID]
	m.mu.RUnlock()
	if job == nil {
		return nil, false
	}
	job.mu.Lock()
	owner := job.userID
	job.mu.Unlock()
	if userID != "" && owner != userID {
		return nil, false
	}
	return job, true
}

// List returns the caller's jobs, newest first.
func (m *Manager) List(userID string) []JobView {
	m.mu.RLock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.RUnlock()

	type entry struct {
		view    JobView
		created time.Time
	}
	entries := make([]entry, 0, len(jobs))
	for _, j := range jobs {
		j.mu.Lock()
		if userID != "" && j.userID != userID {
			j.mu.Unlock()
			continue
		}
		created := j.createdAt
		j.mu.Unlock()
		entries = append(entries, entry{view: j.Snapshot(), created: created})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].created.After(entries[j].created) })

	out := make([]JobView, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.view)
	}
	return out
}

// Cancel stops a job wherever it is.
func (m *Manager) Cancel(userID, jobID string) error {
	job, ok := m.Get(userID, jobID)
	if !ok {
		return errors.New("job not found")
	}
	job.mu.Lock()
	cancel := job.cancel
	job.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.fail(job, "cancelled")
	return nil
}

func (m *Manager) fail(job *Job, reason string) {
	job.mu.Lock()
	already := job.Phase == PhaseDone || job.Phase == PhaseFailed
	job.Phase = PhaseFailed
	job.Result = reason
	job.Decision = nil
	job.mu.Unlock()
	if !already {
		job.log("error", reason)
	}
	m.broadcast(job)
	m.persist(job)
}

func (m *Manager) broadcast(job *Job) {
	if m.sse == nil {
		return
	}
	snap := job.Snapshot()
	job.mu.Lock()
	userID := job.userID
	job.mu.Unlock()
	m.sse.SendToUser(userID, "download:job", snap)
}

func (m *Manager) persist(job *Job) {
	if m.store == nil {
		return
	}
	snap := job.Snapshot()
	job.mu.Lock()
	userID := job.userID
	job.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.store.Save(ctx, userID, snap); err != nil {
		m.logger.Warn("could not persist download job", zap.Error(err), zap.String("job", snap.ID))
	}
}

func firstReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return reasons[0]
}

func modeLabel(mode string) string {
	if mode == "best" {
		return "best quality"
	}
	return "source order"
}

// humanBytes keeps the result line short: "486 MB".
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(b)/float64(div), "KMGT"[exp])
}
