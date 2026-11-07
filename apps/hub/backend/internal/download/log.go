package download

import (
	"fmt"
	"strings"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download/postprocess"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// metadataLines reports what the post-processors are about to write, once per
// album. The per-track details follow as tracks download; this is the "what is
// going into these files" header, so a user never has to open a tag editor.
func metadataLines(job *Job, cand downloader.AlbumCandidate, mb *mbInfo, cover []byte, chain *postprocess.Chain) []string {
	// The same precedence the tagger uses, or the log would describe one edition
	// and the files would carry another.
	title := firstNonEmptyString(mb.albumTitle(), cand.Title)
	date := firstNonEmptyString(mb.albumDate(), dateFor(job, cand))
	label := firstNonEmptyString(mb.albumLabel(), cand.Label)

	var album []string
	if title != "" || cand.Artist != "" {
		album = append(album, strings.TrimSpace(cand.Artist+" — "+title))
	}
	if cand.Format != "" {
		album = append(album, cand.Format)
	}
	if date != "" {
		album = append(album, date)
	}
	if cand.Genre != "" {
		album = append(album, cand.Genre)
	}
	if label != "" {
		album = append(album, label)
	}

	lines := []string{
		"tags · " + strings.Join(album, " · "),
		"post-processors · " + strings.Join(chain.Names(), ", "),
	}

	if len(cover) > 0 {
		mime, w, h := postprocess.ImageInfo(cover)
		size := ""
		if w > 0 && h > 0 {
			size = fmt.Sprintf("%d×%d ", w, h)
		}
		lines = append(lines, fmt.Sprintf("cover · %s%s · embedded in every track + cover.jpg", size, mime))
	} else {
		lines = append(lines, "cover · none — the provider had no artwork for this album")
	}

	if job.RGID != "" {
		line := "mb · release group " + job.RGID
		if job.Album.ReleaseID != "" {
			line += " · release " + job.Album.ReleaseID
		}
		lines = append(lines, line)
	}
	return lines
}
