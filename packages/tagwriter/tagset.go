package tagwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	id3v2 "github.com/bogem/id3v2/v2"
	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

// Tag is one canonical tag and its values.
//
// The keys are Picard's, in the order they should be written, and values are a
// slice because "GENRE=j-pop" and "GENRE=j-rock" are both true. This is the shape
// that crosses the hub/agent boundary: the hub decides the values, the agent
// writes them into whichever container the file turns out to be.
type Tag struct {
	Key    string
	Values []string
}

// WriteTags writes a tag set into path, picking the container from the extension.
func WriteTags(path string, tags []Tag) error {
	switch {
	case strings.EqualFold(filepath.Ext(path), ".flac"):
		return writeFLACTagSet(path, tags)
	case strings.EqualFold(filepath.Ext(path), ".mp3"):
		return writeMP3TagSet(path, tags)
	default:
		return fmt.Errorf("tagwriter: no tagger for %s", filepath.Ext(path))
	}
}

// WriteCover embeds a front cover into path, if the container supports it.
func WriteCover(path string, img []byte) error {
	if len(img) == 0 {
		return nil
	}
	switch {
	case strings.EqualFold(filepath.Ext(path), ".flac"):
		return EmbedFLACCover(path, img)
	case strings.EqualFold(filepath.Ext(path), ".mp3"):
		return embedMP3Cover(path, img)
	default:
		return fmt.Errorf("tagwriter: no picture writer for %s", filepath.Ext(path))
	}
}

// WriteCoverFile drops cover.jpg beside the tracks, for players that read folder
// artwork instead of embedded artwork.
func WriteCoverFile(dir string, img []byte) error {
	if len(img) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "cover.jpg"), img, 0o644)
}

// TagSet renders an artifact as the canonical tag list. This is what the hub
// sends; ordering here is the order it appears in the file.
func (a *Artifact) TagSet() []Tag {
	var tags []Tag
	add := func(key, value string) {
		if value != "" {
			tags = append(tags, Tag{Key: key, Values: []string{value}})
		}
	}

	add("TITLE", a.Title)
	add("ARTIST", firstNonEmpty(a.Artist, a.AlbumArtist))
	add("ALBUMARTIST", firstNonEmpty(a.AlbumArtist, a.Artist))
	add("ALBUM", a.Album)
	add("TRACKNUMBER", fmt.Sprintf("%d", a.Track))
	// Both spellings: players disagree about which one is the total, and writing
	// one leaves the other group showing "3/?".
	add("TRACKTOTAL", fmt.Sprintf("%d", a.TrackTotal))
	add("TOTALTRACKS", fmt.Sprintf("%d", a.TrackTotal))
	add("DISCNUMBER", fmt.Sprintf("%d", maxInt(a.Disc, 1)))
	add("DISCTOTAL", fmt.Sprintf("%d", maxInt(a.DiscTotal, 1)))
	add("TOTALDISCS", fmt.Sprintf("%d", maxInt(a.DiscTotal, 1)))

	genres := make([]string, 0, len(a.Genres))
	for _, g := range a.Genres {
		if g != "" {
			genres = append(genres, g)
		}
	}
	if len(genres) > 0 {
		tags = append(tags, Tag{Key: "GENRE", Values: genres})
	}

	add("DATE", a.Year)
	add("ORIGINALDATE", a.OriginalDate)
	add("ISRC", a.ISRC)
	add("LABEL", a.Label)
	add("CATALOGNUMBER", a.CatalogNumber)
	add("BARCODE", a.Barcode)
	add("RELEASETYPE", a.ReleaseType)
	add("RELEASESTATUS", a.ReleaseStatus)
	add("RELEASECOUNTRY", a.ReleaseCountry)
	add("MUSICBRAINZ_TRACKID", a.MBTrackID)
	add("MUSICBRAINZ_ARTISTID", a.MBArtistID)
	add("MUSICBRAINZ_ALBUMARTISTID", a.MBAlbumArtistID)
	add("MUSICBRAINZ_ALBUMID", firstNonEmpty(a.MBAlbumID, a.MBReleaseID))
	// The tag the fs agent matches on: without it the file lands in the right
	// folder and the library still reports the release group as missing.
	add("MUSICBRAINZ_RELEASEGROUPID", a.ReleaseGroupID)
	add("ARTISTSORT", a.ArtistSort)
	add("ALBUMARTISTSORT", a.AlbumArtistSort)

	return tags
}

// writeFLACTagSet replaces the keys kyoku owns and leaves every other comment
// alone: a library tagged by another tool keeps its COMPOSER, LANGUAGE and
// PERFORMER fields.
func writeFLACTagSet(path string, tags []Tag) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse flac: %w", err)
	}

	var block *flac.MetaDataBlock
	idx := -1
	for i, m := range f.Meta {
		if m.Type == flac.VorbisComment {
			block, idx = m, i
		}
	}

	var existing []string
	if block != nil {
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*block)
		if err == nil {
			existing = cmt.Comments
		}
	}

	owned := map[string]bool{}
	for _, t := range tags {
		owned[strings.ToUpper(t.Key)] = true
	}
	for _, k := range ownedKeys {
		owned[k] = true
	}

	// Keep foreign comments in the order they had.
	out := make([]string, 0, len(existing)+len(tags))
	for _, c := range existing {
		kv := strings.SplitN(c, "=", 2)
		if len(kv) == 2 && owned[strings.ToUpper(kv[0])] {
			continue
		}
		out = append(out, c)
	}
	for _, t := range tags {
		for _, v := range t.Values {
			if v == "" {
				continue
			}
			out = append(out, t.Key+"="+v)
		}
	}

	cmt := flacvorbis.New()
	for _, c := range out {
		kv := strings.SplitN(c, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if err := cmt.Add(kv[0], kv[1]); err != nil {
			return fmt.Errorf("add comment %q: %w", c, err)
		}
	}
	newBlock := cmt.Marshal()

	if idx >= 0 {
		f.Meta[idx] = &newBlock
	} else {
		f.Meta = append(f.Meta, &newBlock)
	}
	if err := f.Save(path); err != nil {
		return fmt.Errorf("save flac: %w", err)
	}
	return nil
}

// id3TextFrames maps canonical keys onto the ID3v2.4 frames Picard writes.
var id3TextFrames = map[string]string{
	"TITLE":           "TIT2",
	"ARTIST":          "TPE1",
	"ALBUMARTIST":     "TPE2",
	"ALBUM":           "TALB",
	"DATE":            "TDRC",
	"ORIGINALDATE":    "TDOR",
	"ISRC":            "TSRC",
	"LABEL":           "TPUB",
	"ARTISTSORT":      "TSOP",
	"ALBUMARTISTSORT": "TSO2",
}

// id3TXXXFrames are the fields Picard stores as user-defined text, described
// exactly as Picard describes them so the two tools read each other's files.
var id3TXXXFrames = map[string]string{
	"MUSICBRAINZ_ALBUMID":        "MusicBrainz Album Id",
	"MUSICBRAINZ_RELEASEGROUPID": "MusicBrainz Release Group Id",
	"MUSICBRAINZ_ARTISTID":       "MusicBrainz Artist Id",
	"MUSICBRAINZ_ALBUMARTISTID":  "MusicBrainz Album Artist Id",
	"RELEASETYPE":                "MusicBrainz Album Type",
	"RELEASESTATUS":              "MusicBrainz Album Status",
	"RELEASECOUNTRY":             "MusicBrainz Album Release Country",
	"CATALOGNUMBER":              "CATALOGNUMBER",
	"BARCODE":                    "BARCODE",
}

func writeMP3TagSet(path string, tags []Tag) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open mp3: %w", err)
	}
	defer tag.Close()

	// v2.4 is what Picard writes by default and the only version with TDRC.
	tag.SetVersion(4)
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)

	for _, id := range ownedMP3Frames {
		tag.DeleteFrames(id)
	}

	values := map[string][]string{}
	for _, t := range tags {
		values[strings.ToUpper(t.Key)] = t.Values
	}
	first := func(key string) string {
		if v := values[key]; len(v) > 0 {
			return v[0]
		}
		return ""
	}

	for key, frame := range id3TextFrames {
		if v := first(key); v != "" {
			tag.AddTextFrame(frame, id3v2.EncodingUTF8, v)
		}
	}

	// A genre and an artist list are multi-valued in v2.4: NUL-separated.
	if g := values["GENRE"]; len(g) > 0 {
		tag.AddTextFrame("TCON", id3v2.EncodingUTF8, strings.Join(g, "\x00"))
	}
	if a := values["ARTIST"]; len(a) > 1 {
		tag.AddTextFrame("TPE1", id3v2.EncodingUTF8, strings.Join(a, "\x00"))
	}

	// Track and disc are one frame each ("3/13"); ID3v2 has no total field.
	set := func(frame, pos, total string) {
		if pos != "" && pos != "0" {
			if total != "" && total != "0" {
				tag.AddTextFrame(frame, id3v2.EncodingUTF8, pos+"/"+total)
				return
			}
			tag.AddTextFrame(frame, id3v2.EncodingUTF8, pos)
		}
	}
	set("TRCK", first("TRACKNUMBER"), first("TRACKTOTAL"))
	set("TPOS", first("DISCNUMBER"), first("DISCTOTAL"))

	for key, desc := range id3TXXXFrames {
		if v := first(key); v != "" {
			tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
				Encoding:    id3v2.EncodingUTF8,
				Description: desc,
				Value:       v,
			})
		}
	}

	// The recording id belongs in UFID, never in a TXXX frame: "MusicBrainz Track
	// Id" holds the release-track id, which is a different MBID.
	if id := first("MUSICBRAINZ_TRACKID"); id != "" {
		tag.AddUFIDFrame(id3v2.UFIDFrame{
			OwnerIdentifier: "http://musicbrainz.org",
			Identifier:      []byte(id),
		})
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("save mp3 tags: %w", err)
	}
	return nil
}

func embedMP3Cover(path string, img []byte) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open mp3: %w", err)
	}
	defer tag.Close()

	tag.DeleteFrames("APIC")
	mime, _, _ := ImageInfo(img)
	if mime == "" {
		mime = "image/jpeg"
	}
	tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    mime,
		PictureType: id3v2.PTFrontCover,
		Description: "Cover",
		Picture:     img,
	})
	return tag.Save()
}
