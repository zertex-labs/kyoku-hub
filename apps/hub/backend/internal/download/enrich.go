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

	// chosen is the release the user picked on the download page. When set it
	// wins outright: guessing from the track count is only for the case where
	// nobody chose, and it can pick a different edition than the one on screen.
	chosen string

	mu      sync.Mutex
	artist  string
	release string
	rec     map[string]string // isrc -> recording id

	relTitle   string // the chosen release's title, date and label
	relDate    string
	relLabel   string
	relID      string
	relStatus  string
	relCountry string
	relCatalog string
	relBarcode string

	groupGenres    []string
	groupFirst     string // first release date of the group
	groupPrimary   string // Album, Single, EP…
	groupSecondary []string
	artistSort     string

	// trackRec maps "disc:track" to the recording on the chosen edition.
	trackRec   map[string]string
	trackTotal int
	pending    map[string]bool // isrcs not yet looked up
	err        string
}

type mbRelease struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Date    string       `json:"date"`
	Country string       `json:"country"`
	Status  string       `json:"status"`
	Media   []mbMediaOpt `json:"media"`
}

type mbMediaOpt struct {
	TrackCount int `json:"track-count"`
	Position   int `json:"position"`
	Tracks     []struct {
		Position  int `json:"position"`
		Recording struct {
			ID string `json:"id"`
		} `json:"recording"`
	} `json:"tracks"`
}

type mbGroupResp struct {
	ID            string   `json:"id"`
	PrimaryType   string   `json:"primary-type"`
	SecondaryType []string `json:"secondary-types"`
	FirstRelease  string   `json:"first-release-date"`
	Genres        []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"genres"`
	Tags []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"tags"`
	ArtistCredit []struct {
		Name   string `json:"name"`
		Artist struct {
			ID       string `json:"id"`
			SortName string `json:"sort-name"`
		} `json:"artist"`
	} `json:"artist-credit"`
	Releases []mbRelease `json:"releases"`
}

// mbReleaseResp is one edition: the thing that owns a label, a date and a title.
type mbReleaseResp struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Date      string `json:"date"`
	Country   string `json:"country"`
	Status    string `json:"status"`
	Barcode   string `json:"barcode"`
	LabelInfo []struct {
		Label struct {
			Name string `json:"name"`
		} `json:"label"`
		CatalogNumber string `json:"catalog-number"`
	} `json:"label-info"`
	Media []mbMediaOpt `json:"media"`
}

// CatalogNumber is the edition's identifier at its label.
func (r mbReleaseResp) CatalogNumber() string {
	for _, l := range r.LabelInfo {
		if l.CatalogNumber != "" {
			return l.CatalogNumber
		}
	}
	return ""
}

// LabelName is the imprint of the edition, without the catalogue number.
func (r mbReleaseResp) LabelName() string {
	for _, l := range r.LabelInfo {
		if l.Label.Name != "" {
			return l.Label.Name
		}
	}
	return ""
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
func (m *Manager) resolveMB(ctx context.Context, rgid, releaseID string, cand downloader.AlbumCandidate) *mbInfo {
	info := &mbInfo{
		rgid:       rgid,
		chosen:     releaseID,
		rec:        make(map[string]string, len(cand.Tracks)),
		trackTotal: len(cand.Tracks),
	}
	// No group id, no client, or an id that is not a UUID (a test fixture, a
	// malformed request): nothing to look up, and nothing must wait for it.
	if rgid == "" || m.mb == nil || !looksLikeUUID(rgid) {
		return info
	}

	// The release group is fetched before the first file is written: genres, the
	// release type, the artist id and its sort name are album-level tags, and a
	// value that arrives after the last file is written is not a tag at all.
	var group mbGroupResp
	if err := m.mb.get(ctx, "/release-group/"+rgid+"?fmt=json&inc=artist-credits+releases+media+genres", &group); err == nil {
		if len(group.ArtistCredit) > 0 {
			info.artist = group.ArtistCredit[0].Artist.ID
			info.artistSort = group.ArtistCredit[0].Artist.SortName
		}
		info.groupFirst = group.FirstRelease
		info.groupPrimary = group.PrimaryType
		info.groupSecondary = group.SecondaryType
		// Genres by vote count: "j-pop", not the provider's regional label
		// ("Asian Music", translated per country).
		for _, g := range group.Genres {
			info.groupGenres = append(info.groupGenres, g.Name)
		}
		// Only claim a release when exactly one matches what we downloaded:
		// several editions of the same group are normal, and a wrong
		// MUSICBRAINZ_ALBUMID is worse than none. A release the user chose
		// needs no guessing at all.
		if releaseID != "" {
			info.release = releaseID
		} else if id := pickRelease(group.Releases, len(cand.Tracks)); id != "" {
			info.release = id
		}
	} else {
		info.err = err.Error()
	}

	// One request, synchronously, because the album title, date, label and the
	// recording ids go into the very first file. The ISRC lookups below can lag;
	// these cannot.
	if info.release != "" {
		m.loadRelease(ctx, info, info.release)
	}

	// The edition's tracklist already names every recording; asking per ISRC on
	// top of that is N more requests for the same answer, and N chances to be
	// rate-limited halfway through an album.
	if len(info.trackRec) >= len(cand.Tracks) {
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

		isrcs := make([]string, 0, len(info.pending))
		info.mu.Lock()
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
// pickRelease chooses the edition the tags should name.
//
// Staying silent when several editions match sounds cautious and is worse in
// practice: an album with nine editions then gets no MUSICBRAINZ_ALBUMID at all,
// and every file stays half-identified. The edition that comes first — exact
// track count, then the earliest release date — is the original, which is the one
// whose identifiers other tools will recognise.
func pickRelease(releases []mbRelease, tracks int) string {
	if tracks > 0 {
		if id := earliestRelease(releases, func(n int) bool { return n == tracks }); id != "" {
			return id
		}
	}
	// A folder that is missing a track still belongs to the edition that contains
	// every file it does have.
	return earliestRelease(releases, func(n int) bool { return tracks == 0 || n >= tracks })
}

func earliestRelease(releases []mbRelease, ok func(int) bool) string {
	best, bestDate := "", ""
	for _, r := range releases {
		if !ok(releaseTrackCount(r)) {
			continue
		}
		if strings.EqualFold(r.Status, "Pseudo-Release") || strings.EqualFold(r.Status, "Bootleg") {
			continue
		}
		if best == "" || (r.Date != "" && (bestDate == "" || r.Date < bestDate)) {
			best, bestDate = r.ID, r.Date
		}
	}
	return best
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

func (i *mbInfo) albumTitle() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.relTitle
}

// firstDate is the release group's first release date, used when no edition was
// resolved but a year is still wanted.
func (i *mbInfo) firstDate() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.groupFirst
}

func (i *mbInfo) albumDate() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.relDate
}

func (i *mbInfo) albumLabel() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.relLabel
}

// recordingAt is the recording id the edition itself declares for this position.
// It needs no lookup, no ISRC and no waiting.
func (i *mbInfo) recordingAt(disc, track int) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.trackRec) == 0 {
		return ""
	}
	if disc == 0 {
		disc = 1
	}
	return i.trackRec[fmt.Sprintf("%d:%d", disc, track)]
}

func (i *mbInfo) releaseStatus() string  { i.mu.Lock(); defer i.mu.Unlock(); return i.relStatus }
func (i *mbInfo) releaseCountry() string { i.mu.Lock(); defer i.mu.Unlock(); return i.relCountry }
func (i *mbInfo) catalogNumber() string  { i.mu.Lock(); defer i.mu.Unlock(); return i.relCatalog }
func (i *mbInfo) barcode() string        { i.mu.Lock(); defer i.mu.Unlock(); return i.relBarcode }
func (i *mbInfo) genres() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.groupGenres...)
}
func (i *mbInfo) firstRelease() string   { i.mu.Lock(); defer i.mu.Unlock(); return i.groupFirst }
func (i *mbInfo) artistSortName() string { i.mu.Lock(); defer i.mu.Unlock(); return i.artistSort }

// loadRelease fetches one edition and indexes what only it knows: the label, the
// country, the barcode, the catalogue number and which recording is at each
// position.
func (m *Manager) loadRelease(ctx context.Context, info *mbInfo, releaseID string) {
	if m.mb == nil || !looksLikeUUID(releaseID) {
		return
	}
	var rel mbReleaseResp
	if err := m.mb.get(ctx, "/release/"+releaseID+"?fmt=json&inc=labels+release-groups+recordings", &rel); err != nil {
		return
	}

	info.mu.Lock()
	defer info.mu.Unlock()
	info.relID = rel.ID
	info.relTitle = rel.Title
	info.relDate = rel.Date
	info.relLabel = rel.LabelName()
	info.relStatus = rel.Status
	info.relCountry = rel.Country
	info.relBarcode = rel.Barcode
	info.relCatalog = rel.CatalogNumber()
	info.trackRec = make(map[string]string, len(rel.Media))
	for _, md := range rel.Media {
		disc := md.Position
		if disc == 0 {
			disc = 1
		}
		for _, tr := range md.Tracks {
			if tr.Recording.ID != "" {
				info.trackRec[fmt.Sprintf("%d:%d", disc, tr.Position)] = tr.Recording.ID
			}
		}
	}
}

// releaseType is the group's primary type, with any secondary types appended the
// way MusicBrainz presents them ("album + compilation").
func (i *mbInfo) releaseType() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.groupPrimary == "" {
		return ""
	}
	out := strings.ToLower(i.groupPrimary)
	for _, s := range i.groupSecondary {
		out += " + " + strings.ToLower(s)
	}
	return out
}

// albumArtistID is the release artist: the same credit as the group's here,
// which is wrong only for compilations that name a various-artists credit.
func (i *mbInfo) albumArtistID() string { return i.artistID() }

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

	if i.artist == "" && len(i.rec) == 0 && len(i.trackRec) == 0 {
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
	// Count against the album, not against what we happened to look up: "14/14"
	// on a 16-track album reads as complete when two tracks have no id.
	have := make(map[string]bool, len(i.rec)+len(i.trackRec))
	for _, id := range i.trackRec {
		have[id] = true
	}
	for _, id := range i.rec {
		have[id] = true
	}
	total := i.trackTotal
	if total < len(have) {
		total = len(have)
	}
	parts = append(parts, fmt.Sprintf("recordings %d/%d", len(have), total))
	return "mb · " + strings.Join(parts, " · ")
}

func (i *mbInfo) failures() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.err
}
