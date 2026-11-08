package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
	postprocess "github.com/zertex-labs/kyoku-hub/packages/tagwriter"
)

// mbWait is how long one track waits for its MusicBrainz id before giving up.
// Lookups run at 1 req/s in the background, so a download of any real length has
// the answer ready; this ceiling only matters on a cold, slow network.
const mbWait = 2500 * time.Millisecond

// artifact is the download's view of one file, handed to the post-processors.
// Everything they need was resolved during probe/enrich/download, so a processor
// never has to ask a provider or MusicBrainz anything.
// genres decides whose genre wins. MusicBrainz's is a listener's word for the
// music; the provider's is a marketing bucket ("Asian Music"). That is why MB is
// the default, and why the setting exists for people who disagree.
func (m *Manager) genres(job *Job, mb *mbInfo, cand downloader.AlbumCandidate) []string {
	if m.options(job).PreferProviderGenre {
		if cand.Genre != "" {
			return []string{cand.Genre}
		}
		return firstNonEmptyStrings(mb.genres(), "")
	}
	return firstNonEmptyStrings(mb.genres(), cand.Genre)
}

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
	// The chosen edition wins over the provider's licensed title: Deezer calls
	// 放生会 "Carnival", and a file tagged with that is a file nobody can find.
	album := firstNonEmptyString(mb.albumTitle(), cand.Title, job.Album.Title)

	return postprocess.Artifact{
		Path: path,
		Ext:  ext,

		Album:          album,
		AlbumArtist:    artist,
		Artist:         artist,
		Year:           firstNonEmptyString(mb.albumDate(), dateFor(job, cand)),
		Genres:         m.genres(job, mb, cand),
		Label:          firstNonEmptyString(mb.albumLabel(), cand.Label),
		ReleaseType:    firstNonEmptyString(mb.releaseType(), cand.Format),
		ReleaseStatus:  mb.releaseStatus(),
		ReleaseCountry: mb.releaseCountry(),
		CatalogNumber:  mb.catalogNumber(),
		Barcode:        mb.barcode(),
		OriginalDate:   mb.firstRelease(),
		ReleaseGroupID: job.RGID,
		DiscTotal:      discCount(cand),

		Title:      tr.Title,
		Track:      tr.Number,
		TrackTotal: len(cand.Tracks),
		Disc:       tr.Disc,
		ISRC:       tr.ISRC,

		// Position first: the edition already told us which recording sits at
		// track N, and it cannot be rate-limited. The ISRC lookup is the fallback
		// for a track the edition does not list.
		MBTrackID:       firstNonEmptyString(mb.recordingAt(tr.Disc, tr.Number), mb.recording(context.Background(), tr.ISRC, mbWait)),
		MBArtistID:      mb.artistID(),
		MBAlbumArtistID: mb.albumArtistID(),
		MBAlbumID:       mb.releaseID(),

		ArtistSort:      mb.artistSortName(),
		AlbumArtistSort: mb.artistSortName(),

		Cover: cover,
	}
}

// firstNonEmptyStrings prefers the resolved list and falls back to the single
// provider genre, so a release MusicBrainz has no genres for still gets one.
func firstNonEmptyStrings(list []string, fallback string) []string {
	if len(list) > 0 {
		return list
	}
	if fallback != "" {
		return []string{fallback}
	}
	return nil
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
