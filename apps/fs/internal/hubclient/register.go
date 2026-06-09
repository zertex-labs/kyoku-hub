package hubclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type registerRequest struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Label     string `json:"label"`
}

type registerResponse struct {
	AgentID string `json:"agent_id"`
	Label   string `json:"label"`
}

// Register sends the agent's public key and one-time token to hub,
// returning the assigned agent ID.
func Register(hubAPIURL, token, publicKey, label string) (string, error) {
	body, err := json.Marshal(registerRequest{
		Token:     token,
		PublicKey: publicKey,
		Label:     label,
	})
	if err != nil {
		return "", fmt.Errorf("marshal register request: %w", err)
	}

	resp, err := http.Post(hubAPIURL+"/agent/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registration failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result registerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse register response: %w", err)
	}

	if result.AgentID == "" {
		return "", fmt.Errorf("server returned empty agent_id")
	}

	return result.AgentID, nil
}
