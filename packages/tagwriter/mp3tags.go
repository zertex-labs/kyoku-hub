package tagwriter

import (
	"fmt"
	"strings"

	id3v2 "github.com/bogem/id3v2/v2"
)

// The ID3 side of the same idea as the FLAC writer: the exact frames Picard
// writes, so a file tagged by kyoku reads the same in Picard and vice versa.
//
// Two things are easy to get wrong and are worth stating:
//
//   - The recording id belongs in UFID (owner http://musicbrainz.org), NOT in
//     "MusicBrainz Track Id". That TXXX frame holds the *release-track* id, a
//     different MBID; swapping them makes every file claim a track it is not.
//   - Track and disc numbers are one frame each ("3/13", "1/2"), not separate
//     totals. ID3v2 has no total field.
func WriteMP3Tags(path string, a *Artifact) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open mp3: %w", err)
	}
	defer tag.Close()

	// v2.4 is what Picard writes by default and the only version with TDRC.
	tag.SetVersion(4)
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)

	// Replace our own frames instead of stacking duplicates on a re-run.
	for _, id := range ownedMP3Frames {
		tag.DeleteFrames(id)
	}

	tag.SetTitle(a.Title)
	tag.SetAlbum(a.Album)
	tag.SetArtist(firstNonEmpty(a.Artist, a.AlbumArtist))
	tag.SetGenre(strings.Join(a.Genres, "\x00")) // multi-value in v2.4
	tag.SetYear(a.Year)

	set := func(id, value string) {
		if value != "" {
			tag.AddTextFrame(id, id3v2.EncodingUTF8, value)
		}
	}

	set("TPE2", firstNonEmpty(a.AlbumArtist, a.Artist))
	set("TSRC", a.ISRC)
	set("TPUB", a.Label)
	set("TDOR", a.OriginalDate)
	set("TSOP", a.ArtistSort)
	set("TSO2", a.AlbumArtistSort)
	set("TRCK", numberPair(a.Track, a.TrackTotal))
	set("TPOS", numberPair(maxInt(a.Disc, 1), maxInt(a.DiscTotal, 1)))

	// TXXX frames, described exactly as Picard describes them.
	txxx := []struct{ desc, value string }{
		{"MusicBrainz Album Id", firstNonEmpty(a.MBAlbumID, a.MBReleaseID)},
		{"MusicBrainz Release Group Id", a.ReleaseGroupID},
		{"MusicBrainz Artist Id", a.MBArtistID},
		{"MusicBrainz Album Artist Id", a.MBAlbumArtistID},
		{"MusicBrainz Album Type", a.ReleaseType},
		{"MusicBrainz Album Status", a.ReleaseStatus},
		{"MusicBrainz Album Release Country", a.ReleaseCountry},
		{"CATALOGNUMBER", a.CatalogNumber},
		{"BARCODE", a.Barcode},
	}
	for _, f := range txxx {
		if f.value == "" {
			continue
		}
		tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
			Encoding:    id3v2.EncodingUTF8,
			Description: f.desc,
			Value:       f.value,
		})
	}

	if a.MBTrackID != "" {
		tag.AddUFIDFrame(id3v2.UFIDFrame{
			OwnerIdentifier: "http://musicbrainz.org",
			Identifier:      []byte(a.MBTrackID),
		})
	}

	if len(a.Cover) > 0 {
		mime, _, _ := ImageInfo(a.Cover)
		if mime == "" {
			mime = "image/jpeg"
		}
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    mime,
			PictureType: id3v2.PTFrontCover,
			Description: "Cover",
			Picture:     a.Cover,
		})
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("save mp3 tags: %w", err)
	}
	return nil
}

// ownedMP3Frames are the frames this writer replaces. Custom TXXX frames are
// removed by description rather than wholesale, so a plugin's own fields survive.
var ownedMP3Frames = []string{
	"TIT2", "TALB", "TPE1", "TPE2", "TCON", "TDRC", "TDOR", "TSRC", "TPUB",
	"TSOP", "TSO2", "TRCK", "TPOS", "UFID",
}

// numberPair renders ID3's "position/total" form, and just the position when the
// total is unknown — "3/0" is worse than "3".
func numberPair(pos, total int) string {
	if pos <= 0 {
		return ""
	}
	if total <= 0 {
		return fmt.Sprintf("%d", pos)
	}
	return fmt.Sprintf("%d/%d", pos, total)
}
