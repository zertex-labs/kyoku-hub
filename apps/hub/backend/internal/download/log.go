package download

import (
	"fmt"
	"strings"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download/postprocess"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/settings"
	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// metadataLines reports what the post-processors are about to write, once per
// album. The per-track details follow as tracks download; this is the "what is
// going into these files" header, so a user never has to open a tag editor.
// coverPlacement says where the artwork goes, so the log matches what settings
// asked for instead of always promising both.
func coverPlacement(o settings.Options) string {
	switch {
	case o.EmbedCover && o.WriteCoverFile:
		return "embedded in every track + cover.jpg"
	case o.EmbedCover:
		return "embedded in every track"
	case o.WriteCoverFile:
		return "cover.jpg beside the tracks"
	default:
		return ""
	}
}

func metadataLines(job *Job, cand downloader.AlbumCandidate, mb *mbInfo, cover []byte, chain *postprocess.Chain, opts settings.Options) []string {
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
	if genres := mb.genres(); len(genres) > 0 {
		album = append(album, strings.Join(genres, ", "))
	} else if cand.Genre != "" {
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
		where := coverPlacement(opts)
		if where == "" {
			lines = append(lines, fmt.Sprintf("cover · %s%s · written nowhere, artwork is off in settings", size, mime))
		} else {
			lines = append(lines, fmt.Sprintf("cover · %s%s · %s", size, mime, where))
		}
	} else if job.Kind == KindRetag {
		// A re-tag was never going to ask a provider: it looked at the disk.
		lines = append(lines, "cover · none — this album has no artwork in the folder or in its files")
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
