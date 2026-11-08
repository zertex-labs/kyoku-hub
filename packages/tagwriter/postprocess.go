// Package postprocess is everything that happens to a file after it lands on
// disk and before it is published: writing tags, embedding artwork, and later
// replaygain, lyrics, transcoding or renaming.
//
// Each step is a Processor with one job, so a new one is a new file with no
// edits anywhere else — and a processor that fails or does not apply to a given
// file cannot stop the download or the ones after it.
package tagwriter

import (
	"context"
	"errors"
)

// Artifact is one downloaded file plus everything known about it. Processors
// read metadata from here rather than querying providers themselves: by the time
// a file is on disk, the download already resolved all of it.
type Artifact struct {
	Path string
	Ext  string // ".flac", ".mp3" — with the dot

	// Album level.
	Album          string
	AlbumArtist    string
	Artist         string
	Year           string // YYYY-MM-DD when known, else YYYY
	Genres         []string
	Label          string
	ReleaseType    string // album | single | ep
	ReleaseStatus  string // Official, Promotion, Bootleg…
	ReleaseCountry string // ISO code of the edition
	CatalogNumber  string // the edition's catalogue number
	Barcode        string
	OriginalDate   string // first release of the group, YYYY-MM-DD
	ReleaseGroupID string
	DiscTotal      int

	// Track level.
	Title           string
	Track           int
	TrackTotal      int
	Disc            int
	ISRC            string
	MBTrackID       string // MusicBrainz recording id
	MBArtistID      string
	MBAlbumArtistID string // the release artist, which differs on compilations
	MBAlbumID       string
	MBReleaseID     string

	ArtistSort      string
	AlbumArtistSort string

	// Cover holds the album artwork, shared by every file in the album.
	Cover []byte
}

// Processor is one post-download step.
//
// Process must be idempotent: a resumed or repeated download passes over the same
// files again, and running twice must leave one set of tags and one picture, not
// two. Return Skip when the file simply is not this processor's business.
type Processor interface {
	Name() string
	Process(ctx context.Context, a *Artifact) error
}

// Outcome is what happened to one processor for one file.
type Outcome struct {
	Name string
	Err  error
}

// Skipped reports a processor that did not apply, with the reason.
func (o Outcome) Skipped() (string, bool) {
	var s skip
	if errors.As(o.Err, &s) {
		return s.reason, true
	}
	return "", false
}

// Failed reports a real failure, as opposed to a deliberate skip. Skips carry a
// per-file reason, so they cannot be compared by value.
func (o Outcome) Failed() bool {
	var s skip
	return o.Err != nil && !errors.As(o.Err, &s)
}

type skip struct{ reason string }

func (s skip) Error() string { return s.reason }

// Skip marks a processor as not applicable to this file.
func Skip(reason string) error { return skip{reason} }

type skipMarker interface{ Error() string }

// Chain runs processors in order and never stops early: one broken step must not
// cost the other tags.
type Chain struct {
	processors []Processor
}

func New(processors ...Processor) *Chain {
	return &Chain{processors: processors}
}

func (c *Chain) Names() []string {
	out := make([]string, 0, len(c.processors))
	for _, p := range c.processors {
		out = append(out, p.Name())
	}
	return out
}

func (c *Chain) Run(ctx context.Context, a *Artifact) []Outcome {
	out := make([]Outcome, 0, len(c.processors))
	for _, p := range c.processors {
		if err := ctx.Err(); err != nil {
			out = append(out, Outcome{Name: p.Name(), Err: err})
			return out
		}
		out = append(out, Outcome{Name: p.Name(), Err: p.Process(ctx, a)})
	}
	return out
}

// Default is the chain a download runs today. Order matters: tags first, so a
// later processor can read what was written.
func Default() *Chain {
	return New(Tags{}, Cover{})
}

// Configured builds the built-in chain for a user's tagger options. Tags always
// run — the album's metadata is the point — while artwork is what people turn
// off, either because the files carry their own or because the folder already has
// a cover.jpg from another tool.
func Configured(embedCover, writeCoverFile bool) *Chain {
	processors := []Processor{Tags{}}
	if embedCover || writeCoverFile {
		processors = append(processors, Cover{NoEmbed: !embedCover, NoFile: !writeCoverFile})
	}
	return New(processors...)
}
