package kyoku

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"
)

// Event is one hook invocation.
type Event struct {
	Hook   string
	Plugin string
	App    api.App
	Album  api.Album
	Track  *api.Track
	Files  []api.File

	config   map[string]any
	response api.Response
}

// Config decodes this plugin's configuration into v. Values arrive typed from
// the hub (already merged with the manifest defaults and validated against the
// manifest schema), so a missing or wrong-typed field is a bug in the manifest,
// not something to paper over at runtime.
func (e *Event) Config(v any) error {
	raw, err := json.Marshal(e.config)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("config does not match the manifest: %w", err)
	}
	return nil
}

// Message is the one line the hub prints in the job log for this plugin.
func (e *Event) Message(format string, a ...any) {
	e.response.Message = fmt.Sprintf(format, a...)
}

// Added reports files the plugin created in the album directory, so the hub can
// name them in the log without diffing the directory itself.
func (e *Event) Added(paths ...string) {
	e.response.Files = append(e.response.Files, paths...)
}

// Run executes a binary and returns its combined output. It inherits whatever
// confinement the hub applied to this process: a plugin cannot widen it, so
// everything it starts is confined by the same rules.
func (e *Event) Run(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = e.Album.Dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s: %w", text, err)
		}
		return "", err
	}
	return text, nil
}
