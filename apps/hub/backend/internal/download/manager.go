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

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/sse"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
	postprocess "github.com/zertex-labs/kyoku-hub/packages/tagwriter"
	fspb "github.com/zertex-labs/kyoku-hub/proto/gen/fs/v1"
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
	opts     settings.Provider  // the user's tagger options; nil means the defaults
	filer    Filer              // the agent that writes to the user's disk

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

// Filer is the agent-side half of a download: the hub works out what the album
// is, the agent puts it on the disk.
type Filer interface {
	MakeAlbum(ctx context.Context, userID, agentID string, album *fspb.WriteAlbum, onEvent func(*fspb.WriteEvent)) error
}

// SetFiler installs the thing that can reach an agent.
func (m *Manager) SetFiler(f Filer) { m.filer = f }

// SetSettings installs the user's tagger options. Without it the manager behaves
// as it did before the options existed: everything on.
func (m *Manager) SetSettings(p settings.Provider) { m.opts = p }

// options returns the tagger options for a job's owner.
func (m *Manager) options(job *Job) settings.Options {
	if m.opts == nil {
		return settings.Default()
	}
	return m.opts.Get(context.Background(), job.userID)
}

// chain is the post-processing pipeline this job should run.
func (m *Manager) chain(o settings.Options) *postprocess.Chain {
	return postprocess.Configured(o.EmbedCover, o.WriteCoverFile)
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
	// MusicBrainz artwork of the tags, resolved in the background at 1 req/s
	// while the audio downloads. Best-effort: no network, no MB tags.
	mb := m.resolveMB(ctx, job.RGID, job.Album.ReleaseID, pick.cand)
	if err := mb.failures(); err != "" {
		// The message already names the service; prefixing it again produced
		// "musicbrainz: musicbrainz: status 503".
		job.log("warn", err)
	}

	// One cover download for the whole album. The hub fetches it because it is
	// small, and because the artwork URL is a provider detail the agent has no
	// business knowing.
	cover, err := m.fetchCover(job.Album.Cover)
	if err != nil {
		job.log("warn", "cover art: "+err.Error())
	}

	opts := m.options(job)
	chain := m.chain(opts)
	for _, line := range metadataLines(job, pick.cand, mb, cover, chain, opts) {
		job.log("info", line)
	}

	// The hub resolves what to fetch; the agent fetches it. Anything the hub did
	// to a file itself — staging, tagging, renaming into the library — was only
	// ever possible because the two happened to share a disk.
	ticketer, ok := pick.source.(downloader.Ticketer)
	if !ok {
		m.fail(job, "this source cannot hand out download tickets")
		return
	}
	if m.filer == nil {
		m.fail(job, "no agent is connected, so there is nowhere to put the files")
		return
	}

	album := &fspb.WriteAlbum{
		JobId:       job.ID,
		Kind:        "download",
		AlbumTitle:  job.Album.Title,
		AlbumArtist: job.Album.Artist,
		StagingRel:  filepath.ToSlash(filepath.Join(".staging", job.ID)),
		LibraryRel: filepath.ToSlash(AlbumDirFor(
			opts.AlbumPath, job.Album.Artist, job.Album.Artist, job.Album.Title, m.albumYear(job, mb))),
		Cover:          cover,
		EmbedCover:     opts.EmbedCover,
		WriteCoverFile: opts.WriteCoverFile,
		ScanAfter:      true,
	}

	total := len(pick.cand.Tracks)
	for _, tr := range pick.cand.Tracks {
		if ctx.Err() != nil {
			m.fail(job, "cancelled")
			return
		}
		ext := extFor(tr.Quality)
		name := TrackFileName(tr.Disc, tr.Number, tr.Title, ext)

		ticket, err := ticketer.TrackTicket(ctx, pick.albumID, tr.ProviderID)
		if err != nil {
			m.fail(job, fmt.Sprintf("t%02d %q: %v", tr.Number, tr.Title, err))
			return
		}

		art := m.artifact(job, tr, pick.cand, mb, name, ext, cover)
		album.Tracks = append(album.Tracks, &fspb.WriteTrack{
			DestRel:     name,
			Url:         ticket.URL,
			Headers:     ticket.Headers,
			Track:       int32(tr.Number),
			Disc:        int32(tr.Disc),
			Tags:        tagMessages(&art),
			ExpectedExt: ext,
			Transform:   transformMessage(ticket.Transform),
		})
	}

	job.mu.Lock()
	job.Phase = PhaseDownloading
	job.Progress = &Progress{Total: total}
	job.mu.Unlock()
	m.broadcast(job)

	var finalDir string
	err = m.filer.MakeAlbum(ctx, job.userID, job.agentID, album, func(ev *fspb.WriteEvent) {
		if msg := ev.GetMessage(); msg != "" {
			job.log(ev.GetLevel(), msg)
		}
		if p := phaseFor(ev.GetPhase()); p != "" {
			job.mu.Lock()
			job.Phase = p
			job.mu.Unlock()
		}
		if ev.GetTotal() > 0 {
			job.setProgress(int(ev.GetDone()), "")
		}
		if ev.GetDir() != "" {
			finalDir = ev.GetDir()
		}
		m.broadcast(job)
	})
	if err != nil {
		m.fail(job, err.Error())
		return
	}

	if line := mb.summary(); line != "" {
		job.log("info", line)
	}

	job.mu.Lock()
	job.Phase = PhaseDone
	job.Dir = finalDir
	job.Result = fmt.Sprintf("%d/%d tracks · %s", total, total, pick.cand.Floor.ShortLabel())
	job.mu.Unlock()
	job.log("ok", fmt.Sprintf("Done. %d/%d files · %s → %s", total, total, pick.cand.Floor.ShortLabel(), finalDir))
	m.broadcast(job)
	m.persist(job)
}

// albumYear is the year the folder should carry. The request usually has one
// because the page knows the release group's date, but a download started from a
// search result often does not — and the folder is named before MusicBrainz has
// had a chance to say. "Welcome to MY World" with no year next to a library full
// of dated folders is exactly that gap.
func (m *Manager) albumYear(job *Job, mb *mbInfo) int {
	if y := atoi(job.Album.Year); y > 0 {
		return y
	}
	// The edition's date first, then the release group's first release.
	for _, date := range []string{mb.albumDate(), mb.firstDate()} {
		if len(date) >= 4 {
			if y := atoi(date[:4]); y > 0 {
				return y
			}
		}
	}
	return 0
}

// tagMessages renders an artifact's tag set for the wire.
func tagMessages(a *postprocess.Artifact) []*fspb.Tag {
	set := a.TagSet()
	out := make([]*fspb.Tag, 0, len(set))
	for _, t := range set {
		out = append(out, &fspb.Tag{Key: t.Key, Values: t.Values})
	}
	return out
}

// transformMessage renders the decryption recipe, if the bytes need one.
func transformMessage(tf *downloader.StreamTransform) *fspb.Transform {
	if tf == nil {
		return nil
	}
	return &fspb.Transform{Kind: tf.Kind, Chunk: int32(tf.Chunk), Every: int32(tf.Every), Key: tf.Key, Iv: tf.IV}
}

// phaseFor maps the agent's wording onto the job's, so the UI shows the same
// phases whether the work happened here or on someone else's machine.
func phaseFor(phase string) string {
	switch phase {
	case "downloading":
		return PhaseDownloading
	case "tagging", "publishing":
		return PhaseWorking
	default:
		return ""
	}
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
	job.Dir = destDir
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
