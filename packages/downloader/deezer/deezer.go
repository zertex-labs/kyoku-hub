package deezer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

// Compile-time interface check.
var _ downloader.Provider = (*Deezer)(nil)

const (
	PublicAPIBase = "https://api.deezer.com"
	GWLightBase   = "https://www.deezer.com/ajax/gw-light.php"
	MediaAPIBase  = "https://media.deezer.com/v1/get_url"

	DefaultTimeout = 30 * time.Second

	browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Deezer implements the downloader.Provider interface for Deezer.
type Deezer struct {
	auth       Auth
	config     downloader.Config
	httpClient *http.Client
}

// New creates a new Deezer provider with the given auth and config.
func New(auth Auth, config downloader.Config) *Deezer {
	jar, _ := cookiejar.New(nil)
	d := &Deezer{
		auth:   auth,
		config: config,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Jar:     jar,
		},
	}
	if auth.ARL != "" {
		d.initSessionClient(auth.ARL, auth.SessionID)
	}
	return d
}

// Name returns the service name.
func (d *Deezer) Name() string {
	return "deezer"
}

// IsAuthenticated checks if the provider has valid credentials.
func (d *Deezer) IsAuthenticated() bool {
	return d.auth.ARL != "" && d.auth.APIToken != ""
}

// SetAuth updates the provider's authentication credentials.
func (d *Deezer) SetAuth(auth Auth) {
	d.auth = auth
	if auth.ARL != "" {
		d.initSessionClient(auth.ARL, auth.SessionID)
	}
}

// GetTrack fetches track metadata by ID.
func (d *Deezer) GetTrack(id string) (downloader.Track, error) {
	body, err := d.publicAPICall("/track/" + id)
	if err != nil {
		return downloader.Track{}, fmt.Errorf("failed to get track: %w", err)
	}

	var resp TrackResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return downloader.Track{}, fmt.Errorf("failed to parse track response: %w", err)
	}

	return metadataToTrack(resp), nil
}

// GetAlbum fetches album metadata and tracks by ID.
func (d *Deezer) GetAlbum(id string) (downloader.Album, error) {
	body, err := d.publicAPICall("/album/" + id)
	if err != nil {
		return downloader.Album{}, fmt.Errorf("failed to get album: %w", err)
	}

	var resp AlbumResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return downloader.Album{}, fmt.Errorf("failed to parse album response: %w", err)
	}

	// Fetch full track list with positions
	tracksBody, err := d.publicAPICall("/album/" + id + "/tracks?limit=10000")
	if err != nil {
		return downloader.Album{}, fmt.Errorf("failed to get album tracks: %w", err)
	}

	var trackList TrackList
	if err := json.Unmarshal(tracksBody, &trackList); err != nil {
		return downloader.Album{}, fmt.Errorf("failed to parse album tracks: %w", err)
	}

	albumGenre := ""
	if len(resp.Genres.Data) > 0 {
		albumGenre = resp.Genres.Data[0].Name
	}

	tracks := make([]downloader.Track, len(trackList.Data))
	for i, t := range trackList.Data {
		track := metadataToTrack(t)
		track.Date = resp.ReleaseDate
		track.AlbumArtist = resp.Artist.Name
		track.TotalTracks = resp.NbTracks
		track.Genre = albumGenre
		if track.Album == "" {
			track.Album = resp.Title
		}
		if track.CoverURL == "" {
			track.CoverURL = CoverURL(resp.MD5Image, 1000)
		}
		tracks[i] = track
	}

	return downloader.Album{
		ID:          strconv.Itoa(resp.ID),
		Title:       resp.Title,
		Artist:      resp.Artist.Name,
		Tracks:      tracks,
		Date:        resp.ReleaseDate,
		TotalTracks: resp.NbTracks,
		CoverURL:    CoverURL(resp.MD5Image, 1000),
	}, nil
}

// GetStreamURL returns stream info for a track.
func (d *Deezer) GetStreamURL(trackID string) (downloader.StreamInfo, error) {
	quality := deezerQuality(d.config.Quality)
	info, err := d.getStreamURL(trackID, quality, GWSongData{})
	if err != nil {
		return downloader.StreamInfo{}, err
	}
	return downloader.StreamInfo{
		URL:              info.URL,
		MimeType:         info.MimeType,
		IsEncrypted:      true, // All Deezer streams are Blowfish encrypted
		TrackID:          trackID,
		RequestedQuality: info.RequestedQuality,
		ActualQuality:    info.ActualQuality,
	}, nil
}

// SearchAlbum searches for albums on Deezer.
func (d *Deezer) SearchAlbum(query string) ([]downloader.SearchResult, error) {
	path := "/search/album?q=" + url.QueryEscape(query) + "&limit=10"
	body, err := d.publicAPICall(path)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var resp struct {
		Data []AlbumResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	results := make([]downloader.SearchResult, len(resp.Data))
	for i, album := range resp.Data {
		results[i] = downloader.SearchResult{
			Provider:    "deezer",
			ID:          strconv.Itoa(album.ID),
			Title:       album.Title,
			Artist:      album.Artist.Name,
			CoverURL:    CoverURL(album.MD5Image, 250),
			TrackCount:  album.NbTracks,
			ReleaseDate: album.ReleaseDate,
		}
	}

	return results, nil
}

// GetAlbumByURL extracts the album ID from a Deezer URL.
func (d *Deezer) GetAlbumByURL(deezerURL string) (string, error) {
	id := ExtractAlbumID(deezerURL)
	if id == "" {
		return "", fmt.Errorf("could not extract album ID from URL: %s", deezerURL)
	}
	return id, nil
}

// --- Stream URL Resolution ---

type streamInfo struct {
	URL              string
	MimeType         string
	IsEncrypted      bool
	SongData         GWSongData
	RequestedQuality string
	ActualQuality    string
	Copyright        string
}

func (d *Deezer) getStreamURL(trackID, quality string, songData GWSongData) (streamInfo, error) {
	if songData.SNG_ID == "" {
		var err error
		songData, err = d.getSongData(trackID)
		if err != nil {
			return streamInfo{}, fmt.Errorf("failed to get song data: %w", err)
		}
	}

	actualQuality := quality
	if quality == "FLAC" && songData.FILESIZE_FLAC == "0" {
		actualQuality = "MP3_320"
	}
	if actualQuality == "MP3_320" && songData.FILESIZE_MP3_320 == "0" {
		actualQuality = "MP3_128"
	}

	if actualQuality == "FLAC" && !d.auth.CanStreamLossless {
		if d.auth.CanStreamHQ {
			actualQuality = "MP3_320"
		} else {
			actualQuality = "MP3_128"
		}
	}
	if actualQuality == "MP3_320" && !d.auth.CanStreamHQ {
		actualQuality = "MP3_128"
	}

	mimeType := "audio/mpeg"
	if actualQuality == "FLAC" {
		mimeType = "audio/flac"
	}

	// Try modern media API first
	if songData.TRACK_TOKEN != "" && d.auth.LicenseToken != "" {
		mediaURL, err := d.resolveMediaURL(songData.TRACK_TOKEN, actualQuality)
		if err == nil && mediaURL != "" {
			return streamInfo{
				URL:              mediaURL,
				MimeType:         mimeType,
				IsEncrypted:      true,
				SongData:         songData,
				RequestedQuality: quality,
				ActualQuality:    actualQuality,
				Copyright:        songData.COPYRIGHT,
			}, nil
		}
		// Check for geo-restriction — try FALLBACK
		var geoErr *GeoRestrictedError
		if errors.As(err, &geoErr) && songData.FALLBACK.SNG_ID != "" && songData.FALLBACK.SNG_ID != songData.SNG_ID {
			fallbackData, fbErr := d.getSongData(songData.FALLBACK.SNG_ID)
			if fbErr == nil {
				return d.getStreamURL(songData.FALLBACK.SNG_ID, quality, fallbackData)
			}
		}
	}

	// Fallback: legacy CDN URL
	if songData.MD5_ORIGIN == "" {
		return streamInfo{}, fmt.Errorf("no MD5_ORIGIN available for track %s — cannot build CDN URL", trackID)
	}

	cdnURL, err := BuildLegacyCDNURL(songData.MD5_ORIGIN, songData.SNG_ID, songData.MEDIA_VERSION, actualQuality)
	if err != nil {
		return streamInfo{}, fmt.Errorf("failed to build legacy CDN URL: %w", err)
	}

	return streamInfo{
		URL:              cdnURL,
		MimeType:         mimeType,
		IsEncrypted:      true,
		SongData:         songData,
		RequestedQuality: quality,
		ActualQuality:    actualQuality,
		Copyright:        songData.COPYRIGHT,
	}, nil
}

func (d *Deezer) resolveMediaURL(trackToken, quality string) (string, error) {
	reqBody := MediaRequest{
		LicenseToken: d.auth.LicenseToken,
		Media: []MediaTrack{{
			Type: "FULL",
			Formats: []MediaFormat{{
				Format: quality,
				Cipher: "BF_CBC_STRIPE",
			}},
		}},
		TrackTokens: []string{trackToken},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal media request: %w", err)
	}

	req, err := http.NewRequest("POST", MediaAPIBase, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create media request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("media API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read media response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("media API returned status %d: %s", resp.StatusCode, string(body))
	}

	var mediaResp MediaResponse
	if err := json.Unmarshal(body, &mediaResp); err != nil {
		return "", fmt.Errorf("failed to parse media response: %w", err)
	}

	if len(mediaResp.Data) > 0 {
		if len(mediaResp.Data[0].Errors) > 0 {
			for _, e := range mediaResp.Data[0].Errors {
				if e.Code == 2002 {
					return "", &GeoRestrictedError{Code: e.Code, Message: e.Message}
				}
			}
		}
		if len(mediaResp.Data[0].Media) == 0 {
			return "", fmt.Errorf("media API returned no media entries")
		}
		if len(mediaResp.Data[0].Media[0].Sources) == 0 {
			return "", fmt.Errorf("media API returned no sources for format %s", quality)
		}
	} else {
		return "", fmt.Errorf("media API returned empty data array")
	}

	return mediaResp.Data[0].Media[0].Sources[0].URL, nil
}

// --- Internal Methods ---

func (d *Deezer) initSessionClient(arl, sessionID string) {
	jar, _ := cookiejar.New(nil)

	deezerURL, _ := url.Parse("https://www.deezer.com")
	cookies := []*http.Cookie{
		{Name: "arl", Value: arl, Domain: ".deezer.com", Path: "/"},
	}
	if sessionID != "" {
		cookies = append(cookies, &http.Cookie{Name: "sid", Value: sessionID, Domain: ".deezer.com", Path: "/"})
	}
	jar.SetCookies(deezerURL, cookies)

	d.httpClient = &http.Client{
		Timeout: DefaultTimeout,
		Jar:     jar,
	}
}

func (d *Deezer) publicAPICall(path string) ([]byte, error) {
	return d.publicAPICallInner(path, false)
}

func (d *Deezer) publicAPICallInner(path string, retried bool) ([]byte, error) {
	fullURL := PublicAPIBase + path

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create public API request: %w", err)
	}
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("public API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public API returned status %d: %s", resp.StatusCode, string(body))
	}

	if !retried {
		var apiErr PublicAPIError
		if json.Unmarshal(body, &apiErr) == nil && (apiErr.Error.Code == 4 || apiErr.Error.Code == 700) {
			time.Sleep(5 * time.Second)
			return d.publicAPICallInner(path, true)
		}
	}

	return body, nil
}

func (d *Deezer) gwAPICall(method string, params interface{}) (json.RawMessage, error) {
	return d.gwAPICallInner(method, params, false)
}

func (d *Deezer) gwAPICallInner(method string, params interface{}, retried bool) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("method", method)
	q.Set("api_version", "1.0")
	q.Set("api_token", d.auth.APIToken)
	q.Set("input", "3")

	fullURL := GWLightBase + "?" + q.Encode()

	var bodyReader io.Reader
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		bodyReader = strings.NewReader(string(paramBytes))
	} else {
		bodyReader = strings.NewReader("{}")
	}

	req, err := http.NewRequest("POST", fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GW API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GW API response: %w", err)
	}

	var gwResp GWResponse
	if err := json.Unmarshal(body, &gwResp); err != nil {
		return nil, fmt.Errorf("failed to parse GW API response: %w", err)
	}

	if errMap := gwResp.GWErrorMap(); errMap != nil {
		if !retried && isCSRFTokenError(errMap) {
			if refreshErr := d.refreshCSRFToken(); refreshErr != nil {
				return nil, fmt.Errorf("CSRF token expired and refresh failed: %w", refreshErr)
			}
			return d.gwAPICallInner(method, params, true)
		}
		var errParts []string
		for k, v := range errMap {
			errParts = append(errParts, fmt.Sprintf("%s: %s", k, v))
		}
		return nil, fmt.Errorf("GW API error: %s", strings.Join(errParts, "; "))
	}

	return gwResp.Results, nil
}

func isCSRFTokenError(errMap map[string]string) bool {
	if _, ok := errMap["VALID_TOKEN_REQUIRED"]; ok {
		return true
	}
	for _, v := range errMap {
		lower := strings.ToLower(v)
		if strings.Contains(lower, "invalid csrf token") || strings.Contains(lower, "invalid api token") {
			return true
		}
	}
	return false
}

func (d *Deezer) refreshCSRFToken() error {
	apiURL := "https://www.deezer.com/ajax/gw-light.php?method=deezer.getUserData&input=3&api_version=1.0&api_token="

	req, err := http.NewRequest("POST", apiURL, strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("failed to create getUserData request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call getUserData: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read getUserData response: %w", err)
	}

	var gwResp GWResponse
	if err := json.Unmarshal(body, &gwResp); err != nil {
		return fmt.Errorf("failed to parse getUserData response: %w", err)
	}

	var userData GWUserData
	if err := json.Unmarshal(gwResp.Results, &userData); err != nil {
		return fmt.Errorf("failed to parse user data: %w", err)
	}

	if userData.USER.USER_ID == 0 {
		return fmt.Errorf("ARL expired — cannot refresh CSRF token")
	}

	d.auth.APIToken = userData.CheckForm
	d.auth.LicenseToken = userData.USER.OPTIONS.LicenseToken
	d.auth.CanStreamLossless = userData.USER.OPTIONS.WebLossless
	d.auth.CanStreamHQ = userData.USER.OPTIONS.WebHQ

	return nil
}

func (d *Deezer) getSongData(trackID string) (GWSongData, error) {
	params := map[string]string{"sng_id": trackID}
	results, err := d.gwAPICall("song.getData", params)
	if err != nil {
		return GWSongData{}, fmt.Errorf("failed to get song data: %w", err)
	}

	var songData GWSongData
	if err := json.Unmarshal(results, &songData); err != nil {
		return GWSongData{}, fmt.Errorf("failed to parse song data: %w", err)
	}

	return songData, nil
}

// --- Helper Functions ---

func metadataToTrack(resp TrackResponse) downloader.Track {
	return downloader.Track{
		ID:          strconv.Itoa(resp.ID),
		Title:       resp.Title,
		Artist:      resp.Artist.Name,
		Album:       resp.Album.Title,
		Duration:    resp.Duration,
		TrackNumber: resp.TrackPosition,
		DiscNumber:  resp.DiskNumber,
		ISRC:        resp.ISRC,
		CoverURL:    CoverURL(resp.Album.MD5Image, 1000),
	}
}

// CoverURL builds a Deezer CDN URL for cover art.
func CoverURL(md5 string, size int) string {
	if md5 == "" {
		return ""
	}
	return fmt.Sprintf("https://cdn-images.dzcdn.net/images/cover/%s/%dx%d-000000-80-0-0.jpg", md5, size, size)
}

func deezerQuality(configQuality string) string {
	switch configQuality {
	case "FLAC", "LOSSLESS", "HI_RES_LOSSLESS":
		return "FLAC"
	case "HIGH", "MP3_320":
		return "MP3_320"
	default:
		return "MP3_128"
	}
}

// ExtractAlbumID parses a Deezer URL to extract the album ID.
func ExtractAlbumID(deezerURL string) string {
	match := albumPattern.FindStringSubmatch(deezerURL)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
