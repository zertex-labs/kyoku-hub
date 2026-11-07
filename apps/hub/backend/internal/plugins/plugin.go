// Package plugins loads user-installed post-download plugins from a directory and
// runs them as separate processes.
//
// They are processes, not Go plugins (build-mode=plugin): a .so is locked to the
// exact toolchain and dependency graph of the binary that loads it, so every
// kyoku rebuild would break every installed plugin, and a panic inside one would
// take the hub down with it. A process has no ABI, can be written in anything,
// and can be debugged by running it by hand.
//
// A plugin is a directory containing plugin.yaml and an executable:
//
//	plugins/replaygain/
//	  plugin.yaml
//	  replaygain-rs
//
// The host writes a JSON payload to the plugin's stdin, the plugin edits the
// files it was given, and the exit code decides the outcome:
//
//	0  ok
//	3  skipped (not applicable to this file/album)
//	*  failure, with stderr shown in the job log
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Manifest is plugin.yaml.
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Command is argv. A relative path is resolved against the plugin directory.
	Command []string `yaml:"command"`
	// Hooks the plugin subscribes to. A plugin is never run for a hook it did
	// not ask for.
	Hooks []string `yaml:"hooks"`
	// Timeout is a Go duration ("90s"); defaults to 60s.
	Timeout string `yaml:"timeout"`
	// Required makes a failure block the download instead of warning.
	Required bool `yaml:"required"`
	// Extensions limits the plugin to some containers, e.g. [".flac"].
	Extensions []string `yaml:"extensions"`

	dir string
}

// Plugin is a loaded manifest with a resolved working directory.
type Plugin struct {
	Manifest
	timeout time.Duration
}

// Response is the optional JSON a plugin may print on stdout.
type Response struct {
	Message string   `json:"message"`
	Files   []string `json:"files"`
}

// Registry is the set of installed plugins.
type Registry struct {
	plugins []Plugin
	log     *zap.Logger
}

// Load scans a directory of plugin directories. A missing directory is not an
// error: most installs have no plugins at all.
func Load(root string) ([]Plugin, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Plugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
		if err != nil {
			continue // not a plugin directory
		}
		var m Manifest
		if err := yaml.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("%s/plugin.yaml: %w", e.Name(), err)
		}
		m.dir = dir
		if m.Name == "" {
			m.Name = e.Name()
		}
		if len(m.Command) == 0 {
			m.Command = []string{"./" + e.Name()}
		}
		if !filepath.IsAbs(m.Command[0]) && !strings.Contains(m.Command[0], "/") {
			m.Command[0] = "./" + m.Command[0]
		}
		p := Plugin{Manifest: m}
		p.timeout = 60 * time.Second
		if m.Timeout != "" {
			d, err := time.ParseDuration(m.Timeout)
			if err != nil {
				return nil, fmt.Errorf("%s: timeout: %w", m.Name, err)
			}
			p.timeout = d
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func NewRegistry(root string, log *zap.Logger) (*Registry, error) {
	if log == nil {
		log = zap.NewNop()
	}
	ps, err := Load(root)
	if err != nil {
		return nil, err
	}
	return &Registry{plugins: ps, log: log}, nil
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p.Name)
	}
	return out
}

// Required reports whether this plugin declared a failure as blocking. Part of
// download.Dispatcher: the pipeline decides what a required failure means.
func (r *Registry) Required(name string) bool {
	if r == nil {
		return false
	}
	for _, p := range r.plugins {
		if p.Name == name {
			return p.Required
		}
	}
	return false
}

// Plugins exposes the loaded manifests (settings, diagnostics).
func (r *Registry) Plugins() []Plugin {
	if r == nil {
		return nil
	}
	return r.plugins
}

func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.plugins)
}

// Dispatch runs every plugin subscribed to hook. It never stops early: one
// plugin's failure must not cost another the chance to run.
func (r *Registry) Dispatch(ctx context.Context, hook Hook, p Payload) []Outcome {
	if r == nil {
		return nil
	}
	var out []Outcome
	for _, pl := range r.plugins {
		if !pl.wants(hook) || !pl.handlesExt(p) {
			continue
		}
		payload := p
		payload.Hook = string(hook)
		payload.Plugin = pl.Name

		res := r.run(ctx, pl, payload)
		res.Plugin = pl.Name
		res.Hook = string(hook)
		out = append(out, res)
	}
	return out
}

func (pl Plugin) wants(h Hook) bool {
	for _, v := range pl.Hooks {
		if strings.EqualFold(strings.TrimSpace(v), string(h)) {
			return true
		}
	}
	return false
}

func (pl Plugin) handlesExt(p Payload) bool {
	if len(pl.Extensions) == 0 {
		return true
	}
	ext := ""
	if p.Track != nil {
		ext = p.Track.Ext
	} else if len(p.Files) > 0 {
		ext = p.Files[0].Ext
	}
	for _, v := range pl.Extensions {
		if strings.EqualFold(v, ext) {
			return true
		}
	}
	return false
}

func (r *Registry) run(ctx context.Context, pl Plugin, p Payload) Outcome {
	arg0 := pl.Command[0]
	if !filepath.IsAbs(arg0) {
		arg0 = filepath.Join(pl.dir, arg0)
	}
	if _, err := os.Stat(arg0); err != nil {
		return Outcome{Err: fmt.Errorf("executable not found: %s", arg0)}
	}

	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Outcome{Err: fmt.Errorf("payload: %w", err)}
	}

	cctx, cancel := context.WithTimeout(ctx, pl.timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, arg0, pl.Command[1:]...)
	cmd.Dir = pl.dir
	// A shell script spawns children; killing only the shell leaves them holding
	// stdout and Wait blocks until they exit. Kill the group and bound the wait.
	isolateProcess(cmd)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = pluginEnv(p)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	out := Outcome{Stderr: strings.TrimSpace(stderr.String())}

	var exitErr *exec.ExitError
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		out.Err = fmt.Errorf("timed out after %s", pl.timeout)
	case err == nil:
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 3:
		out.Skipped = true
	case exitErr != nil:
		out.Err = fmt.Errorf("exit %d", exitErr.ExitCode())
	default:
		out.Err = err
	}

	if trimmed := bytes.TrimSpace(stdout.Bytes()); len(trimmed) > 0 {
		var resp Response
		if json.Unmarshal(trimmed, &resp) == nil {
			out.Message = resp.Message
		} else {
			out.Message = string(trimmed)
		}
	}

	if out.Message == "" && out.Skipped {
		out.Message = "skipped"
	}
	return out
}

// pluginEnv is deliberately small: PATH so the plugin can find its own tools,
// HOME and TMPDIR so it can cache, and the hook identity.
//
// The few payload fields a shell script needs are mirrored as variables: parsing
// JSON in sh is the first thing that makes a plugin feel unpleasant to write.
func pluginEnv(p Payload) []string {
	env := []string{
		"KYOKU_HOOK=" + p.Hook,
		"KYOKU_PLUGIN=" + p.Plugin,
		"KYOKU_APP_VERSION=" + p.App.Version,
		"KYOKU_ALBUM_DIR=" + p.Album.Dir,
		"KYOKU_ALBUM_TITLE=" + p.Album.Title,
		"KYOKU_ALBUM_ARTIST=" + p.Album.Artist,
		"KYOKU_RELEASE_GROUP_ID=" + p.Album.ReleaseGroupID,
	}
	if p.Track != nil {
		env = append(env,
			"KYOKU_TRACK_PATH="+p.Track.Path,
			"KYOKU_TRACK_TITLE="+p.Track.Title,
			"KYOKU_TRACK_ISRC="+p.Track.ISRC,
		)
	}
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}
