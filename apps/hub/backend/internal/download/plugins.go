package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// AppVersion is reported to plugins. Set from main; "dev" is fine for a build
// from source.
var AppVersion = "dev"

// SetDispatcher installs the plugin dispatcher. Nil is valid and means "no
// plugins installed", which is the normal case.
func (m *Manager) SetDispatcher(d Dispatcher) { m.dispatch = d }

// payloadFor builds the JSON a plugin receives. Everything comes from state the
// download already resolved, so a plugin never talks to Deezer or MusicBrainz.
func (m *Manager) payloadFor(
	job *Job,
	cand downloader.AlbumCandidate,
	dir string,
	coverPath string,
	tr *downloader.TrackQuality,
	files []File,
) Payload {
	album := Album{
		Title:          firstNonEmptyString(cand.Title, job.Album.Title),
		Artist:         firstNonEmptyString(cand.Artist, job.Album.Artist),
		Year:           dateFor(job, cand),
		Genre:          cand.Genre,
		Label:          cand.Label,
		ReleaseType:    cand.Format,
		ReleaseGroupID: job.RGID,
		Dir:            dir,
		TrackCount:     len(cand.Tracks),
		CoverPath:      coverPath,
	}

	p := Payload{
		App:   App{Name: "kyoku-hub", Version: AppVersion},
		Album: album,
		Files: files,
	}

	if tr != nil {
		ext := extFor(tr.Quality)
		p.Track = &Track{
			Path:       filepath.Join(dir, TrackFileName(tr.Disc, tr.Number, tr.Title, ext)),
			Ext:        ext,
			Title:      tr.Title,
			Number:     tr.Number,
			TrackTotal: len(cand.Tracks),
			Disc:       tr.Disc,
			DiscTotal:  discCount(cand),
			ISRC:       tr.ISRC,
		}
	}
	return p
}

// pluginNames is nil-safe: a hub with no plugins installed has no dispatcher at
// all, which is the normal case.
func (m *Manager) pluginNames() []string {
	if m.dispatch == nil {
		return nil
	}
	return m.dispatch.Names()
}

// runHook dispatches one hook and reports what happened. A failing plugin is a
// warning unless it declared required: true, in which case the caller decides to
// block. Skips are reported once per plugin per job.
func (m *Manager) runHook(ctx context.Context, job *Job, hook Hook, p Payload, skips map[string]bool) bool {
	if m.dispatch == nil || m.dispatch.Count() == 0 {
		return false
	}

	failedRequired := false
	for _, out := range m.dispatch.Dispatch(ctx, hook, p) {
		switch {
		case out.Err == nil:
			if out.Message != "" {
				job.log("info", fmt.Sprintf("%s · %s", out.Plugin, out.Message))
			}
		case out.Skipped:
			key := "plugin:" + out.Plugin
			if !skips[key] {
				skips[key] = true
				job.log("info", fmt.Sprintf("%s · skipped %s: %s", out.Plugin, hook, out.Message))
			}
		default:
			detail := out.Err.Error()
			if out.Stderr != "" {
				detail += ": " + firstLine(out.Stderr)
			}
			job.log("warn", fmt.Sprintf("%s · %s failed — %s", out.Plugin, hook, detail))
			if m.dispatch.Required(out.Plugin) {
				failedRequired = true
			}
		}
	}
	return failedRequired
}

// stagedFiles lists what is currently in the staging dir, so plugin-added files
// can be detected by diffing before and after.
func stagedFiles(dir string) []File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, File{
			Path: filepath.Join(dir, e.Name()),
			Ext:  strings.ToLower(filepath.Ext(e.Name())),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func diffFiles(before, after []File) []File {
	had := make(map[string]bool, len(before))
	for _, f := range before {
		had[f.Path] = true
	}
	var added []File
	for _, f := range after {
		if !had[f.Path] {
			added = append(added, f)
		}
	}
	return added
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// coverPathFor reports where the album art lives in a directory, if it does.
func coverPathFor(dir string) string {
	p := filepath.Join(dir, "cover.jpg")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
