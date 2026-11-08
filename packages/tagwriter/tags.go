package tagwriter

import (
	"context"
	"fmt"
	"strings"

	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

// Tags writes the album metadata into whatever container the file uses.
//
// The key names follow MusicBrainz Picard exactly, per format — Vorbis comments
// for FLAC, ID3v2 for MP3 — because the point of these tags is that other tools
// recognise them. A recording id goes in MUSICBRAINZ_TRACKID on FLAC and in the
// UFID frame on MP3, never in "MusicBrainz Track Id": Picard uses that one for
// the release-track id, which is a different identifier.
type Tags struct{}

func (Tags) Name() string { return "tags" }

func (t Tags) Process(ctx context.Context, a *Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case strings.EqualFold(a.Ext, ".flac"):
		return WriteFLACTags(a.Path, a)
	case strings.EqualFold(a.Ext, ".mp3"):
		return WriteMP3Tags(a.Path, a)
	default:
		return Skip("no tagger for " + a.Ext + " yet")
	}
}

// vorbisPairs renders an artifact as the KEY=VALUE lines a FLAC comment block
// holds. Album-level fields are repeated per file on purpose: a tag editor treats
// every file as independent, and a music library does not know our job model.
func vorbisPairs(a *Artifact) []string {
	pairs := []string{
		"TITLE=" + a.Title,
		"ARTIST=" + firstNonEmpty(a.Artist, a.AlbumArtist),
		"ALBUMARTIST=" + firstNonEmpty(a.AlbumArtist, a.Artist),
		"ALBUM=" + a.Album,
		fmt.Sprintf("TRACKNUMBER=%d", a.Track),
		// Both spellings: players disagree about which one is the total, and
		// writing one leaves the other group of players showing "3/?".
		fmt.Sprintf("TRACKTOTAL=%d", a.TrackTotal),
		fmt.Sprintf("TOTALTRACKS=%d", a.TrackTotal),
		fmt.Sprintf("DISCNUMBER=%d", maxInt(a.Disc, 1)),
		fmt.Sprintf("DISCTOTAL=%d", maxInt(a.DiscTotal, 1)),
		fmt.Sprintf("TOTALDISCS=%d", maxInt(a.DiscTotal, 1)),
	}
	// Genre is the one multi-valued field: MusicBrainz carries "j-pop" and
	// "j-rock" as separate votes, and both are true.
	for _, g := range a.Genres {
		if g != "" {
			pairs = append(pairs, "GENRE="+g)
		}
	}
	optional := []struct{ key, value string }{
		{"DATE", a.Year},
		{"ORIGINALDATE", a.OriginalDate},
		{"ISRC", a.ISRC},
		{"LABEL", a.Label},
		{"CATALOGNUMBER", a.CatalogNumber},
		{"BARCODE", a.Barcode},
		{"RELEASETYPE", a.ReleaseType},
		{"RELEASESTATUS", a.ReleaseStatus},
		{"RELEASECOUNTRY", a.ReleaseCountry},
		{"MUSICBRAINZ_TRACKID", a.MBTrackID},
		{"MUSICBRAINZ_ARTISTID", a.MBArtistID},
		{"MUSICBRAINZ_ALBUMARTISTID", a.MBAlbumArtistID},
		{"MUSICBRAINZ_ALBUMID", firstNonEmpty(a.MBAlbumID, a.MBReleaseID)},
		// The tag the fs agent matches on: without it the file downloads into the
		// right folder and the library still reports the release group as missing.
		{"MUSICBRAINZ_RELEASEGROUPID", a.ReleaseGroupID},
		{"ARTISTSORT", a.ArtistSort},
		{"ALBUMARTISTSORT", a.AlbumArtistSort},
	}
	for _, o := range optional {
		if o.value != "" {
			pairs = append(pairs, o.key+"="+o.value)
		}
	}
	return pairs
}

// ownedKeys are the keys this writer owns; existing values are replaced, every
// other comment (REPLAYGAIN_*, custom fields) is preserved.
var ownedKeys = []string{
	"TITLE=", "ARTIST=", "ALBUMARTIST=", "ALBUM=", "TRACKNUMBER=", "TRACKTOTAL=",
	"TOTALTRACKS=", "DISCNUMBER=", "DISCTOTAL=", "TOTALDISCS=", "DATE=",
	"ORIGINALDATE=", "GENRE=", "ISRC=", "LABEL=", "CATALOGNUMBER=", "BARCODE=",
	"RELEASETYPE=", "RELEASESTATUS=", "RELEASECOUNTRY=",
	"MUSICBRAINZ_RELEASEGROUPID=", "MUSICBRAINZ_TRACKID=", "MUSICBRAINZ_ARTISTID=",
	"MUSICBRAINZ_ALBUMARTISTID=", "MUSICBRAINZ_ALBUMID=", "ARTISTSORT=",
	"ALBUMARTISTSORT=",
}

// WriteFLACTags writes the album metadata into a FLAC file, creating the Vorbis
// comment block when the file has none.
func WriteFLACTags(path string, a *Artifact) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse flac: %w", err)
	}

	pairs := vorbisPairs(a)
	for i, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*m)
		if err != nil {
			return fmt.Errorf("parse comments: %w", err)
		}
		kept := cmt.Comments[:0]
		for _, c := range cmt.Comments {
			if !ownsComment(c) {
				kept = append(kept, c)
			}
		}
		cmt.Comments = append(kept, pairs...)
		block := cmt.Marshal()
		f.Meta[i] = &block
		return f.Save(path)
	}

	cmt := flacvorbis.New()
	cmt.Comments = pairs
	block := cmt.Marshal()
	f.Meta = append(f.Meta, &block)
	return f.Save(path)
}

func ownsComment(c string) bool {
	up := strings.ToUpper(c)
	for _, k := range ownedKeys {
		if strings.HasPrefix(up, k) {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
