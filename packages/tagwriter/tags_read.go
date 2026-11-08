package tagwriter

import (
	"fmt"
	"strconv"
	"strings"

	id3v2 "github.com/bogem/id3v2/v2"
	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

// TagValues is what a file already says about itself.
//
// Re-tagging never re-downloads: the music on disk is the music, so the track
// identity — which track this is, which ISRC it carries — has to come from the
// file. Everything album-level is resolved again from MusicBrainz.
type TagValues struct {
	Title          string
	Artist         string
	Album          string
	Track          int
	Disc           int
	ISRC           string
	ReleaseGroupID string
	// ReleaseTrackID is the track's id on one specific release. Picard writes it,
	// and it is enough to find the edition — which is how a re-tag recovers the
	// recording ids when MusicBrainz search is unavailable.
	ReleaseTrackID string

	// The identifiers already in the file. A re-tag keeps them when it cannot
	// resolve better ones: a throttled lookup must not erase what is there.
	MBTrackID       string
	MBAlbumID       string
	MBArtistID      string
	MBAlbumArtistID string
}

// ReadTags reads the fields a re-tag needs, for the formats we can write.
func ReadTags(path string) (TagValues, error) {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".flac"):
		return readFLACTags(path)
	case strings.HasSuffix(strings.ToLower(path), ".mp3"):
		return readMP3Tags(path)
	default:
		return TagValues{}, fmt.Errorf("no reader for %s", path)
	}
}

func readFLACTags(path string) (TagValues, error) {
	f, err := flac.ParseFile(path)
	if err != nil {
		return TagValues{}, fmt.Errorf("parse flac: %w", err)
	}
	for _, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*m)
		if err != nil {
			return TagValues{}, fmt.Errorf("parse comments: %w", err)
		}
		vals := map[string]string{}
		for _, c := range cmt.Comments {
			kv := strings.SplitN(c, "=", 2)
			if len(kv) == 2 {
				vals[strings.ToUpper(kv[0])] = kv[1]
			}
		}
		return TagValues{
			Title:           vals["TITLE"],
			Artist:          vals["ARTIST"],
			Album:           vals["ALBUM"],
			Track:           atoiSafe(vals["TRACKNUMBER"]),
			Disc:            atoiSafe(vals["DISCNUMBER"]),
			ISRC:            vals["ISRC"],
			ReleaseGroupID:  vals["MUSICBRAINZ_RELEASEGROUPID"],
			ReleaseTrackID:  vals["MUSICBRAINZ_RELEASETRACKID"],
			MBTrackID:       vals["MUSICBRAINZ_TRACKID"],
			MBAlbumID:       vals["MUSICBRAINZ_ALBUMID"],
			MBArtistID:      vals["MUSICBRAINZ_ARTISTID"],
			MBAlbumArtistID: vals["MUSICBRAINZ_ALBUMARTISTID"],
		}, nil
	}
	return TagValues{}, fmt.Errorf("no vorbis comments")
}

func readMP3Tags(path string) (TagValues, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return TagValues{}, fmt.Errorf("open mp3: %w", err)
	}
	defer tag.Close()

	out := TagValues{
		Title:  tag.Title(),
		Artist: tag.Artist(),
		Album:  tag.Album(),
		ISRC:   tag.GetTextFrame("TSRC").Text,
	}
	out.Track = atoiSafe(strings.SplitN(tag.GetTextFrame("TRCK").Text, "/", 2)[0])
	out.Disc = atoiSafe(strings.SplitN(tag.GetTextFrame("TPOS").Text, "/", 2)[0])

	for _, f := range tag.GetFrames("TXXX") {
		udtf, ok := f.(id3v2.UserDefinedTextFrame)
		if !ok {
			continue
		}
		switch udtf.Description {
		case "MusicBrainz Release Group Id":
			out.ReleaseGroupID = udtf.Value
		case "MusicBrainz Release Track Id":
			out.ReleaseTrackID = udtf.Value
		case "MusicBrainz Album Id":
			out.MBAlbumID = udtf.Value
		case "MusicBrainz Track Id":
			out.MBTrackID = udtf.Value
		case "MusicBrainz Artist Id":
			out.MBArtistID = udtf.Value
		case "MusicBrainz Album Artist Id":
			out.MBAlbumArtistID = udtf.Value
		}
	}
	return out, nil
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
