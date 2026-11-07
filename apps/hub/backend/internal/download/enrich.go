package download

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// mbInfo is the MusicBrainz side of a download: who made the release group,
// which release it probably is, and which recording each ISRC names.
//
// The hub only knows a release *group* — that is what the artist page and the
// fs agent match on — so a tagger that writes only MUSICBRAINZ_RELEASEGROUPID is
// telling the truth and nothing more. The rest is recoverable, at one request per
// second: the group carries the artist id and its releases, and every track
// carries an ISRC that MusicBrainz maps straight to a recording.
type mbInfo struct {
	rgid string

	mu      sync.Mutex
	artist  string
	release string
	rec     map[string]string // isrc -> recording id
	pending map[string]bool   // isrcs not yet looked up
	err     string
}

type mbRelease struct {
	ID     string       `json:"id"`
	Date   string       `json:"date"`
	Status string       `json:"status"`
	Media  []mbMediaOpt `json:"media"`
}

type mbMediaOpt struct {
	TrackCount int `json:"track-count"`
}

type mbGroupResp struct {
	ID           string `json:"id"`
	PrimaryType  string `json:"primary-type"`
	FirstRelease string `json:"first-release-date"`
	ArtistCredit []struct {
		Name   string `json:"name"`
		Artist struct {
			ID string `json:"id"`
		} `json:"artist"`
	} `json:"artist-credit"`
	Releases []mbRelease `json:"releases"`
}

type mbISRCResp struct {
	Recordings []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"recordings"`
}

// resolveMB starts the lookups in the background and returns immediately: tracks
// are still downloading while it runs, and a 1 req/s budget means a 14-track
// album takes ~15s, which is roughly how long the audio takes anyway.
func (m *Manager) resolveMB(ctx context.Context, rgid string, cand downloader.AlbumCandidate) *mbInfo {
	info := &mbInfo{
		rgid: rgid,
		rec:  make(map[string]string, len(cand.Tracks)),
	}
	// No group id, no client, or an id that is not a UUID (a test fixture, a
	// malformed request): nothing to look up, and nothing must wait for it.
	if rgid == "" || m.mb == nil || !looksLikeUUID(rgid) {
		return info
	}

	info.pending = make(map[string]bool, len(cand.Tracks))
	for _, t := range cand.Tracks {
		if t.ISRC != "" {
			info.pending[t.ISRC] = true
		}
	}
	if len(info.pending) == 0 {
		info.pending = nil
	}

	go func() {
		defer func() {
			info.mu.Lock()
			info.pending = nil
			info.mu.Unlock()
		}()

		mb := m.mb
		if mb == nil {
			return
		}

		var group mbGroupResp
		path := "/release-group/" + rgid + "?fmt=json&inc=artist-credits+releases+media"
		if err := mb.get(ctx, path, &group); err != nil {
			info.mu.Lock()
			info.err = err.Error()
			info.pending = nil
			info.mu.Unlock()
			return
		}

		artistID := ""
		if len(group.ArtistCredit) > 0 {
			artistID = group.ArtistCredit[0].Artist.ID
		}

		info.mu.Lock()
		info.artist = artistID
		// Only claim a release when exactly one matches what we downloaded:
		// several editions of the same group are normal, and a wrong
		// MUSICBRAINZ_ALBUMID is worse than none.
		if id := pickRelease(group.Releases, len(cand.Tracks)); id != "" {
			info.release = id
		}
		isrcs := make([]string, 0, len(info.pending))
		for isrc := range info.pending {
			isrcs = append(isrcs, isrc)
		}
		info.mu.Unlock()

		for _, isrc := range isrcs {
			if ctx.Err() != nil {
				return
			}
			var resp mbISRCResp
			if err := mb.get(ctx, "/isrc/"+isrc+"?fmt=json", &resp); err != nil {
				continue
			}
			if len(resp.Recordings) == 0 {
				continue
			}
			info.mu.Lock()
			info.rec[isrc] = resp.Recordings[0].ID
			delete(info.pending, isrc)
			info.mu.Unlock()
		}
	}()

	return info
}

// pickRelease returns a release id only when the choice is unambiguous.
func pickRelease(releases []mbRelease, tracks int) string {
	match := ""
	for _, r := range releases {
		if tracks > 0 && releaseTrackCount(r) != tracks {
			continue
		}
		if strings.EqualFold(r.Status, "Pseudo-Release") || strings.EqualFold(r.Status, "Bootleg") {
			continue
		}
		if match != "" {
			return "" // more than one candidate: stay silent
		}
		match = r.ID
	}
	return match
}

func releaseTrackCount(r mbRelease) int {
	n := 0
	for _, m := range r.Media {
		n += m.TrackCount
	}
	return n
}

// recording waits briefly for an ISRC that may still be queued. The download of
// a single track takes longer than the 1.1s request budget, so in practice the
// answer is already there by the time the file is written.
func (i *mbInfo) recording(ctx context.Context, isrc string, wait time.Duration) string {
	if isrc == "" {
		return ""
	}
	deadline := time.Now().Add(wait)
	for {
		i.mu.Lock()
		id := i.rec[isrc]
		pending := i.pending != nil && i.pending[isrc]
		i.mu.Unlock()

		if id != "" || !pending {
			return id
		}
		if time.Now().After(deadline) {
			return ""
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(120 * time.Millisecond):
		}
	}
}

// looksLikeUUID is deliberately loose: it only needs to reject ids that would
// waste a request, not validate MusicBrainz.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func (i *mbInfo) artistID() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.artist
}

func (i *mbInfo) releaseID() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.release
}

// summary reports how much of the MusicBrainz enrichment landed, so the log
// says "12/14" instead of silently writing files with missing ids.
func (i *mbInfo) summary() string {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.artist == "" && len(i.rec) == 0 {
		if i.err != "" {
			return "mb · unavailable (" + i.err + ")"
		}
		return ""
	}

	parts := []string{}
	if i.artist != "" {
		parts = append(parts, "artist "+shortID(i.artist))
	}
	if i.release != "" {
		parts = append(parts, "release "+shortID(i.release))
	}
	total := len(i.rec) + len(i.pending)
	parts = append(parts, fmt.Sprintf("recordings %d/%d", len(i.rec), total))
	return "mb · " + strings.Join(parts, " · ")
}

func (i *mbInfo) failures() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.err
}
