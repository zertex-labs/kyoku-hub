package deezer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// ArtistAlbums implements downloader.ArtistSource.
//
// Deezer's catalogue is keyed on the name it was licensed under, which is often
// not the name a user types: 椎名林檎 is filed as "Sheena Ringo" and 三毒史 as
// "triviṣa-itihāsa". A text search for the Japanese pair returns an empty set,
// so the fallback resolves the artist first — Deezer's own search does know the
// alias — and then lists that artist's albums, ranking the right title to the
// top when the spellings happen to agree.
func (d *Deezer) ArtistAlbums(ctx context.Context, artist, title string) ([]downloader.SearchResult, error) {
	if artist == "" {
		return nil, fmt.Errorf("artist required")
	}

	id, name, err := d.findArtist(artist)
	if err != nil {
		return nil, err
	}

	albums, err := d.artistAlbums(id)
	if err != nil {
		return nil, err
	}

	results := make([]downloader.SearchResult, 0, len(albums))
	for _, a := range albums {
		year := a.ReleaseDate
		if len(year) > 4 {
			year = year[:4]
		}
		// A discography is mostly singles; they are noise next to the album the
		// user came for unless the title actually matches one.
		rank := downloader.TitleRank(title, a.Title)
		if (a.RecordType == "single" || a.RecordType == "") && rank == 0 {
			continue
		}
		results = append(results, downloader.SearchResult{
			Provider:   "deezer",
			ID:         strconv.Itoa(a.ID),
			Title:      a.Title,
			Artist:     name,
			CoverURL:   CoverURL(a.MD5Image, 250),
			TrackCount: a.NbTracks,
			Year:       year,
			Quality:    d.tier(),
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		ri := downloader.TitleRank(title, results[i].Title)
		rj := downloader.TitleRank(title, results[j].Title)
		if ri != rj {
			return ri > rj
		}
		return results[i].Year > results[j].Year
	})

	// The artist listing carries no track count, and a picker that shows
	// "0 tracks" is worse than one that shows nothing — fill it in for the
	// handful of rows the UI will actually render.
	for i := range results {
		if i >= 12 {
			break
		}
		results[i].TrackCount = d.albumTrackCount(results[i].ID)
	}

	return results, nil
}

type artistHit struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	NbAlbum int    `json:"nb_album"`
}

// findArtist picks the artist to walk. An exact name match wins; otherwise the
// top hit is used, since Deezer resolves the alias (and returned 椎名林檎's
// profile first for the Japanese query).
func (d *Deezer) findArtist(artist string) (int, string, error) {
	body, err := d.publicAPICall("/search/artist?q=" + url.QueryEscape(artist) + "&limit=10")
	if err != nil {
		return 0, "", fmt.Errorf("artist search: %w", err)
	}
	var resp struct {
		Data []artistHit `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, "", fmt.Errorf("artist search: parse: %w", err)
	}
	if len(resp.Data) == 0 {
		return 0, "", fmt.Errorf("artist not found: %s", artist)
	}
	for _, a := range resp.Data {
		if downloader.ArtistMatches(artist, a.Name) {
			return a.ID, a.Name, nil
		}
	}
	return resp.Data[0].ID, resp.Data[0].Name, nil
}

type artistAlbum struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	MD5Image    string `json:"md5_image"`
	ReleaseDate string `json:"release_date"`
	RecordType  string `json:"record_type"`
	NbTracks    int    `json:"nb_tracks"`
}

func (d *Deezer) artistAlbums(artistID int) ([]artistAlbum, error) {
	body, err := d.publicAPICall(fmt.Sprintf("/artist/%d/albums?limit=200", artistID))
	if err != nil {
		return nil, fmt.Errorf("artist albums: %w", err)
	}
	var resp struct {
		Data []artistAlbum `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("artist albums: parse: %w", err)
	}
	return resp.Data, nil
}

// albumTrackCount is a best-effort track count; zero means unknown and the UI
// omits it.
func (d *Deezer) albumTrackCount(albumID string) int {
	body, err := d.publicAPICall("/album/" + albumID)
	if err != nil {
		return 0
	}
	var album AlbumResponse
	if err := json.Unmarshal(body, &album); err != nil {
		return 0
	}
	return album.NbTracks
}

// tier is the quality label the account can reach, for display only.
func (d *Deezer) tier() string {
	switch {
	case d.auth.CanStreamLossless:
		return "LOSSLESS"
	case d.auth.CanStreamHQ:
		return "MP3_320"
	default:
		return "MP3_128"
	}
}
