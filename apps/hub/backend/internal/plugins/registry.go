// Package plugins loads user-installed plugins from a directory and runs them.
//
// A plugin is a directory containing plugin.yaml and a compiled Go binary. The
// binary is a separate process, not a Go plugin (.so): a shared object is locked
// to the exact toolchain and dependency graph of the binary that loads it, so
// every rebuild would break every installed plugin, and a panic inside one would
// take the hub down. A process has no ABI and can only be killed.
//
// Plugins are installed by hand — copy a directory into $PLUGINS_DIR — because
// installing one is granting code access to the machine, and that should require
// access to the machine. Nothing here accepts an upload.
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

	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
)

// FieldType is the set of config inputs the settings UI can render.
type FieldType string

const (
	FieldString FieldType = "string"
	FieldSecret FieldType = "secret"
	FieldBool   FieldType = "bool"
	FieldInt    FieldType = "int"
	FieldSelect FieldType = "select"
)

// Field is one configuration input.
type Field struct {
	Key     string    `yaml:"key"`
	Type    FieldType `yaml:"type"`
	Label   string    `yaml:"label"`
	Help    string    `yaml:"help"`
	Default any       `yaml:"default"`
	Options []string  `yaml:"options"`
	Min     *int      `yaml:"min"`
	Max     *int      `yaml:"max"`
	// Required makes an unset value a startup error rather than an empty string.
	Required bool `yaml:"required"`
}

// Permissions is what a plugin asks for. Requests are shown to the admin; the
// floor (album dir read/write, no network) is not negotiable and is not
// expressed here.
type Permissions struct {
	// Exec is the list of binaries this plugin may run, by absolute path.
	Exec []string `yaml:"exec"`
	// Net is the list of hosts this plugin may reach. Empty means no network.
	Net []string `yaml:"net"`
}

// Manifest is plugin.yaml.
type Manifest struct {
	APIVersion  int         `yaml:"apiVersion"`
	Name        string      `yaml:"name"`
	Version     string      `yaml:"version"`
	Author      string      `yaml:"author"`
	Description string      `yaml:"description"`
	Executable  string      `yaml:"executable"`
	Hooks       []string    `yaml:"hooks"`
	Timeout     string      `yaml:"timeout"`
	Required    bool        `yaml:"required"`
	Extensions  []string    `yaml:"extensions"`
	Permissions Permissions `yaml:"permissions"`
	Config      []Field     `yaml:"config"`

	dir     string
	path    string
	timeout time.Duration
}

// Plugin is a validated manifest with a resolved executable.
type Plugin struct {
	Manifest
}

// ConfigStore supplies persisted configuration. Absent means defaults only.
type ConfigStore interface {
	Values(ctx context.Context, pluginID string) (map[string]any, error)
}

// Registry is the installed plugin set.
type Registry struct {
	plugins []Plugin
	log     *zap.Logger
	store   ConfigStore
	app     api.App
}

// NewRegistry scans the plugins directory. A missing directory is not an error:
// most installs have none.
func NewRegistry(root string, store ConfigStore, app api.App, log *zap.Logger) (*Registry, error) {
	if log == nil {
		log = zap.NewNop()
	}
	ps, err := Load(root)
	if err != nil && len(ps) == 0 {
		return nil, err
	}
	if err != nil {
		log.Warn("some plugins were not loaded", zap.Error(err))
	}

	// Verify every binary against its manifest before it is dispatched anything.
	// A plugin that cannot describe itself is not enabled, and the reason is in
	// the log rather than in a failed download.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ok := make([]Plugin, 0, len(ps))
	for _, pl := range ps {
		if _, err := pl.Describe(ctx); err != nil {
			log.Warn("plugin disabled", zap.String("plugin", pl.Name), zap.Error(err))
			continue
		}
		ok = append(ok, pl)
	}
	return &Registry{plugins: ok, log: log, store: store, app: app}, nil
}

// Load reads and validates every plugin directory under root.
func Load(root string) ([]Plugin, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Plugin
	var problems []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
		if err != nil {
			continue // not a plugin directory
		}

		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true) // a typo is a load error, not a silently ignored key

		var m Manifest
		if err := dec.Decode(&m); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		m.dir = dir
		m.path = filepath.Join(dir, "plugin.yaml")

		if err := m.validate(e.Name()); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		out = append(out, Plugin{Manifest: m})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	if len(problems) > 0 {
		return out, fmt.Errorf("plugin problems: %s", strings.Join(problems, "; "))
	}
	return out, nil
}

func (m *Manifest) validate(dirName string) error {
	if m.APIVersion != api.APIVersion {
		return fmt.Errorf("apiVersion %d is not supported (this hub speaks %d)", m.APIVersion, api.APIVersion)
	}
	if m.Name == "" {
		return errors.New("missing name")
	}
	if m.Name != dirName {
		return fmt.Errorf("name %q does not match its directory %q", m.Name, dirName)
	}

	if m.Executable == "" {
		return errors.New("missing executable")
	}
	if filepath.IsAbs(m.Executable) || strings.Contains(m.Executable, "..") {
		return fmt.Errorf("executable %q must be a relative path inside the plugin directory", m.Executable)
	}
	exe := filepath.Join(m.dir, m.Executable)
	info, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("executable %q: %w", m.Executable, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("executable %q is not an executable file", m.Executable)
	}
	if err := rejectShellScript(exe); err != nil {
		return err
	}

	if len(m.Hooks) == 0 {
		return errors.New("no hooks declared")
	}
	seen := map[string]bool{}
	for _, h := range m.Hooks {
		if !api.Hook(h).Known() {
			return fmt.Errorf("unknown hook %q", h)
		}
		if seen[h] {
			return fmt.Errorf("hook %q declared twice", h)
		}
		seen[h] = true
	}

	m.timeout = 60 * time.Second
	if m.Timeout != "" {
		d, err := time.ParseDuration(m.Timeout)
		if err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
		m.timeout = d
	}

	keys := map[string]bool{}
	for _, f := range m.Config {
		if f.Key == "" {
			return errors.New("config field without a key")
		}
		if keys[f.Key] {
			return fmt.Errorf("config field %q declared twice", f.Key)
		}
		keys[f.Key] = true
		switch f.Type {
		case FieldString, FieldSecret, FieldBool, FieldInt, FieldSelect:
		case "":
			return fmt.Errorf("config field %q has no type", f.Key)
		default:
			return fmt.Errorf("config field %q has unknown type %q", f.Key, f.Type)
		}
		if f.Type == FieldSelect && len(f.Options) == 0 {
			return fmt.Errorf("config field %q is a select with no options", f.Key)
		}
	}

	for _, b := range m.Permissions.Exec {
		if !filepath.IsAbs(b) {
			return fmt.Errorf("permissions.exec %q must be absolute", b)
		}
	}
	return nil
}

// rejectShellScript keeps the plugin surface to compiled binaries. A script is
// not a plugin under this contract, and saying so at load time is kinder than a
// confusing failure during a download.
func rejectShellScript(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 2)
	if _, err := f.Read(head); err != nil {
		return nil
	}
	if head[0] == '#' && head[1] == '!' {
		return fmt.Errorf("%s is a script; plugins must be compiled Go binaries", filepath.Base(path))
	}
	return nil
}

// Describe runs the binary's --describe and verifies it against the manifest, so
// a plugin is validated before it is ever dispatched anything.
func (p Plugin) Describe(ctx context.Context) (api.Describe, error) {
	var d api.Describe
	out, err := p.execDescribe(ctx)
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return d, fmt.Errorf("describe is not JSON: %w", err)
	}
	if d.APIVersion != api.APIVersion {
		return d, fmt.Errorf("binary speaks apiVersion %d, manifest declares %d", d.APIVersion, api.APIVersion)
	}
	if d.Name != p.Name {
		return d, fmt.Errorf("binary reports name %q, manifest says %q", d.Name, p.Name)
	}
	implemented := map[string]bool{}
	for _, h := range d.Hooks {
		implemented[h] = true
	}
	for _, h := range p.Hooks {
		if !implemented[h] {
			return d, fmt.Errorf("manifest declares hook %q but the binary does not implement it", h)
		}
	}
	return d, nil
}

func (p Plugin) execDescribe(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, filepath.Join(p.dir, p.Executable), "--describe")
	cmd.Dir = p.dir
	isolateProcess(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("describe failed: %s", msg)
		}
		return nil, fmt.Errorf("describe failed: %w", err)
	}
	return stdout.Bytes(), nil
}

// Names lists installed plugins.
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

func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return len(r.plugins)
}

// Plugins exposes the loaded manifests.
func (r *Registry) Plugins() []Plugin {
	if r == nil {
		return nil
	}
	return r.plugins
}

// Required reports whether a plugin declared a failure as blocking.
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

// Dispatch runs every plugin subscribed to hook. It never stops early: one
// plugin's failure must not cost another the chance to run.
func (r *Registry) Dispatch(ctx context.Context, hook download.Hook, p download.Payload) []download.Outcome {
	if r == nil {
		return nil
	}
	var out []download.Outcome
	for _, pl := range r.plugins {
		if !pl.wants(hook) || !pl.handlesExt(p) {
			continue
		}
		payload := api.Payload{
			Hook:   string(hook),
			Plugin: pl.Name,
			App:    r.app,
			Album:  p.Album,
			Track:  p.Track,
			Files:  p.Files,
			Config: r.configFor(ctx, pl),
		}

		res := r.run(ctx, pl, payload)
		res.Plugin = pl.Name
		res.Hook = string(hook)
		out = append(out, res)
	}
	return out
}

func (pl Plugin) wants(h download.Hook) bool {
	for _, v := range pl.Hooks {
		if v == string(h) {
			return true
		}
	}
	return false
}

func (pl Plugin) handlesExt(p download.Payload) bool {
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

// configFor merges the manifest defaults with whatever the admin saved.
func (r *Registry) configFor(ctx context.Context, pl Plugin) map[string]any {
	values := make(map[string]any, len(pl.Config))
	for _, f := range pl.Config {
		if f.Default != nil {
			values[f.Key] = f.Default
		}
	}
	if r.store == nil {
		return values
	}
	stored, err := r.store.Values(ctx, pl.Name)
	if err != nil {
		r.log.Warn("plugin config unreadable", zap.String("plugin", pl.Name), zap.Error(err))
		return values
	}
	for k, v := range stored {
		values[k] = v
	}
	return values
}

func (r *Registry) run(ctx context.Context, pl Plugin, p api.Payload) download.Outcome {
	body, err := json.Marshal(p)
	if err != nil {
		return download.Outcome{Err: fmt.Errorf("payload: %w", err)}
	}

	cctx, cancel := context.WithTimeout(ctx, pl.timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, filepath.Join(pl.dir, pl.Executable))
	cmd.Dir = p.Album.Dir // plugins work relative to the album, never the hub
	cmd.Stdin = bytes.NewReader(body)
	// PATH is empty on purpose: a plugin runs the binaries it declared in
	// permissions.exec, not whatever happens to be resolvable.
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=/usr/local/bin:/usr/bin:/bin",
	}
	isolateProcess(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	out := download.Outcome{Stderr: strings.TrimSpace(stderr.String())}

	var exitErr *exec.ExitError
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		out.Err = fmt.Errorf("timed out after %s", pl.timeout)
	case err == nil:
	case errors.As(err, &exitErr) && exitErr.ExitCode() == api.ExitSkip:
		out.Skipped = true
	case exitErr != nil:
		out.Err = fmt.Errorf("exit %d", exitErr.ExitCode())
	default:
		out.Err = err
	}

	if trimmed := bytes.TrimSpace(stdout.Bytes()); len(trimmed) > 0 {
		var resp api.Response
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
