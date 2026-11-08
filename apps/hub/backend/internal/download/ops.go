package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download/postprocess"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// KindInspect answers "what would this provider give me now", without downloading
// a byte.
//
// For an album already in the library that is the question behind "is there a
// better copy out there": the answer is the quality each source would deliver, and
// the user decides whether that is worth a download. It runs as a job because the
// browser already renders jobs — a second progress mechanism would be a second set
// of bugs.
const (
	KindInspect = "inspect"
	KindRetag   = "retag"
)

// StartInspect creates the job and probes in the background.
func (m *Manager) StartInspect(ctx context.Context, userID, rgID string, req StartRequest) (JobView, error) {
	sources := m.sources(ctx, userID)
	if len(sources) == 0 {
		return JobView{}, fmt.Errorf("no download sources configured")
	}

	job := &Job{
		JobView: JobView{
			ID:    uuid.NewString(),
			Kind:  KindInspect,
			RGID:  rgID,
			Phase: PhaseWalking,
			Mode:  orDefault(req.Mode, "order"),
			Album: req.Album,
			Log:   []LogLine{},
		},
		decide:  make(chan string, 1),
		cancel:  func() {},
		ctx:     ctx,
		userID:  userID,
		agentID: req.AgentID,
		targets: downloader.DefaultTargets(),
		prefer:  sourceHint{provider: req.PreferredProvider, albumID: req.PreferredAlbumID},

		bySource:  map[string]sourcePick{},
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
		src := sources[name]
		if src == nil {
			continue
		}
		job.Sources = append(job.Sources, SourceView{
			ID:      name,
			Name:    displayName(name),
			Status:  "pending",
			Ceiling: src.Ceiling().ShortLabel(),
		})
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job.ctx = jobCtx
	job.cancel = cancel

	m.mu.Lock()
	m.jobs[job.ID] = job
	if rgID != "" {
		m.byRG[userID+"\x00"+rgID] = job.ID
	}
	m.mu.Unlock()

	job.log("info", "inspect · "+req.Album.Title+" — nothing will be downloaded")
	go m.inspect(jobCtx, job, chain, sources)

	m.persist(job)
	return job.Snapshot(), nil
}

func (m *Manager) inspect(ctx context.Context, job *Job, chain []string, sources map[string]downloader.Source) {
	defer func() {
		if r := recover(); r != nil {
			m.fail(job, fmt.Sprintf("inspect crashed: %v", r))
		}
	}()

	query := strings.TrimSpace(job.Album.Artist + " " + job.Album.Title)
	best := downloader.UnknownQuality
	bestName := ""

	for _, name := range chain {
		if ctx.Err() != nil {
			return
		}
		src := sources[name]
		if src == nil {
			continue
		}
		job.setSource(name, "running", "", "")
		m.broadcast(job)

		pick, verdict, reasons, _ := m.probeSource(ctx, job, name, src, query)
		status := strings.ToLower(string(verdict))

		if verdict == downloader.VerdictMiss || pick.cand.ProviderID == "" {
			job.setSource(name, status, firstReason(reasons), "")
			job.log("warn", name+": "+verdictLine(verdict, reasons))
			m.broadcast(job)
			continue
		}

		job.setSource(name, status, firstReason(reasons), pick.cand.Summary(job.targets))
		job.log("info", fmt.Sprintf("%s: %s — %s", name, status, pick.cand.Summary(job.targets)))

		// The album floor is the weakest track: one 128kbps file makes the whole
		// album 128kbps, which is exactly the number someone upgrading cares about.
		floor, ok := candFloor(pick.cand)
		if ok && (best == downloader.UnknownQuality || floor.TierScore() > best.TierScore()) {
			best = floor
			bestName = name
		}
		m.broadcast(job)
	}

	job.mu.Lock()
	job.Phase = PhaseDone
	job.Progress = nil
	if best.Known() {
		job.Result = "best available · " + best.ShortLabel() + " (" + displayName(bestName) + ")"
	} else {
		job.Result = "nothing available"
	}
	job.mu.Unlock()

	if best.Known() {
		job.log("info", "best available · "+best.ShortLabel()+" from "+displayName(bestName))
	} else {
		job.log("info", "no source has this album in a known quality")
	}
	job.log("info", "Done.")
	m.broadcast(job)
	m.persist(job)
}

// candFloor is the lowest known track quality of a candidate.
func candFloor(c downloader.AlbumCandidate) (downloader.AudioQuality, bool) {
	floor := downloader.UnknownQuality
	found := false
	for _, t := range c.Tracks {
		if !t.Known || !t.Quality.Known() {
			continue
		}
		if !found || t.Quality.TierScore() < floor.TierScore() {
			floor = t.Quality
			found = true
		}
	}
	return floor, found
}

func verdictLine(v downloader.Verdict, reasons []string) string {
	switch v {
	case downloader.VerdictFit:
		return "fit"
	case downloader.VerdictPartial:
		return "partial"
	default:
		if r := firstReason(reasons); r != "" {
			return "miss — " + r
		}
		return "miss"
	}
}

// StartRetag rewrites the tags of an album already on disk.
//
// The audio is not touched. Whatever the file says about itself — track number,
// ISRC — is kept, and the album-level facts are resolved again: the edition's
// title, date, label, country, barcode, catalogue number, its genres and the
// MusicBrainz ids. That is what fixes an album carrying the provider's licensed
// title, or one tagged before editions existed.
//
// dir comes from the download record: the hub only re-tags what it published.
func (m *Manager) StartRetag(ctx context.Context, userID, rgID string, req StartRequest, dir string) (JobView, error) {
	if dir == "" {
		return JobView{}, fmt.Errorf("this album has no download record, so there is nothing here to re-tag")
	}

	job := &Job{
		JobView: JobView{
			ID:    uuid.NewString(),
			Kind:  KindRetag,
			RGID:  rgID,
			Phase: PhaseWalking,
			Album: req.Album,
			Dir:   dir,
			Log:   []LogLine{},
		},
		decide:    make(chan string, 1),
		cancel:    func() {},
		ctx:       ctx,
		userID:    userID,
		agentID:   req.AgentID,
		targets:   downloader.DefaultTargets(),
		bySource:  map[string]sourcePick{},
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job.ctx = jobCtx
	job.cancel = cancel

	m.mu.Lock()
	m.jobs[job.ID] = job
	if rgID != "" {
		m.byRG[userID+"\x00"+rgID] = job.ID
	}
	m.mu.Unlock()

	job.log("info", "re-tag · "+req.Album.Title+" — reading what is on disk")
	go m.retag(jobCtx, job, dir)

	m.persist(job)
	return job.Snapshot(), nil
}

// retag is the work: read the files, resolve the album again, write it back.
func (m *Manager) retag(ctx context.Context, job *Job, dir string) {
	defer func() {
		if r := recover(); r != nil {
			m.fail(job, fmt.Sprintf("retag crashed: %v", r))
		}
	}()

	files, err := audioFilesIn(dir)
	if err != nil {
		m.fail(job, "cannot read the album directory: "+err.Error())
		return
	}
	if len(files) == 0 {
		m.fail(job, "no FLAC or MP3 files in "+dir)
		return
	}

	cand := downloader.AlbumCandidate{
		Title:  job.Album.Title,
		Artist: job.Album.Artist,
		Tracks: make([]downloader.TrackQuality, 0, len(files)),
	}
	for _, f := range files {
		v, err := postprocess.ReadTags(f)
		if err != nil {
			job.log("warn", filepath.Base(f)+": "+err.Error())
			continue
		}
		n := v.Track
		if n == 0 {
			n = 1
		}
		cand.Tracks = append(cand.Tracks, downloader.TrackQuality{
			Title:  v.Title,
			Number: n,
			Disc:   v.Disc,
			ISRC:   v.ISRC,
			Known:  true,
		})
	}
	if len(cand.Tracks) == 0 {
		m.fail(job, "no readable audio files")
		return
	}
	job.log("info", plural(len(cand.Tracks), "file", "files")+" · resolving the release")

	mb := m.resolveMB(ctx, job.RGID, job.Album.ReleaseID, cand)
	cover := readCoverIfAny(dir)
	opts := m.options(job)
	chain := m.chain(opts)
	for _, line := range metadataLines(job, cand, mb, cover, chain, opts) {
		job.log("info", line)
	}

	job.mu.Lock()
	job.Phase = PhaseWorking
	job.Progress = &Progress{Total: len(files)}
	job.mu.Unlock()
	m.broadcast(job)

	done := 0
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		v, err := postprocess.ReadTags(f)
		if err != nil {
			continue
		}
		n := v.Track
		if n == 0 {
			n = 1
		}
		tq := downloader.TrackQuality{
			Title: v.Title, Number: n, Disc: v.Disc, ISRC: v.ISRC, Known: true,
		}
		art := m.artifact(job, tq, cand, mb, f, filepath.Ext(f), cover)
		for _, o := range chain.Run(ctx, &art) {
			if reason, skipped := o.Skipped(); skipped {
				job.log("warn", filepath.Base(f)+": "+o.Name+" skipped — "+reason)
			} else if o.Err != nil {
				job.log("error", filepath.Base(f)+": "+o.Name+" — "+o.Err.Error())
			}
		}
		done++
		job.setProgress(done, filepath.Base(f))
		job.log("info", fmt.Sprintf("t%02d retagged · %s", n, filepath.Base(f)))
		m.broadcast(job)
	}

	job.mu.Lock()
	job.Phase = PhaseDone
	job.Progress = nil
	job.Result = plural(done, "file retagged", "files retagged")
	job.mu.Unlock()
	job.log("info", "Done.")
	m.broadcast(job)
	m.persist(job)

	if m.OnPublish != nil {
		m.OnPublish(job.userID, job.agentID)
	}
}

func audioFilesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".flac", ".mp3":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func readCoverIfAny(dir string) []byte {
	if data, err := os.ReadFile(filepath.Join(dir, "cover.jpg")); err == nil {
		return data
	}
	return nil
}

// plural keeps the log readable at one file, which is the common case for singles.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// FindAlbumDir locates an album in the library by what its files say about
// themselves.
//
// The download record only covers albums this hub published; a library that
// predates kyoku, or one built by another tool, has no record at all. Reading the
// first track's release-group tag is exact — the folder name is not, since every
// tool has its own idea of that ("Armageddon (2024)" vs "[2024] Armageddon - The
// 1st Album") — so the tag decides and the walk only narrows the search.
func (m *Manager) FindAlbumDir(rgID string) string {
	if rgID == "" || m.root == "" {
		return ""
	}

	var found string
	root := m.root
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Staging and trash are not albums — but "." is the root itself, and
		// skipping that skips the whole library.
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && rel != "." && strings.HasPrefix(rel, ".") {
			return filepath.SkipDir
		}
		if depth := strings.Count(rel, string(os.PathSeparator)); depth > 4 {
			return filepath.SkipDir
		}
		files, err := audioFilesIn(path)
		if err != nil || len(files) == 0 {
			return nil
		}
		for _, f := range files {
			v, err := postprocess.ReadTags(f)
			if err != nil {
				continue
			}
			if v.ReleaseGroupID == rgID {
				found = path
				return nil
			}
			break // the first readable file speaks for the album
		}
		return nil
	})
	return found
}
