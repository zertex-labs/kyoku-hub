package handler

import (
	"testing"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/plugins"
)

func intPtr(v int) *int { return &v }

func testFields() []plugins.Field {
	return []plugins.Field{
		{Key: "url", Type: plugins.FieldString, Required: true},
		{Key: "token", Type: plugins.FieldSecret},
		{Key: "skip", Type: plugins.FieldBool, Default: true},
		{Key: "mode", Type: plugins.FieldSelect, Options: []string{"fast", "thorough"}, Default: "fast"},
		{Key: "threads", Type: plugins.FieldInt, Min: intPtr(1), Max: intPtr(16), Default: 4},
	}
}

func TestValidateConfigAcceptsAndCoerces(t *testing.T) {
	got, err := validateConfig(testFields(), map[string]any{
		"url":     "https://navidrome.lan",
		"token":   "hunter2",
		"skip":    false,
		"mode":    "thorough",
		"threads": float64(8), // JSON numbers arrive as float64
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got["threads"] != 8 {
		t.Errorf("threads = %v", got["threads"])
	}
	if got["skip"] != false {
		t.Errorf("skip = %v", got["skip"])
	}
}

func TestValidateConfigFillsDefaults(t *testing.T) {
	got, err := validateConfig(testFields(), map[string]any{"url": "x"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got["skip"] != true || got["mode"] != "fast" || got["threads"] != 4 {
		t.Fatalf("defaults missing: %v", got)
	}
}

func TestValidateConfigRejects(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"missing required", map[string]any{}, "url is required"},
		{"unknown field", map[string]any{"url": "x", "nope": 1}, `unknown config field "nope"`},
		{"wrong type", map[string]any{"url": 5}, "url must be a string"},
		{"not an option", map[string]any{"url": "x", "mode": "warp"}, "mode must be one of"},
		{"below min", map[string]any{"url": "x", "threads": 0}, "at least 1"},
		{"above max", map[string]any{"url": "x", "threads": 99}, "at most 16"},
		{"bool as string", map[string]any{"url": "x", "skip": "yes"}, "skip must be true or false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateConfig(testFields(), c.body)
			if err == nil {
				t.Fatalf("accepted %v", c.body)
			}
			if !contains(err.Error(), c.want) {
				t.Fatalf("error = %q, want %q", err, c.want)
			}
		})
	}
}

// The API describes that a secret exists; it never returns its value.
func TestMaskSecretsHidesValues(t *testing.T) {
	fields := []plugins.Field{{Key: "token", Type: plugins.FieldSecret}, {Key: "url", Type: plugins.FieldString}}
	out := maskSecrets(fields, map[string]any{"token": "hunter2", "url": "https://x"})

	if out["token"] != "" {
		t.Errorf("secret leaked: %v", out["token"])
	}
	if out["token_set"] != true {
		t.Errorf("secret presence not reported: %v", out["token_set"])
	}
	if out["url"] != "https://x" {
		t.Errorf("non-secret field was masked: %v", out["url"])
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
