package tidal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	AuthBaseURL     = "https://auth.tidal.com/v1/oauth2/"
	DeviceAuthScope = "r_usr w_usr"
)

type clientCredentials struct {
	ID     string
	Secret string
}

var knownClients = []clientCredentials{
	{ID: "mRAETZku02e8Sl6O", Secret: "xLcbVdJDyj7VZPX8wZtlccwxbWakMhmn9zMDM4YZFPg="},
	{ID: "fX2JxdmntZWK0ixT", Secret: "1Nm5AfDAjxrgJFJbKNWLeAyKGVGmINuXPPLHVXAvxAg="},
	{ID: "4N3n6Q1x95LL5K7p", Secret: "oKOXfJW371cX6xaZ0PyhgGNBdNLlBZd4AKKYougMjik="},
	{ID: "cgiF7TQuB97BUIu3", Secret: "1nqpgx8uvBdZigrx4hUPDV2hOwgYAAAG5DYXOr6uNf8="},
}

// DeviceAuthResponse is returned when requesting a device code.
type DeviceAuthResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

// TokenResponse is returned when polling for or refreshing a token.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		UserID      int    `json:"userId"`
		CountryCode string `json:"countryCode"`
	} `json:"user"`
}

// RequestDeviceCode initiates the device authorization flow.
// Returns the device auth info and the credentials that worked.
func RequestDeviceCode() (*DeviceAuthResponse, error) {
	var lastErr error

	for i := range knownClients {
		creds := &knownClients[i]
		deviceAuth, err := tryRequestDeviceCode(creds)
		if err == nil {
			return deviceAuth, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("all client credentials failed - %w", lastErr)
}

func tryRequestDeviceCode(creds *clientCredentials) (*DeviceAuthResponse, error) {
	data := url.Values{
		"client_id": {creds.ID},
		"scope":     {DeviceAuthScope},
	}

	resp, err := http.PostForm(AuthBaseURL+"device_authorization", data)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device authorization failed (status %d): %s", resp.StatusCode, string(body))
	}

	var deviceAuth DeviceAuthResponse
	if err := json.Unmarshal(body, &deviceAuth); err != nil {
		return nil, fmt.Errorf("failed to parse device auth response: %w", err)
	}

	return &deviceAuth, nil
}

// PollForToken polls the token endpoint until the user authorizes the device.
// Returns the auth data on success.
func PollForToken(deviceCode string) (*Auth, error) {
	interval := 5 * time.Second
	deadline := time.Now().Add(5 * time.Minute) // reasonable timeout

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		for _, creds := range knownClients {
			auth, done, err := checkDeviceToken(deviceCode, &creds)
			if err != nil {
				continue
			}
			if done {
				return auth, nil
			}
			break // authorization_pending, wait and try again
		}
	}

	return nil, fmt.Errorf("device authorization timed out")
}

func checkDeviceToken(deviceCode string, creds *clientCredentials) (*Auth, bool, error) {
	data := url.Values{
		"client_id":     {creds.ID},
		"client_secret": {creds.Secret},
		"device_code":   {deviceCode},
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
		"scope":         {DeviceAuthScope},
	}

	resp, err := http.PostForm(AuthBaseURL+"token", data)
	if err != nil {
		return nil, false, fmt.Errorf("failed to poll token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode == http.StatusBadRequest {
		var tokenErr struct {
			Error     string `json:"error"`
			SubStatus int    `json:"sub_status"`
		}
		if err := json.Unmarshal(body, &tokenErr); err == nil {
			if tokenErr.SubStatus == 1002 || tokenErr.Error == "authorization_pending" {
				return nil, false, nil
			}
			if tokenErr.Error == "expired_token" {
				return nil, false, fmt.Errorf("device code expired")
			}
		}
		return nil, false, fmt.Errorf("token request failed: %s", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("token request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, false, fmt.Errorf("failed to parse token response: %w", err)
	}

	auth := &Auth{
		Token:        tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
		UserID:       fmt.Sprintf("%d", tokenResp.User.UserID),
		CountryCode:  tokenResp.User.CountryCode,
	}

	return auth, true, nil
}

// RefreshAccessToken uses the refresh token to obtain a new access token.
func RefreshAccessToken(auth *Auth) error {
	if auth.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	var lastErr error
	for _, creds := range knownClients {
		err := tryRefreshToken(auth, &creds)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("token refresh failed with all credentials: %w", lastErr)
}

func tryRefreshToken(auth *Auth, creds *clientCredentials) error {
	data := url.Values{
		"client_id":     {creds.ID},
		"client_secret": {creds.Secret},
		"refresh_token": {auth.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm(AuthBaseURL+"token", data)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse refresh response: %w", err)
	}

	auth.Token = tokenResp.AccessToken
	auth.ExpiresAt = time.Now().Unix() + int64(tokenResp.ExpiresIn)
	if tokenResp.RefreshToken != "" {
		auth.RefreshToken = tokenResp.RefreshToken
	}

	return nil
}
