package tidal

import (
	"encoding/xml"
	"regexp"
	"time"
)

// URL patterns for matching Tidal links
var (
	trackPattern  = regexp.MustCompile(`tidal\.com.*?/track/(\d+)`)
	albumPattern  = regexp.MustCompile(`tidal\.com.*?/album/(\d+)`)
	artistPattern = regexp.MustCompile(`tidal\.com.*?/artist/(\d+)`)
)

// --- API Response Types ---

type Artist struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Album struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	Cover          string `json:"cover,omitempty"`
	NumberOfTracks int    `json:"numberOfTracks"`
}

type TrackMetadata struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Duration     int    `json:"duration"`
	TrackNumber  int    `json:"trackNumber"`
	VolumeNumber int    `json:"volumeNumber"`
	AudioQuality string `json:"audioQuality"`
	Artist       Artist `json:"artist"`
	Album        Album  `json:"album"`
	Copyright    string `json:"copyright,omitempty"`
	ISRC         string `json:"isrc,omitempty"`
}

type AlbumMetadata struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	NumberOfTracks  int    `json:"numberOfTracks"`
	NumberOfVolumes int    `json:"numberOfVolumes"`
	ReleaseDate     string `json:"releaseDate"`
	Type            string `json:"type,omitempty"`
	AudioQuality    string `json:"audioQuality"`
	Artist          Artist `json:"artist"`
	Cover           string `json:"cover,omitempty"`
	UPC             string `json:"upc,omitempty"`
}

type PlaybackInfo struct {
	TrackID          int    `json:"trackId"`
	AudioQuality     string `json:"audioQuality"`
	ManifestMimeType string `json:"manifestMimeType"`
	Manifest         string `json:"manifest"`
}

type StreamManifest struct {
	MimeType       string   `json:"mimeType"`
	Codecs         string   `json:"codecs,omitempty"`
	EncryptionType string   `json:"encryptionType"`
	URLs           []string `json:"urls"`
}

// --- Auth Types ---

type Auth struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	UserID       string `json:"user_id"`
	CountryCode  string `json:"country_code"`
}

// IsTokenExpired checks whether the access token has expired (with 5-min buffer).
func (a *Auth) IsTokenExpired() bool {
	if a.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() >= a.ExpiresAt-300
}

// --- DASH MPD Types ---

type MPD struct {
	XMLName xml.Name    `xml:"MPD"`
	Periods []MPDPeriod `xml:"Period"`
}

type MPDPeriod struct {
	AdaptationSets []MPDAdaptationSet `xml:"AdaptationSet"`
}

type MPDAdaptationSet struct {
	MimeType        string              `xml:"mimeType,attr"`
	Representations []MPDRepresentation `xml:"Representation"`
}

type MPDRepresentation struct {
	ID              string              `xml:"id,attr"`
	Codecs          string              `xml:"codecs,attr"`
	BaseURLs        []string            `xml:"BaseURL"`
	SegmentTemplate *MPDSegmentTemplate `xml:"SegmentTemplate"`
}

type MPDSegmentTemplate struct {
	Initialization string              `xml:"initialization,attr"`
	Media          string              `xml:"media,attr"`
	StartNumber    int                 `xml:"startNumber,attr"`
	Timeline       *MPDSegmentTimeline `xml:"SegmentTimeline"`
}

type MPDSegmentTimeline struct {
	Segments []MPDSegment `xml:"S"`
}

type MPDSegment struct {
	D int `xml:"d,attr"`
	R int `xml:"r,attr"`
}
