package download

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// The JSON shapes here are the modal's contract
// (apps/hub/frontend/app/types/download-job.ts). The server emits the same
// object the frontend renders, so there is no mapping layer to drift: one
// snapshot, one shape.
//
// The whole job is sent on every change over SSE (`download:job`). Granular
// per-field events would save bytes and cost a client-side reducer; at album
// scale (tens of sends) the snapshot is simpler and cannot desync.

type LogLine struct {
	T     string `json:"t"`
	Level string `json:"level"` // info | ok | warn | error
	Text  string `json:"text"`
}

type AlbumView struct {
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Year       string `json:"year"`
	TrackCount int    `json:"trackCount"`
	Cover      string `json:"cover,omitempty"`

	// ReleaseID is the MusicBrainz release the user picked, when they picked
	// one. A release group is the album; a release is the edition — the thing
	// that owns a label, a date and a barcode. Left empty, the resolver guesses
	// one from the track count, which is a guess and is not always right.
	ReleaseID string `json:"releaseId,omitempty"`
}

type SourceView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"` // pending | running | fit | partial | miss | failed
	Reason  string `json:"reason,omitempty"`
	Summary string `json:"summary,omitempty"`
	Ceiling string `json:"ceiling,omitempty"`
}

type CandidateTrack struct {
	N     int    `json:"n"`
	Title string `json:"title"`
	OK    bool   `json:"ok"`
	Facts string `json:"facts"` // "FLAC 16-bit/44.1kHz"
}

type Candidate struct {
	ID         string           `json:"id"` // provider name, used as the pick id
	Provider   string           `json:"provider"`
	Title      string           `json:"title"`
	Artist     string           `json:"artist"`
	Year       string           `json:"year"`
	TrackCount int              `json:"trackCount"`
	OkTracks   int              `json:"okTracks"`
	Summary    string           `json:"summary"`
	Deltas     []string         `json:"deltas"`
	Tracks     []CandidateTrack `json:"tracks,omitempty"`
	Tone       string           `json:"tone"` // fit | partial
	Cover      string           `json:"cover,omitempty"`
}

type DecisionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"` // primary | danger | ghost
}

type Decision struct {
	Kind    string           `json:"kind"` // pick_release | track_failed
	Title   string           `json:"title"`
	Body    string           `json:"body,omitempty"`
	Options []DecisionOption `json:"options"`
}

type Progress struct {
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current,omitempty"`
}

// Job phases mirror the frontend union.
const (
	PhaseWalking     = "walking"
	PhaseDownloading = "downloading"
	PhaseWorking     = "working" // retag: rewriting files, nothing is downloading
	PhaseResults     = "results"
	PhaseDone        = "done"
	PhaseFailed      = "failed"
)

// JobView is the serializable state: exactly what the browser renders. It is a
// plain value — no lock, safe to copy and to hand to JSON.
type JobView struct {
	ID   string `json:"id"`
	RGID string `json:"rg_id"`

	// Kind is what the job is doing: a download, a look at what a provider would
	// give (inspect), or a rewrite of the tags of an album already on disk
	// (retag). All three report through the same snapshot, which is why one modal
	// can show any of them.
	Kind string `json:"kind"`

	// Dir is where the album ended up, published or retagged.
	Dir string `json:"dir,omitempty"`

	Phase      string       `json:"phase"`
	Mode       string       `json:"mode"` // order | best
	Album      AlbumView    `json:"album"`
	Sources    []SourceView `json:"sources"`
	Log        []LogLine    `json:"log"`
	Progress   *Progress    `json:"progress"`
	Candidates []Candidate  `json:"candidates"`
	Decision   *Decision    `json:"decision"`
	Result     string       `json:"result,omitempty"`
}

// Job is the live job: the view plus the machinery that mutates it. Unexported
// fields never reach JSON, and Snapshot is the only way out of the lock — a
// struct containing a mutex must never be copied by value (go vet agrees).
type Job struct {
	JobView

	mu        sync.Mutex
	userID    string
	agentID   string
	ref       downloader.AlbumRef
	targets   []downloader.QualityTarget
	bySource  map[string]sourcePick // provider name -> what to download if picked
	prefer    sourceHint            // what the user clicked on the download page
	decide    chan string
	cancel    context.CancelFunc
	ctx       context.Context
	createdAt time.Time
	updatedAt time.Time
}

// sourceHint is the album the user clicked, so the walk can skip its own search
// for that one provider.
type sourceHint struct {
	provider string
	albumID  string
}

// sourcePick is what the manager needs to turn a candidate into a download.
type sourcePick struct {
	source  downloader.Source
	albumID string
	cand    downloader.AlbumCandidate
}

// Snapshot returns a copy that can be serialized while the run goroutine keeps
// mutating the live job.
func (j *Job) Snapshot() JobView {
	j.mu.Lock()
	defer j.mu.Unlock()

	v := j.JobView
	v.Log = append([]LogLine(nil), j.Log...)
	v.Sources = append([]SourceView(nil), j.Sources...)
	v.Candidates = append([]Candidate(nil), j.Candidates...)
	if j.Progress != nil {
		p := *j.Progress
		v.Progress = &p
	}
	if j.Decision != nil {
		d := *j.Decision
		d.Options = append([]DecisionOption(nil), j.Decision.Options...)
		v.Decision = &d
	}
	return v
}

func (j *Job) log(level, text string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Log = append(j.Log, LogLine{T: clock(time.Since(j.createdAt)), Level: level, Text: text})
	// Ring buffer: a 200-line tail is plenty to replay into a reopened modal and
	// stops an unattended job from growing without bound.
	if len(j.Log) > 200 {
		j.Log = j.Log[len(j.Log)-200:]
	}
	j.updatedAt = time.Now()
}

func (j *Job) setProgress(done int, current string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.Progress == nil {
		j.Progress = &Progress{}
	}
	j.Progress.Done = done
	j.Progress.Current = current
	j.updatedAt = time.Now()
}

func (j *Job) setSource(id, status, reason, summary string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := range j.Sources {
		if j.Sources[i].ID != id {
			continue
		}
		j.Sources[i].Status = status
		if reason != "" {
			j.Sources[i].Reason = reason
		}
		if summary != "" {
			j.Sources[i].Summary = summary
		}
		j.updatedAt = time.Now()
		return
	}
}

func clock(d time.Duration) string {
	sec := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d.%d", sec/60, sec%60, d.Milliseconds()/100%10)
}
