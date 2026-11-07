package plugins

import (
	"strings"
	"testing"
)

func TestSecretBoxRoundTrip(t *testing.T) {
	box, err := NewSecretBox("session-secret")
	if err != nil {
		t.Fatalf("box: %v", err)
	}

	values := map[string]any{"url": "https://navidrome.lan", "pass": "hunter2", "skip": true}
	blob, err := box.Seal(values)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(string(blob), "hunter2") {
		t.Fatal("password is readable in the sealed blob")
	}

	got, err := box.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got["pass"] != "hunter2" || got["url"] != "https://navidrome.lan" || got["skip"] != true {
		t.Fatalf("round trip lost data: %v", got)
	}
}

func TestSecretBoxRejectsAnotherSecret(t *testing.T) {
	box, _ := NewSecretBox("one")
	blob, _ := box.Seal(map[string]any{"pass": "hunter2"})

	other, _ := NewSecretBox("two")
	if _, err := other.Open(blob); err == nil {
		t.Fatal("opened with the wrong key")
	}
}

func TestSecretBoxRejectsTampering(t *testing.T) {
	box, _ := NewSecretBox("session-secret")
	blob, _ := box.Seal(map[string]any{"pass": "hunter2"})

	blob[len(blob)-1] ^= 0xFF
	if _, err := box.Open(blob); err == nil {
		t.Fatal("opened a tampered blob")
	}
}

func TestSecretBoxNeedsASecret(t *testing.T) {
	if _, err := NewSecretBox(""); err == nil {
		t.Fatal("built a box with no key material")
	}
}

func TestSecretBoxEmptyConfig(t *testing.T) {
	box, _ := NewSecretBox("session-secret")
	blob, err := box.Seal(nil)
	if err != nil || blob != nil {
		t.Fatalf("seal(nil) = %v, %v", blob, err)
	}
	got, err := box.Open(nil)
	if err != nil || got != nil {
		t.Fatalf("open(nil) = %v, %v", got, err)
	}
}
