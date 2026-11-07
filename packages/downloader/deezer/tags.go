package deezer

import (
	"encoding/json"
	"fmt"
	"strings"

	flacvorbis "github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

type albumMeta struct {
	Title       string
	AlbumArtist string
	Year        string
	ReleaseDate string // full ISO date when Deezer has one, else ""
	Genre       string
	Label       string
	RecordType  string
	TotalTracks int
	TotalDiscs  int
}

func (d *Deezer) getAlbumMeta(albumID string, tracks []internalTrack) albumMeta {
	body, err := d.publicAPICall("/album/" + albumID)
	if err != nil {
		return albumMeta{TotalTracks: len(tracks), TotalDiscs: maxDisc(tracks)}
	}
	var resp AlbumResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return albumMeta{TotalTracks: len(tracks), TotalDiscs: maxDisc(tracks)}
	}
	year := resp.ReleaseDate
	if len(year) > 4 {
		year = year[:4]
	}
	fullDate := resp.ReleaseDate
	if len(fullDate) < 4 {
		fullDate = ""
	}
	genre := ""
	if len(resp.Genres.Data) > 0 {
		genre = resp.Genres.Data[0].Name
	}
	return albumMeta{
		Title:       resp.Title,
		AlbumArtist: resp.Artist.Name,
		Year:        year,
		ReleaseDate: fullDate,
		Genre:       genre,
		Label:       resp.Label,
		RecordType:  resp.RecordType,
		TotalTracks: len(tracks),
		TotalDiscs:  maxDisc(tracks),
	}
}

func maxDisc(tracks []internalTrack) int {
	m := 1
	for _, t := range tracks {
		if t.DiscNumber > m {
			m = t.DiscNumber
		}
	}
	return m
}

func tagFlacTrack(path string, track internalTrack, song GWSongData, meta albumMeta) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return err
	}

	tags := []string{
		"TITLE=" + song.SNG_TITLE,
		"ARTIST=" + song.ART_NAME,
		"ALBUMARTIST=" + meta.AlbumArtist,
		"ALBUM=" + meta.Title,
		"TRACKNUMBER=" + fmt.Sprintf("%d", track.TrackNumber),
		"TRACKTOTAL=" + fmt.Sprintf("%d", meta.TotalTracks),
		"DISCNUMBER=" + fmt.Sprintf("%d", track.DiscNumber),
		"DISCTOTAL=" + fmt.Sprintf("%d", meta.TotalDiscs),
		"DATE=" + meta.Year,
	}
	if meta.Genre != "" {
		tags = append(tags, "GENRE="+meta.Genre)
	}
	if song.ISRC != "" {
		tags = append(tags, "ISRC="+song.ISRC)
	}
	if song.COPYRIGHT != "" {
		tags = append(tags, "COPYRIGHT="+song.COPYRIGHT)
	}

	keepKey := func(c string) bool {
		upper := strings.ToUpper(c)
		skip := []string{"TITLE=", "ARTIST=", "ALBUMARTIST=", "ALBUM=", "TRACKNUMBER=",
			"TRACKTOTAL=", "DISCNUMBER=", "DISCTOTAL=", "DATE=", "GENRE=", "ISRC=", "COPYRIGHT="}
		for _, k := range skip {
			if strings.HasPrefix(upper, k) {
				return false
			}
		}
		return true
	}

	for i, m := range f.Meta {
		if m.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*m)
		if err != nil {
			return err
		}
		filtered := cmt.Comments[:0]
		for _, c := range cmt.Comments {
			if keepKey(c) {
				filtered = append(filtered, c)
			}
		}
		cmt.Comments = append(filtered, tags...)
		block := cmt.Marshal()
		f.Meta[i] = &block
		return f.Save(path)
	}

	cmt := flacvorbis.New()
	cmt.Comments = tags
	block := cmt.Marshal()
	f.Meta = append(f.Meta, &block)
	return f.Save(path)
}
