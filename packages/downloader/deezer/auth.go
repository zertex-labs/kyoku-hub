package deezer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// InitSession initializes a Deezer session using an ARL cookie.
func InitSession(arl string) (*Auth, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	deezerURL, _ := url.Parse("https://www.deezer.com")
	jar.SetCookies(deezerURL, []*http.Cookie{
		{Name: "arl", Value: arl, Domain: ".deezer.com", Path: "/"},
	})

	client := &http.Client{Jar: jar}

	apiURL := "https://www.deezer.com/ajax/gw-light.php?method=deezer.getUserData&input=3&api_version=1.0&api_token="

	req, err := http.NewRequest("POST", apiURL, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create getUserData request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call getUserData: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read getUserData response: %w", err)
	}

	var gwResp GWResponse
	if err := json.Unmarshal(body, &gwResp); err != nil {
		return nil, fmt.Errorf("failed to parse getUserData response: %w", err)
	}

	var userData GWUserData
	if err := json.Unmarshal(gwResp.Results, &userData); err != nil {
		return nil, fmt.Errorf("failed to parse user data: %w", err)
	}

	if userData.USER.USER_ID == 0 {
		return nil, fmt.Errorf("invalid ARL — not logged in")
	}

	sessionID := userData.SessionID
	for _, cookie := range jar.Cookies(deezerURL) {
		if cookie.Name == "sid" {
			sessionID = cookie.Value
			break
		}
	}

	auth := &Auth{
		ARL:               arl,
		SessionID:         sessionID,
		LicenseToken:      userData.USER.OPTIONS.LicenseToken,
		APIToken:          userData.CheckForm,
		UserID:            fmt.Sprintf("%d", userData.USER.USER_ID),
		CanStreamLossless: userData.USER.OPTIONS.WebLossless,
		CanStreamHQ:       userData.USER.OPTIONS.WebHQ,
	}

	return auth, nil
}
