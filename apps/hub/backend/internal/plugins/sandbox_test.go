//go:build linux

package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary act as the hub's sandbox launcher, so the real
// confinement path is exercised instead of a stub: the tests exec a probe plugin
// through the same SandboxFlag the hub uses.
func TestMain(m *testing.M) {
	if len(os.Args) > 3 && os.Args[1] == SandboxFlag {
		if err := RunSandboxed(os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

// probeSource is a plugin that reports whether it can read and write the paths it
// was given, then exits.
const probeSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	readPath, writePath := os.Args[1], os.Args[2]
	if _, err := os.ReadFile(readPath); err != nil {
		fmt.Println("read:denied")
	} else {
		fmt.Println("read:allowed")
	}
	if err := os.WriteFile(writePath, []byte("x"), 0o644); err != nil {
		fmt.Println("write:denied")
	} else {
		fmt.Println("write:allowed")
	}
}
`

func buildProbe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(probeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module probe\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "probe")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build probe: %v: %s", err, out)
	}
	return bin
}

func runProbe(t *testing.T, bin, albumDir, tempDir, readPath, writePath string) string {
	t.Helper()
	spec := SpecFor(filepath.Dir(bin), albumDir, tempDir, false)
	cmd, err := SandboxedCommand(t.Context(), spec, bin, readPath, writePath)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe failed: %v: %s", err, out)
	}
	return string(out)
}

// The point of the whole thing: a plugin cannot read the hub's secrets, and
// cannot write anywhere it was not given.
func TestSandboxConfinesFileAccess(t *testing.T) {
	bin := buildProbe(t)
	album := t.TempDir()
	temp := t.TempDir()

	// A secret planted where the hub's own config lives.
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.env")
	if err := os.WriteFile(secret, []byte("SESSION_SECRET=nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(secretDir, "written.txt")

	out := runProbe(t, bin, album, temp, secret, outside)

	if strings.Contains(out, "read:allowed") {
		t.Errorf("plugin read a file outside the album:\n%s", out)
	}
	if strings.Contains(out, "write:allowed") {
		t.Errorf("plugin wrote outside the album:\n%s", out)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("file appeared outside the sandbox")
	}
}

func TestSandboxAllowsTheAlbumAndTemp(t *testing.T) {
	bin := buildProbe(t)
	album := t.TempDir()
	temp := t.TempDir()

	inside := filepath.Join(album, "track.flac")
	if err := os.WriteFile(inside, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(album, "lyrics.lrc")

	out := runProbe(t, bin, album, temp, inside, target)

	if !strings.Contains(out, "read:allowed") {
		t.Errorf("plugin could not read its own album:\n%s", out)
	}
	if !strings.Contains(out, "write:allowed") {
		t.Errorf("plugin could not write to the album:\n%s", out)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file was not written: %v", err)
	}
}

func TestSandboxDeniesNetwork(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", time.Second)
	if err != nil {
		fmt.Println("net:denied")
		return
	}
	conn.Close()
	fmt.Println("net:allowed")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module netprobe\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "netprobe")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build probe: %v: %s", err, out)
	}

	spec := SpecFor(dir, t.TempDir(), t.TempDir(), false)
	cmd, err := SandboxedCommand(t.Context(), spec, bin)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "net:denied") {
		t.Fatalf("network was reachable from a sandboxed plugin:\n%s", out)
	}
}
