package deezer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/packages/downloader"
)

var _ downloader.Provider = (*Deezer)(nil)

const (
	PublicAPIBase = "https://api.deezer.com"
	GWLightBase   = "https://www.deezer.com/ajax/gw-light.php"
	MediaAPIBase  = "https://media.deezer.com/v1/get_url"

	DefaultTimeout = 30 * time.Second

	deezerCallsPerSecond = 8.0

	browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

type internalTrack struct {
	ID          string
	Title       string
	TrackNumber int
	DiscNumber  int
}

type Deezer struct {
	auth       Auth
	config     downloader.Config
	httpClient *http.Client
	// throttle is one budget for this account, shared by every caller in the
	// process: search, probe and download all pull from the same quota.
	throttle *downloader.Throttle
}

func New(auth Auth, config downloader.Config) *Deezer {
	jar, _ := cookiejar.New(nil)
	d := &Deezer{
		auth:     auth,
		config:   config,
		throttle: downloader.NewThrottle(deezerCallsPerSecond),
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

	quality := "MP3_128"
	if d.auth.CanStreamLossless {
		quality = "LOSSLESS"
	} else if d.auth.CanStreamHQ {
		quality = "MP3_320"
	}

	results := make([]downloader.SearchResult, len(resp.Data))
	for i, album := range resp.Data {
		year := album.ReleaseDate
		if len(year) > 4 {
			year = year[:4]
		}
		results[i] = downloader.SearchResult{
			Provider:   "deezer",
			ID:         strconv.Itoa(album.ID),
			Title:      album.Title,
			Artist:     album.Artist.Name,
			CoverURL:   CoverURL(album.MD5Image, 250),
			TrackCount: album.NbTracks,
			Year:       year,
			Quality:    quality,
		}
	}

	return results, nil
}

func (d *Deezer) DownloadAlbum(ctx context.Context, albumID, destDir string, onProgress func(phase string, n, total int)) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	tracks, err := d.getTracksForAlbum(albumID)
	if err != nil {
		return fmt.Errorf("failed to get album tracks: %w", err)
	}

	total := len(tracks)
	quality := deezerQuality(d.config.Quality)
	meta := d.getAlbumMeta(albumID, tracks)

	for i, track := range tracks {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		onProgress("downloading", i+1, total)

		songData, err := d.getSongData(track.ID)
		if err != nil {
			return fmt.Errorf("track %s: failed to get song data: %w", track.ID, err)
		}

		info, err := d.getStreamURL(track.ID, quality, songData)
		if err != nil {
			return fmt.Errorf("track %s: failed to get stream URL: %w", track.ID, err)
		}

		ext := ".mp3"
		if info.MimeType == "audio/flac" {
			ext = ".flac"
		}

		fname := fmt.Sprintf("%02d - %s%s", track.TrackNumber, sanitizeFilename(track.Title), ext)
		fpath := filepath.Join(destDir, fname)

		if err := d.downloadTrack(ctx, info.URL, fpath, track.ID); err != nil {
			return fmt.Errorf("track %s: download failed: %w", track.ID, err)
		}

		if ext == ".flac" {
			_ = tagFlacTrack(fpath, track, songData, meta)
		}
	}

	onProgress("complete", total, total)
	return nil
}

func (d *Deezer) getTracksForAlbum(albumID string) ([]internalTrack, error) {
	body, err := d.publicAPICall("/album/" + albumID + "/tracks?limit=10000")
	if err != nil {
		return nil, err
	}

	var trackList TrackList
	if err := json.Unmarshal(body, &trackList); err != nil {
		return nil, fmt.Errorf("failed to parse track list: %w", err)
	}

	tracks := make([]internalTrack, len(trackList.Data))
	for i, t := range trackList.Data {
		tracks[i] = internalTrack{
			ID:          strconv.Itoa(t.ID),
			Title:       t.Title,
			TrackNumber: t.TrackPosition,
			DiscNumber:  t.DiskNumber,
		}
	}
	return tracks, nil
}

func (d *Deezer) downloadTrack(ctx context.Context, streamURL, destPath, trackID string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream request returned %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return DecryptStream(resp.Body, f, trackID)
}

// --- Stream URL Resolution ---

type streamInfo struct {
	URL              string
	MimeType         string
	SongData         GWSongData
	RequestedQuality string
	ActualQuality    string
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

	if songData.TRACK_TOKEN != "" && d.auth.LicenseToken != "" {
		mediaURL, err := d.resolveMediaURL(songData.TRACK_TOKEN, actualQuality)
		if err == nil && mediaURL != "" {
			return streamInfo{
				URL:              mediaURL,
				MimeType:         mimeType,
				SongData:         songData,
				RequestedQuality: quality,
				ActualQuality:    actualQuality,
			}, nil
		}
		var geoErr *GeoRestrictedError
		if errors.As(err, &geoErr) && songData.FALLBACK.SNG_ID != "" && songData.FALLBACK.SNG_ID != songData.SNG_ID {
			fallbackData, fbErr := d.getSongData(songData.FALLBACK.SNG_ID)
			if fbErr == nil {
				return d.getStreamURL(songData.FALLBACK.SNG_ID, quality, fallbackData)
			}
		}
	}

	if songData.MD5_ORIGIN == "" {
		return streamInfo{}, fmt.Errorf("no MD5_ORIGIN available for track %s", trackID)
	}

	cdnURL, err := BuildLegacyCDNURL(songData.MD5_ORIGIN, songData.SNG_ID, songData.MEDIA_VERSION, actualQuality)
	if err != nil {
		return streamInfo{}, fmt.Errorf("failed to build CDN URL: %w", err)
	}

	return streamInfo{
		URL:              cdnURL,
		MimeType:         mimeType,
		SongData:         songData,
		RequestedQuality: quality,
		ActualQuality:    actualQuality,
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
		return "", err
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
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("media API returned status %d", resp.StatusCode)
	}

	var mediaResp MediaResponse
	if err := json.Unmarshal(body, &mediaResp); err != nil {
		return "", fmt.Errorf("failed to parse media response: %w", err)
	}

	if len(mediaResp.Data) == 0 {
		return "", fmt.Errorf("media API returned empty data")
	}
	if len(mediaResp.Data[0].Errors) > 0 {
		for _, e := range mediaResp.Data[0].Errors {
			if e.Code == 2002 {
				return "", &GeoRestrictedError{Code: e.Code, Message: e.Message}
			}
		}
	}
	if len(mediaResp.Data[0].Media) == 0 || len(mediaResp.Data[0].Media[0].Sources) == 0 {
		return "", fmt.Errorf("media API returned no sources for %s", quality)
	}

	return mediaResp.Data[0].Media[0].Sources[0].URL, nil
}

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
	if err := d.throttle.Wait(context.Background()); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", PublicAPIBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	// Deezer localises genre names to the caller's language ("Азиатска музика"
	// for a bg locale). Tags are portable, so ask for the English taxonomy.
	req.Header.Set("Accept-Language", "en")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("public API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
	if err := d.throttle.Wait(context.Background()); err != nil {
		return nil, err
	}

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
		return nil, err
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
		return nil, err
	}

	var gwResp GWResponse
	if err := json.Unmarshal(body, &gwResp); err != nil {
		return nil, fmt.Errorf("failed to parse GW API response: %w", err)
	}

	if errMap := gwResp.GWErrorMap(); errMap != nil {
		if !retried && isCSRFTokenError(errMap) {
			if refreshErr := d.refreshCSRFToken(); refreshErr != nil {
				return nil, fmt.Errorf("CSRF token refresh failed: %w", refreshErr)
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
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var gwResp GWResponse
	if err := json.Unmarshal(body, &gwResp); err != nil {
		return err
	}

	var userData GWUserData
	if err := json.Unmarshal(gwResp.Results, &userData); err != nil {
		return err
	}

	if userData.USER.USER_ID == 0 {
		return fmt.Errorf("ARL expired")
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
		return GWSongData{}, err
	}

	var songData GWSongData
	if err := json.Unmarshal(results, &songData); err != nil {
		return GWSongData{}, err
	}

	return songData, nil
}

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

func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	)
	return strings.TrimSpace(r.Replace(s))
}
