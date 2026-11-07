package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writePlugin lays down a plugin directory with a shell script as its executable.
func writePlugin(t *testing.T, root, name, manifest, script string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	bin := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("script: %v", err)
	}
	return dir
}

func TestLoadReadsManifestsAndAppliesDefaults(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "echo", `
name: echo
command: ["./run.sh"]
hooks: [track.tagged]
timeout: 2s
`, "cat > /dev/null\n")

	// A directory without a manifest is not a plugin.
	if err := os.MkdirAll(filepath.Join(root, "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	ps, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(ps) != 1 {
		t.Fatalf("plugins = %d, want 1", len(ps))
	}
	if ps[0].Name != "echo" || len(ps[0].Hooks) != 1 || ps[0].Hooks[0] != "track.tagged" {
		t.Fatalf("manifest parsed wrong: %+v", ps[0].Manifest)
	}
	if ps[0].timeout != 2*time.Second {
		t.Fatalf("timeout = %s", ps[0].timeout)
	}
}

func TestLoadMissingDirectoryIsFine(t *testing.T) {
	ps, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil || ps != nil {
		t.Fatalf("got %v, %v", ps, err)
	}
}

func TestDispatchOnlyRunsSubscribedHooks(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "tag", `
name: tag
command: ["./run.sh"]
hooks: [track.tagged]
`, "cat > payload.json\n")
	_ = dir

	reg, err := NewRegistry(root, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	// Wrong hook: nothing runs.
	out := reg.Dispatch(context.Background(), HookTrackDownloaded, Payload{Album: Album{Title: "X"}})
	if len(out) != 0 {
		t.Fatalf("ran for an unsubscribed hook: %+v", out)
	}

	// Right hook: runs and receives the payload on stdin.
	out = reg.Dispatch(context.Background(), HookTrackTagged, Payload{
		Album: Album{Title: "DND", Artist: "Ado", Dir: "/tmp/album"},
		Track: &Track{Path: "/tmp/album/01 - DND.flac", Ext: ".flac", ISRC: "JPPO02601234"},
	})
	if len(out) != 1 || out[0].Err != nil {
		t.Fatalf("dispatch: %+v", out)
	}

	raw, err := os.ReadFile(filepath.Join(root, "tag", "payload.json"))
	if err != nil {
		t.Fatalf("plugin did not receive stdin: %v", err)
	}
	var got Payload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload not json: %v (%s)", err, raw)
	}
	if got.Hook != "track.tagged" || got.Plugin != "tag" {
		t.Errorf("hook/plugin = %q/%q", got.Hook, got.Plugin)
	}
	if got.Album.Title != "DND" || got.Track == nil {
		t.Errorf("payload content = %+v", got)
	}
	if got.Track != nil && got.Track.ISRC != "JPPO02601234" {
		t.Errorf("isrc = %q", got.Track.ISRC)
	}
}

func TestDispatchReportsSkipAndFailure(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "skipper", `
name: skipper
command: ["./run.sh"]
hooks: [album.published]
`, "exit 3\n")
	writePlugin(t, root, "broken", `
name: broken
command: ["./run.sh"]
hooks: [album.published]
`, "echo 'boom happened' >&2\nexit 1\n")

	reg, err := NewRegistry(root, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	out := reg.Dispatch(context.Background(), HookAlbumPublished, Payload{})
	if len(out) != 2 {
		t.Fatalf("outcomes = %d", len(out))
	}
	for _, o := range out {
		switch o.Plugin {
		case "skipper":
			if !o.Skipped {
				t.Errorf("skipper not reported as skipped: %+v", o)
			}
		case "broken":
			if o.Err == nil || o.Skipped {
				t.Errorf("broken not reported as failed: %+v", o)
			}
			if !strings.Contains(o.Stderr, "boom happened") {
				t.Errorf("stderr not captured: %q", o.Stderr)
			}
		}
	}
}

func TestDispatchHonoursTimeout(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "slow", `
name: slow
command: ["./run.sh"]
hooks: [album.published]
timeout: 300ms
`, "sleep 5\n")

	reg, err := NewRegistry(root, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	start := time.Now()
	out := reg.Dispatch(context.Background(), HookAlbumPublished, Payload{})
	if len(out) != 1 || out[0].Err == nil {
		t.Fatalf("expected a timeout failure: %+v", out)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("timeout not enforced: %s", d)
	}
}

func TestDispatchFiltersByExtension(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "flaconly", `
name: flaconly
command: ["./run.sh"]
hooks: [track.tagged]
extensions: [".flac"]
`, "cat > /dev/null\n")

	reg, _ := NewRegistry(root, nil)
	if out := reg.Dispatch(context.Background(), HookTrackTagged, Payload{
		Track: &Track{Ext: ".mp3"},
	}); len(out) != 0 {
		t.Fatalf("ran for an excluded extension: %+v", out)
	}
	if out := reg.Dispatch(context.Background(), HookTrackTagged, Payload{
		Track: &Track{Ext: ".flac"},
	}); len(out) != 1 {
		t.Fatalf("did not run for an included extension: %+v", out)
	}
}

func TestDispatchMissingExecutableIsAFailure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ghost")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(`
name: ghost
command: ["./does-not-exist"]
hooks: [album.published]
`), 0o644)

	reg, _ := NewRegistry(root, nil)
	out := reg.Dispatch(context.Background(), HookAlbumPublished, Payload{})
	if len(out) != 1 || out[0].Err == nil {
		t.Fatalf("expected failure: %+v", out)
	}
	if !strings.Contains(out[0].Err.Error(), "executable not found") {
		t.Errorf("err = %v", out[0].Err)
	}
}

func TestNilRegistryIsInert(t *testing.T) {
	var reg *Registry
	if out := reg.Dispatch(context.Background(), HookTrackTagged, Payload{}); out != nil {
		t.Fatalf("got %+v", out)
	}
	if reg.Count() != 0 || reg.Names() != nil {
		t.Fatal("nil registry reported plugins")
	}
}

// Shell plugins should not have to parse JSON for the fields they always want.
func TestDispatchMirrorsPayloadIntoEnv(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "env", `
name: env
command: ["./run.sh"]
hooks: [track.tagged]
`, `printf '%s|%s|%s|%s' "$KYOKU_ALBUM_DIR" "$KYOKU_ALBUM_TITLE" "$KYOKU_TRACK_TITLE" "$KYOKU_TRACK_ISRC" > env.txt
`)

	reg, err := NewRegistry(root, nil)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reg.Dispatch(context.Background(), HookTrackTagged, Payload{
		Album: Album{Title: "Raven Sky", Artist: "Ado", Dir: "/music/album"},
		Track: &Track{Path: "/music/album/01 - Raven Sky.flac", Title: "Raven Sky", ISRC: "JPPO02603857"},
	})

	raw, err := os.ReadFile(filepath.Join(root, "env", "env.txt"))
	if err != nil {
		t.Fatalf("plugin did not run: %v", err)
	}
	want := "/music/album|Raven Sky|Raven Sky|JPPO02603857"
	if string(raw) != want {
		t.Fatalf("env = %q, want %q", raw, want)
	}
}
