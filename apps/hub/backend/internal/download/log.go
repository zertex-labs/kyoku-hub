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
func metadataLines(job *Job, cand downloader.AlbumCandidate, cover []byte, chain *postprocess.Chain) []string {
	var album []string
	if cand.Title != "" || cand.Artist != "" {
		album = append(album, strings.TrimSpace(cand.Artist+" — "+cand.Title))
	}
	if cand.Format != "" {
		album = append(album, cand.Format)
	}
	if d := dateFor(job, cand); d != "" {
		album = append(album, d)
	}
	if cand.Genre != "" {
		album = append(album, cand.Genre)
	}
	if cand.Label != "" {
		album = append(album, cand.Label)
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
		lines = append(lines, "mb · release group "+job.RGID)
	}
	return lines
}
