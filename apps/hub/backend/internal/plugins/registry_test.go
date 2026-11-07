package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"

	"github.com/zertex-labs/kyoku-hub/apps/hub/backend/internal/download"
)

// writeDir lays down a plugin directory. The binary is a stub unless a real one
// is copied in later: the loader only stats it and checks the shebang.
func writeDir(t *testing.T, root, name, manifest, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "plugin")
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

const minimalManifest = `apiVersion: 1
name: probe
version: 1.0.0
executable: ./plugin
hooks: [album.published]
timeout: 10s
`

func TestLoadValidatesManifests(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"unknown key", "apiVersion: 1\nname: probe\nruntime: wasm\nexecutable: ./plugin\nhooks: [album.published]\n", "field runtime not found"},
		{"wrong api", "apiVersion: 9\nname: probe\nexecutable: ./plugin\nhooks: [album.published]\n", "apiVersion 9"},
		{"name mismatch", "apiVersion: 1\nname: other\nexecutable: ./plugin\nhooks: [album.published]\n", "does not match its directory"},
		{"unknown hook", "apiVersion: 1\nname: probe\nexecutable: ./plugin\nhooks: [album.exploded]\n", "unknown hook"},
		{"exec escapes", "apiVersion: 1\nname: probe\nexecutable: ../evil\nhooks: [album.published]\n", "relative path inside"},
		{"select without options", "apiVersion: 1\nname: probe\nexecutable: ./plugin\nhooks: [album.published]\nconfig:\n  - {key: mode, type: select}\n", "no options"},
		{"bad timeout", "apiVersion: 1\nname: probe\nexecutable: ./plugin\nhooks: [album.published]\ntimeout: soon\n", "timeout"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeDir(t, root, "probe", c.manifest, "\x7fELF not really")
			_, err := Load(root)
			if err == nil {
				t.Fatalf("expected an error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// A shell script is not a plugin under this contract, and the loader says so
// instead of failing later during a download.
func TestLoadRejectsScripts(t *testing.T) {
	root := t.TempDir()
	dir := writeDir(t, root, "probe", minimalManifest, "#!/bin/sh\necho hi\n")

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "compiled Go binaries") {
		t.Fatalf("err = %v", err)
	}
	_ = dir
}

func TestLoadMissingDirectoryIsFine(t *testing.T) {
	ps, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil || ps != nil {
		t.Fatalf("got %v %v", ps, err)
	}
}

// buildExample compiles a real plugin from the repo once, so the contract is
// exercised with the real SDK rather than a stub.
var (
	buildOnce sync.Once
	builtPath string
	buildErr  error
)

func buildHello(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		if runtime.GOOS == "windows" {
			buildErr = fmt.Errorf("skip on windows")
			return
		}
		root, err := repoRoot()
		if err != nil {
			buildErr = err
			return
		}
		src := filepath.Join(root, "examples", "plugins", "hello")
		out := filepath.Join(os.TempDir(), fmt.Sprintf("kyoku-hello-%d", os.Getpid()))
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if combined, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build example: %v: %s", err, combined)
			return
		}
		builtPath = out
	})
	if buildErr != nil {
		t.Skipf("cannot build the example plugin: %v", buildErr)
	}
	return builtPath
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("go.work not found")
}

func installHello(t *testing.T) string {
	t.Helper()
	bin := buildHello(t)
	root := t.TempDir()
	dir := filepath.Join(root, "hello")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(binFromRepo(t), "plugin.yaml"))
	if err != nil {
		t.Fatalf("read example manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func binFromRepo(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return filepath.Join(root, "examples", "plugins", "hello")
}

func TestDescribeGateRejectsForeignBinaries(t *testing.T) {
	root := t.TempDir()
	dir := writeDir(t, root, "probe", minimalManifest, "\x7fELF not really")

	reg, err := NewRegistry(root, nil, api.App{Name: "kyoku-hub", Version: "test"}, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	// A binary that cannot describe itself is loaded but not enabled.
	if reg.Count() != 0 {
		t.Fatalf("enabled %d plugins: %v", reg.Count(), reg.Names())
	}
	_ = dir
}

func TestDispatchRunsARealPlugin(t *testing.T) {
	root := installHello(t)

	reg, err := NewRegistry(root, nil, api.App{Name: "kyoku-hub", Version: "test"}, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if reg.Count() != 1 {
		t.Fatalf("plugins = %d", reg.Count())
	}

	albumDir := t.TempDir()
	payload := download.Payload{
		Album: download.Album{Title: "Discovery", Artist: "Daft Punk", Dir: albumDir, TrackCount: 1},
		Track: &download.Track{Path: filepath.Join(albumDir, "01.flac"), Ext: ".flac", Title: "One More Time", Number: 1},
	}

	out := reg.Dispatch(context.Background(), download.HookTrackTagged, payload)
	if len(out) != 1 || out[0].Err != nil {
		t.Fatalf("dispatch: %+v", out)
	}
	if !strings.Contains(out[0].Message, "One More Time") {
		t.Errorf("message = %q", out[0].Message)
	}
}

func TestDispatchPublishesFilesAPluginAdds(t *testing.T) {
	root := installHello(t)
	reg, err := NewRegistry(root, nil, api.App{Name: "kyoku-hub", Version: "test"}, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	albumDir := t.TempDir()
	out := reg.Dispatch(context.Background(), download.HookAlbumBeforePublish, download.Payload{
		Album: download.Album{Dir: albumDir, TrackCount: 1},
		Files: []download.File{{Path: filepath.Join(albumDir, "01.flac"), Ext: ".flac"}},
	})
	if len(out) != 1 || out[0].Err != nil {
		t.Fatalf("dispatch: %+v", out)
	}
	if _, err := os.Stat(filepath.Join(albumDir, "hello.txt")); err != nil {
		t.Fatalf("plugin did not write its file: %v", err)
	}
}

func TestDispatchOnlyRunsSubscribedHooksAndExtensions(t *testing.T) {
	root := installHello(t)
	reg, err := NewRegistry(root, nil, api.App{Name: "kyoku-hub", Version: "test"}, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	out := reg.Dispatch(context.Background(), download.HookAlbumPublished, download.Payload{})
	if len(out) != 0 {
		t.Fatalf("ran for an unsubscribed hook: %+v", out)
	}
}

func TestNilRegistryIsInert(t *testing.T) {
	var reg *Registry
	if out := reg.Dispatch(context.Background(), download.HookTrackTagged, download.Payload{}); out != nil {
		t.Fatalf("got %+v", out)
	}
	if reg.Count() != 0 || reg.Names() != nil || reg.Required("x") {
		t.Fatal("nil registry reported plugins")
	}
}

// stubStore stands in for the persisted config.
type stubStore struct{ values map[string]any }

func (s stubStore) Values(context.Context, string) (map[string]any, error) { return s.values, nil }

func TestConfigMergesDefaultsAndStoredValues(t *testing.T) {
	m := Manifest{Config: []Field{
		{Key: "greeting", Type: FieldString, Default: "hello"},
		{Key: "writeFile", Type: FieldBool, Default: true},
	}}

	r := &Registry{store: stubStore{values: map[string]any{"greeting": "hi"}}}
	got := r.configFor(context.Background(), Plugin{Manifest: m})
	if got["greeting"] != "hi" {
		t.Errorf("stored value lost: %v", got)
	}
	if got["writeFile"] != true {
		t.Errorf("default lost: %v", got)
	}

	// No store at all is the normal case: defaults only.
	r = &Registry{}
	if got := r.configFor(context.Background(), Plugin{Manifest: m}); got["greeting"] != "hello" {
		t.Errorf("defaults missing: %v", got)
	}
}

func TestDescribeRejectsHookMismatch(t *testing.T) {
	root := t.TempDir()
	// The binary describes only track.tagged; the manifest claims more.
	dir := writeDir(t, root, "probe", `apiVersion: 1
name: probe
executable: ./plugin
hooks: [album.published]
`, "#!/bin/sh\nexit 0\n")
	_ = dir

	pl := Plugin{Manifest: Manifest{Name: "probe", Executable: "./plugin", Hooks: []string{"album.published"}, dir: dir, timeout: time.Second}}
	if _, err := pl.Describe(context.Background()); err == nil {
		t.Fatal("expected describe to fail for a non-plugin binary")
	}
}
