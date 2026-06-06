package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const agentFile = "agent.json"

// Identity holds the agent's keypair and hub-assigned ID.
type Identity struct {
	AgentID    string `json:"agent_id"`
	PrivateKey string `json:"private_key"` // base64-encoded ed25519 private key
	PublicKey  string `json:"public_key"`  // base64-encoded ed25519 public key
}

// LoadOrGenerate loads an existing identity from dataDir/agent.json,
// or generates a new keypair if the file doesn't exist.
// Returns the identity and whether it's newly generated (needs registration).
func LoadOrGenerate(dataDir string) (*Identity, bool, error) {
	path := filepath.Join(dataDir, agentFile)

	data, err := os.ReadFile(path)
	if err == nil {
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, false, fmt.Errorf("parse %s: %w", path, err)
		}
		if id.AgentID != "" && id.PrivateKey != "" && id.PublicKey != "" {
			return &id, false, nil
		}
		// File exists but incomplete — regenerate
	}

	// Generate new keypair
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, false, fmt.Errorf("generate keypair: %w", err)
	}

	id := &Identity{
		PrivateKey: base64.StdEncoding.EncodeToString(priv),
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
	}

	return id, true, nil
}

// Save persists the identity to dataDir/agent.json.
func Save(dataDir string, id *Identity) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}

	path := filepath.Join(dataDir, agentFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// PrivateKeyBytes decodes the base64 private key.
func (id *Identity) PrivateKeyBytes() (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	return ed25519.PrivateKey(b), nil
}
