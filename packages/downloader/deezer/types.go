package deezer

import (
	"encoding/json"
	"fmt"
)

type ArtistRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type AlbumRef struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	MD5Image string `json:"md5_image,omitempty"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type GenreList struct {
	Data []Genre `json:"data"`
}

type TrackList struct {
	Data []TrackResponse `json:"data"`
}

type TrackResponse struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	ISRC          string    `json:"isrc"`
	Duration      int       `json:"duration"`
	TrackPosition int       `json:"track_position"`
	DiskNumber    int       `json:"disk_number"`
	ReleaseDate   string    `json:"release_date"`
	Artist        ArtistRef `json:"artist"`
	Album         AlbumRef  `json:"album"`
}

type AlbumResponse struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	MD5Image    string    `json:"md5_image"`
	Genres      GenreList `json:"genres"`
	NbTracks    int       `json:"nb_tracks"`
	ReleaseDate string    `json:"release_date"`
	RecordType  string    `json:"record_type"`
	Label       string    `json:"label"`
	Artist      ArtistRef `json:"artist"`
}

type PublicAPIError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type GWResponse struct {
	Error   json.RawMessage `json:"error"`
	Results json.RawMessage `json:"results"`
}

func (r *GWResponse) GWErrorMap() map[string]string {
	if len(r.Error) == 0 {
		return nil
	}
	var errMap map[string]string
	if err := json.Unmarshal(r.Error, &errMap); err != nil {
		return nil
	}
	if len(errMap) == 0 {
		return nil
	}
	return errMap
}

type GWSongData struct {
	SNG_ID             string `json:"SNG_ID"`
	SNG_TITLE          string `json:"SNG_TITLE"`
	ALB_ID             string `json:"ALB_ID"`
	ALB_TITLE          string `json:"ALB_TITLE"`
	ART_NAME           string `json:"ART_NAME"`
	MD5_ORIGIN         string `json:"MD5_ORIGIN"`
	MEDIA_VERSION      string `json:"MEDIA_VERSION"`
	TRACK_TOKEN        string `json:"TRACK_TOKEN"`
	TRACK_TOKEN_EXPIRE int64  `json:"TRACK_TOKEN_EXPIRE"`
	ISRC               string `json:"ISRC"`
	FILESIZE_MP3_128   string `json:"FILESIZE_MP3_128"`
	FILESIZE_MP3_320   string `json:"FILESIZE_MP3_320"`
	FILESIZE_FLAC      string `json:"FILESIZE_FLAC"`
	COPYRIGHT          string `json:"COPYRIGHT"`
	FALLBACK           struct {
		SNG_ID        string `json:"SNG_ID"`
		MD5_ORIGIN    string `json:"MD5_ORIGIN"`
		MEDIA_VERSION string `json:"MEDIA_VERSION"`
		TRACK_TOKEN   string `json:"TRACK_TOKEN"`
	} `json:"FALLBACK"`
}

type GWUserData struct {
	USER struct {
		USER_ID int64 `json:"USER_ID"`
		OPTIONS struct {
			LicenseToken string `json:"license_token"`
			WebLossless  bool   `json:"web_lossless"`
			WebHQ        bool   `json:"web_hq"`
		} `json:"OPTIONS"`
	} `json:"USER"`
	CheckForm string `json:"checkForm"`
	SessionID string `json:"SESSION_ID"`
}

type MediaRequest struct {
	LicenseToken string       `json:"license_token"`
	Media        []MediaTrack `json:"media"`
	TrackTokens  []string     `json:"track_tokens"`
}

type MediaTrack struct {
	Type    string        `json:"type"`
	Formats []MediaFormat `json:"formats"`
}

type MediaFormat struct {
	Cipher string `json:"cipher"`
	Format string `json:"format"`
}

type MediaResponse struct {
	Data []MediaData `json:"data"`
}

type MediaData struct {
	Media  []MediaSource `json:"media"`
	Errors []MediaError  `json:"errors"`
}

type MediaError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type MediaSource struct {
	Format  string     `json:"format"`
	Sources []MediaURL `json:"sources"`
}

type MediaURL struct {
	URL      string `json:"url"`
	Provider string `json:"provider"`
}

type Auth struct {
	ARL               string `json:"arl"`
	SessionID         string `json:"session_id"`
	LicenseToken      string `json:"license_token"`
	APIToken          string `json:"api_token"`
	UserID            string `json:"user_id"`
	CanStreamLossless bool   `json:"can_stream_lossless"`
	CanStreamHQ       bool   `json:"can_stream_hq"`
}

type GeoRestrictedError struct {
	Code    int
	Message string
}

func (e *GeoRestrictedError) Error() string {
	return fmt.Sprintf("geo-restricted (code %d): %s", e.Code, e.Message)
}
