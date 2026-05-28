package brainzclient

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// API is an HTTP client for the brainz REST API.
type API struct {
	baseURL string
	http    *http.Client
}

// NewAPI creates a new brainz REST API client.
func NewAPI(baseURL string) *API {
	return &API{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ProxyGet fetches a path from the brainz API and returns the status code and body.
// The caller is responsible for the path being safe (MBID-validated).
func (a *API) ProxyGet(path string) (int, []byte, error) {
	url := fmt.Sprintf("%s%s", a.baseURL, path)
	resp, err := a.http.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, body, nil
}
