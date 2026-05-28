package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/config"
	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/database/queries"
	"github.com/labstack/echo/v4"
)

const (
	sessionCookieName = "kyoku_session"
	sessionDuration   = 30 * 24 * time.Hour

	mbAuthorizeURL = "https://musicbrainz.org/oauth2/authorize"
	mbTokenURL     = "https://musicbrainz.org/oauth2/token"
	mbUserInfoURL  = "https://musicbrainz.org/oauth2/userinfo"
)

type AuthHandler struct {
	cfg     *config.Config
	queries *queries.Queries
}

func NewAuthHandler(cfg *config.Config, queries *queries.Queries) *AuthHandler {
	return &AuthHandler{cfg: cfg, queries: queries}
}

func (h *AuthHandler) Register(g *echo.Group) {
	g.GET("/authorize", h.Authorize)
	g.GET("/callback", h.Callback)
	g.GET("/session", h.Session)
	g.POST("/logout", h.Logout)
}

// Authorize initiates the MusicBrainz OAuth2 flow with PKCE
func (h *AuthHandler) Authorize(c echo.Context) error {
	// Generate PKCE code verifier (43-128 chars, base64url)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate verifier"})
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Generate state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate state"})
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Code challenge = base64url(sha256(code_verifier))
	h256 := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h256[:])

	// Store state + verifier in cookies (short-lived)
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "oauth_verifier",
		Value:    codeVerifier,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// Build authorize URL
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.cfg.MusicBrainzClientID},
		"redirect_uri":          {h.cfg.BaseURL + "/api/auth/callback"},
		"scope":                 {"profile"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}

	return c.Redirect(http.StatusFound, mbAuthorizeURL+"?"+params.Encode())
}

// Callback handles the OAuth2 callback from MusicBrainz
func (h *AuthHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" || state == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing code or state"})
	}

	// Validate state
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid state"})
	}

	// Get code verifier
	verifierCookie, err := c.Cookie("oauth_verifier")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing verifier"})
	}

	// Exchange code for token
	tokenResp, err := h.exchangeCode(code, verifierCookie.Value)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "token exchange failed"})
	}

	// Fetch user info
	userInfo, err := h.fetchUserInfo(tokenResp.AccessToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch user info"})
	}

	// Find or create user
	user, err := h.queries.FindOrCreateUser(c.Request().Context(), userInfo.MBID, userInfo.Sub)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
	}

	// Create session
	session, err := h.queries.CreateSession(c.Request().Context(), user.ID, sessionDuration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
	}

	// Set session cookie
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// Clear OAuth cookies
	c.SetCookie(&http.Cookie{Name: "oauth_state", Path: "/", MaxAge: -1})
	c.SetCookie(&http.Cookie{Name: "oauth_verifier", Path: "/", MaxAge: -1})

	// Redirect to frontend
	return c.Redirect(http.StatusFound, h.cfg.FrontendURL)
}

// Session returns the current auth status
func (h *AuthHandler) Session(c echo.Context) error {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"authenticated": false,
			"user":          nil,
		})
	}

	user, err := h.queries.GetSessionWithUser(c.Request().Context(), cookie.Value)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"authenticated": false,
			"user":          nil,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          map[string]string{"username": user.Username},
	})
}

// Logout destroys the session
func (h *AuthHandler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(sessionCookieName)
	if err == nil {
		h.queries.DeleteSession(c.Request().Context(), cookie.Value)
	}

	c.SetCookie(&http.Cookie{
		Name:   sessionCookieName,
		Path:   "/",
		MaxAge: -1,
	})

	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

// --- helpers ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type userInfoResponse struct {
	Sub     string // username (e.g. "xrexy")
	MBID    string // numeric metabrainz_user_id as string
	Profile string
}

func (u *userInfoResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := raw["sub"]; ok {
		json.Unmarshal(v, &u.Sub)
	}
	if v, ok := raw["profile"]; ok {
		json.Unmarshal(v, &u.Profile)
	}

	// metabrainz_user_id is a number
	if v, ok := raw["metabrainz_user_id"]; ok {
		var num json.Number
		if err := json.Unmarshal(v, &num); err == nil {
			u.MBID = num.String()
		}
	}

	return nil
}

func (h *AuthHandler) exchangeCode(code, codeVerifier string) (*tokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {h.cfg.BaseURL + "/api/auth/callback"},
		"client_id":     {h.cfg.MusicBrainzClientID},
		"client_secret": {h.cfg.MusicBrainzClientSecret},
		"code_verifier": {codeVerifier},
	}

	resp, err := http.Post(mbTokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	return &tokenResp, nil
}

func (h *AuthHandler) fetchUserInfo(accessToken string) (*userInfoResponse, error) {
	req, _ := http.NewRequest("GET", mbUserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var info userInfoResponse
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}
