package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download/postprocess"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// mbWait is how long one track waits for its MusicBrainz id before giving up.
// Lookups run at 1 req/s in the background, so a download of any real length has
// the answer ready; this ceiling only matters on a cold, slow network.
const mbWait = 2500 * time.Millisecond

// artifact is the download's view of one file, handed to the post-processors.
// Everything they need was resolved during probe/enrich/download, so a processor
// never has to ask a provider or MusicBrainz anything.
func (m *Manager) artifact(
	job *Job,
	tr downloader.TrackQuality,
	cand downloader.AlbumCandidate,
	mb *mbInfo,
	path string,
	ext string,
	cover []byte,
) postprocess.Artifact {
	artist := firstNonEmptyString(cand.Artist, job.Album.Artist)
	album := firstNonEmptyString(cand.Title, job.Album.Title)

	return postprocess.Artifact{
		Path: path,
		Ext:  ext,

		Album:          album,
		AlbumArtist:    artist,
		Artist:         artist,
		Year:           dateFor(job, cand),
		Genre:          cand.Genre,
		Label:          cand.Label,
		ReleaseType:    cand.Format,
		ReleaseGroupID: job.RGID,
		DiscTotal:      discCount(cand),

		Title:      tr.Title,
		Track:      tr.Number,
		TrackTotal: len(cand.Tracks),
		Disc:       tr.Disc,
		ISRC:       tr.ISRC,

		MBTrackID:  mb.recording(context.Background(), tr.ISRC, mbWait),
		MBArtistID: mb.artistID(),
		MBAlbumID:  mb.releaseID(),

		Cover: cover,
	}
}

// WriteCover stores cover art beside the tracks too: media servers that scan
// directories rather than tags pick it up from here.
func WriteCover(dir string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "cover.jpg"), data, 0o644)
}

// tagSummary is the per-track log line: the identifiers that differ per file.
// Album fields were printed once before the first track; repeating them on every
// file would bury the lines that change.
func tagSummary(a postprocess.Artifact) string {
	parts := []string{}
	if a.ISRC != "" {
		parts = append(parts, "ISRC "+a.ISRC)
	} else {
		parts = append(parts, "no ISRC")
	}
	if a.MBTrackID != "" {
		parts = append(parts, "mbid "+shortID(a.MBTrackID))
	} else {
		parts = append(parts, "mbid —")
	}
	if a.Track > 0 {
		parts = append(parts, fmt.Sprintf("track %d/%d", a.Track, a.TrackTotal))
	}
	return strings.Join(parts, " · ")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// discCount is the highest disc number in the album, minimum 1.
func discCount(cand downloader.AlbumCandidate) int {
	discs := 1
	for _, t := range cand.Tracks {
		if t.Disc > discs {
			discs = t.Disc
		}
	}
	return discs
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
