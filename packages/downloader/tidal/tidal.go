package tidal

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// Compile-time interface check.
var _ downloader.Provider = (*Tidal)(nil)

const (
	APIBaseURL     = "https://api.tidal.com/v1/"
	DefaultTimeout = 10 * time.Second
)

// Tidal implements the downloader.Provider interface for Tidal.
type Tidal struct {
	auth       Auth
	config     downloader.Config
	httpClient *http.Client
}

// New creates a new Tidal provider with the given auth and config.
func New(auth Auth, config downloader.Config) *Tidal {
	return &Tidal{
		auth:   auth,
		config: config,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// Name returns the service name.
func (t *Tidal) Name() string {
	return "tidal"
}

// IsAuthenticated checks if the provider has valid credentials.
func (t *Tidal) IsAuthenticated() bool {
	return t.auth.Token != "" && t.auth.Token != "YOUR_TIDAL_AUTH_TOKEN_HERE"
}

// SetAuth updates the provider's authentication credentials.
func (t *Tidal) SetAuth(auth Auth) {
	t.auth = auth
}

// EnsureValidToken checks if the token is expired and refreshes if needed.
// Returns the updated auth (with new token/expiry) if refreshed.
func (t *Tidal) EnsureValidToken() (*Auth, error) {
	if !t.auth.IsTokenExpired() {
		return nil, nil // no change needed
	}

	if t.auth.RefreshToken == "" {
		return nil, fmt.Errorf("token expired and no refresh token available")
	}

	if err := RefreshAccessToken(&t.auth); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	authCopy := t.auth
	return &authCopy, nil
}

// GetTrack fetches track metadata by ID.
func (t *Tidal) GetTrack(id string) (downloader.Track, error) {
	if err := t.ensureValidToken(); err != nil {
		return downloader.Track{}, err
	}

	metadata, err := t.getTrackMetadata(id)
	if err != nil {
		return downloader.Track{}, err
	}

	return t.metadataToTrack(metadata), nil
}

// GetAlbum fetches album metadata and tracks by ID.
func (t *Tidal) GetAlbum(id string) (downloader.Album, error) {
	if err := t.ensureValidToken(); err != nil {
		return downloader.Album{}, err
	}

	albumMeta, err := t.getAlbumMetadata(id)
	if err != nil {
		return downloader.Album{}, fmt.Errorf("failed to get album metadata: %w", err)
	}

	tracks, err := t.getAlbumTracks(id)
	if err != nil {
		return downloader.Album{}, fmt.Errorf("failed to get album tracks: %w", err)
	}

	genericTracks := make([]downloader.Track, len(tracks))
	for i, tr := range tracks {
		track := t.metadataToTrack(tr)
		track.Date = albumMeta.ReleaseDate
		track.TotalDiscs = albumMeta.NumberOfVolumes
		track.AlbumArtist = albumMeta.Artist.Name
		if track.CoverURL == "" {
			track.CoverURL = CoverURL(albumMeta.Cover, 1280)
		}
		genericTracks[i] = track
	}

	return downloader.Album{
		ID:          strconv.Itoa(albumMeta.ID),
		Title:       albumMeta.Title,
		Artist:      albumMeta.Artist.Name,
		Tracks:      genericTracks,
		Date:        albumMeta.ReleaseDate,
		TotalTracks: albumMeta.NumberOfTracks,
		TotalDiscs:  albumMeta.NumberOfVolumes,
		CoverURL:    CoverURL(albumMeta.Cover, 1280),
	}, nil
}

// GetStreamURL returns stream info for a track.
func (t *Tidal) GetStreamURL(trackID string) (downloader.StreamInfo, error) {
	if err := t.ensureValidToken(); err != nil {
		return downloader.StreamInfo{}, err
	}

	quality := t.config.Quality
	if quality == "" {
		quality = "LOSSLESS"
	}

	streamInfo, err := t.getStreamInfo(trackID, quality)
	if err != nil {
		return downloader.StreamInfo{}, err
	}

	return downloader.StreamInfo{
		URL:              streamInfo.URL,
		URLs:             streamInfo.URLs,
		MimeType:         streamInfo.MimeType,
		IsEncrypted:      false, // Tidal streams are not client-encrypted
		TrackID:          trackID,
		RequestedQuality: streamInfo.RequestedQuality,
		ActualQuality:    streamInfo.ActualQuality,
	}, nil
}

// SearchAlbum searches for albums on Tidal.
func (t *Tidal) SearchAlbum(query string) ([]downloader.SearchResult, error) {
	if err := t.ensureValidToken(); err != nil {
		return nil, err
	}

	apiPath := "search/albums"
	q := url.Values{}
	q.Set("query", query)
	q.Set("countryCode", t.auth.CountryCode)
	q.Set("limit", "10")

	body, status, err := t.performAPICall(apiPath, q)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d", status)
	}

	var resp struct {
		Items []AlbumMetadata `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	results := make([]downloader.SearchResult, len(resp.Items))
	for i, album := range resp.Items {
		results[i] = downloader.SearchResult{
			Provider:    "tidal",
			ID:          strconv.Itoa(album.ID),
			Title:       album.Title,
			Artist:      album.Artist.Name,
			CoverURL:    CoverURL(album.Cover, 320),
			TrackCount:  album.NumberOfTracks,
			ReleaseDate: album.ReleaseDate,
			Quality:     album.AudioQuality,
		}
	}

	return results, nil
}

// GetAlbumByURL extracts the album ID from a Tidal URL.
func (t *Tidal) GetAlbumByURL(tidalURL string) (string, error) {
	id := ExtractAlbumID(tidalURL)
	if id == "" {
		return "", fmt.Errorf("could not extract album ID from URL: %s", tidalURL)
	}
	return id, nil
}

// --- Internal Methods ---

func (t *Tidal) ensureValidToken() error {
	if !t.auth.IsTokenExpired() {
		return nil
	}
	if t.auth.RefreshToken == "" {
		return fmt.Errorf("token expired and no refresh token available")
	}
	return RefreshAccessToken(&t.auth)
}

func (t *Tidal) performAPICall(path string, query url.Values) ([]byte, int, error) {
	fullURL := APIBaseURL + path + "?" + query.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+t.auth.Token)
	req.Header.Set("User-Agent", "kyoku-hub/1.0")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

func (t *Tidal) getTrackMetadata(trackID string) (TrackMetadata, error) {
	q := url.Values{}
	q.Set("countryCode", t.auth.CountryCode)

	body, status, err := t.performAPICall("tracks/"+trackID, q)
	if err != nil {
		return TrackMetadata{}, err
	}
	if status != http.StatusOK {
		return TrackMetadata{}, fmt.Errorf("track metadata request failed with status %d", status)
	}

	var metadata TrackMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return TrackMetadata{}, fmt.Errorf("failed to parse track metadata: %w", err)
	}

	return metadata, nil
}

func (t *Tidal) getAlbumMetadata(albumID string) (AlbumMetadata, error) {
	q := url.Values{}
	q.Set("countryCode", t.auth.CountryCode)

	body, status, err := t.performAPICall("albums/"+albumID, q)
	if err != nil {
		return AlbumMetadata{}, err
	}
	if status != http.StatusOK {
		return AlbumMetadata{}, fmt.Errorf("album metadata request failed with status %d", status)
	}

	var metadata AlbumMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return AlbumMetadata{}, fmt.Errorf("failed to parse album metadata: %w", err)
	}

	return metadata, nil
}

func (t *Tidal) getAlbumTracks(albumID string) ([]TrackMetadata, error) {
	q := url.Values{}
	q.Set("countryCode", t.auth.CountryCode)
	q.Set("limit", "100")

	body, status, err := t.performAPICall("albums/"+albumID+"/tracks", q)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("album tracks request failed with status %d", status)
	}

	var response struct {
		Items []TrackMetadata `json:"items"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse album tracks: %w", err)
	}

	return response.Items, nil
}

func (t *Tidal) getPlaybackInfo(trackID, quality string) (PlaybackInfo, error) {
	q := url.Values{}
	q.Set("audioquality", quality)
	q.Set("playbackmode", "STREAM")
	q.Set("assetpresentation", "FULL")
	q.Set("countryCode", t.auth.CountryCode)

	body, status, err := t.performAPICall("tracks/"+trackID+"/playbackinfopostpaywall", q)
	if err != nil {
		return PlaybackInfo{}, err
	}
	if status != http.StatusOK {
		return PlaybackInfo{}, fmt.Errorf("playback info request failed with status %d: %s", status, string(body))
	}

	var playbackInfo PlaybackInfo
	if err := json.Unmarshal(body, &playbackInfo); err != nil {
		return PlaybackInfo{}, fmt.Errorf("failed to parse playback info: %w", err)
	}

	return playbackInfo, nil
}

type internalStreamInfo struct {
	URL              string
	URLs             []string
	MimeType         string
	RequestedQuality string
	ActualQuality    string
}

func (t *Tidal) getStreamInfo(trackID, quality string) (internalStreamInfo, error) {
	requestedQuality := quality
	if t.config.Quality != "" {
		requestedQuality = t.config.Quality
	}
	if requestedQuality == "" {
		requestedQuality = "LOSSLESS"
	}

	playbackInfo, err := t.getPlaybackInfo(trackID, requestedQuality)
	if err != nil {
		return internalStreamInfo{}, err
	}

	manifest, err := decodeManifest(playbackInfo)
	if err != nil {
		return internalStreamInfo{}, err
	}

	if manifest.EncryptionType != "NONE" {
		return internalStreamInfo{}, fmt.Errorf("stream is encrypted (type: %s)", manifest.EncryptionType)
	}

	if len(manifest.URLs) == 0 {
		return internalStreamInfo{}, fmt.Errorf("no stream URLs found in manifest")
	}

	return internalStreamInfo{
		URL:              manifest.URLs[0],
		URLs:             manifest.URLs,
		MimeType:         manifest.MimeType,
		RequestedQuality: requestedQuality,
		ActualQuality:    playbackInfo.AudioQuality,
	}, nil
}

func decodeManifest(playbackInfo PlaybackInfo) (StreamManifest, error) {
	decoded, err := base64.StdEncoding.DecodeString(playbackInfo.Manifest)
	if err != nil {
		return StreamManifest{}, fmt.Errorf("failed to base64 decode manifest: %w", err)
	}

	switch playbackInfo.ManifestMimeType {
	case "application/dash+xml":
		return parseDASHManifest(decoded)
	default:
		var manifest StreamManifest
		if err := json.Unmarshal(decoded, &manifest); err != nil {
			return StreamManifest{}, fmt.Errorf("failed to parse manifest JSON: %w", err)
		}
		return manifest, nil
	}
}

func parseDASHManifest(data []byte) (StreamManifest, error) {
	var mpd MPD
	if err := xml.Unmarshal(data, &mpd); err != nil {
		return StreamManifest{}, fmt.Errorf("failed to parse DASH manifest XML: %w", err)
	}

	var urls []string
	var mimeType, codecs string

	for _, period := range mpd.Periods {
		for _, adaptSet := range period.AdaptationSets {
			for _, rep := range adaptSet.Representations {
				if codecs == "" && rep.Codecs != "" {
					codecs = rep.Codecs
				}
				if mimeType == "" && adaptSet.MimeType != "" {
					mimeType = adaptSet.MimeType
				}

				if rep.SegmentTemplate != nil {
					tmpl := rep.SegmentTemplate
					if tmpl.Initialization != "" {
						urls = append(urls, tmpl.Initialization)
					}
					if tmpl.Media != "" && tmpl.Timeline != nil {
						segNum := tmpl.StartNumber
						for _, s := range tmpl.Timeline.Segments {
							count := 1 + s.R
							for i := 0; i < count; i++ {
								segURL := strings.Replace(tmpl.Media, "$Number$", strconv.Itoa(segNum), 1)
								urls = append(urls, segURL)
								segNum++
							}
						}
					}
				}

				if len(urls) == 0 {
					for _, baseURL := range rep.BaseURLs {
						if baseURL != "" {
							urls = append(urls, baseURL)
						}
					}
				}

				if len(urls) > 0 {
					break
				}
			}
			if len(urls) > 0 {
				break
			}
		}
		if len(urls) > 0 {
			break
		}
	}

	if len(urls) == 0 {
		return StreamManifest{}, fmt.Errorf("no stream URLs found in DASH manifest")
	}

	return StreamManifest{
		MimeType:       mimeType,
		Codecs:         codecs,
		EncryptionType: "NONE",
		URLs:           urls,
	}, nil
}

func (t *Tidal) metadataToTrack(m TrackMetadata) downloader.Track {
	return downloader.Track{
		ID:          strconv.Itoa(m.ID),
		Title:       m.Title,
		Artist:      m.Artist.Name,
		Album:       m.Album.Title,
		AlbumArtist: m.Artist.Name,
		Quality:     m.AudioQuality,
		Duration:    m.Duration,
		TrackNumber: m.TrackNumber,
		TotalTracks: m.Album.NumberOfTracks,
		DiscNumber:  m.VolumeNumber,
		Copyright:   m.Copyright,
		ISRC:        m.ISRC,
		CoverURL:    CoverURL(m.Album.Cover, 1280),
	}
}

// CoverURL builds a Tidal CDN URL for album cover art.
func CoverURL(coverID string, size int) string {
	if coverID == "" {
		return ""
	}
	path := strings.ReplaceAll(coverID, "-", "/")
	return fmt.Sprintf("https://resources.tidal.com/images/%s/%dx%d.jpg", path, size, size)
}

// ExtractAlbumID parses a Tidal URL to extract the album ID.
func ExtractAlbumID(tidalURL string) string {
	match := albumPattern.FindStringSubmatch(tidalURL)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
