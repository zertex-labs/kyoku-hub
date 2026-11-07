package plugins

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
)

// SecretBox encrypts plugin configuration at rest.
//
// The whole value map is sealed rather than the fields marked secret: the store
// then never has to know which key is sensitive, and the shape of a plugin's
// config does not leak through what is readable in the database.
type SecretBox struct {
	aead cipher.AEAD
}

// NewSecretBox derives a key from the hub's session secret. Rotating that secret
// invalidates stored plugin config, which is the correct trade: the alternative
// is a second secret to lose.
func NewSecretBox(secret string) (*SecretBox, error) {
	if secret == "" {
		return nil, fmt.Errorf("session secret is empty, refusing to store plugin config")
	}
	sum := sha256.Sum256([]byte("kyoku/plugin-config\x00" + secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (s *SecretBox) Seal(values map[string]any) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, plain, nil), nil
}

func (s *SecretBox) Open(blob []byte) (map[string]any, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	ns := s.aead.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("config blob is truncated")
	}
	plain, err := s.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("config blob does not decrypt (session secret changed?): %w", err)
	}
	var values map[string]any
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, err
	}
	return values, nil
}
