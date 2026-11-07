//go:build linux

package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
)

// SandboxSpec is the confinement a plugin runs under. It is built by the hub and
// applied by the hub's own binary in a child process, immediately before the
// plugin is exec'd, so the ruleset is inherited by everything the plugin starts
// and cannot be widened.
type SandboxSpec struct {
	// RO are paths the plugin may read and execute.
	RO []string `json:"ro"`
	// RW are paths the plugin may also write: the album directory and one
	// private temp directory, nothing else.
	RW []string `json:"rw"`
	// Network permits outbound TCP. Off unless the manifest asked and the admin
	// granted it.
	Network bool `json:"network"`
	// CPUSeconds and MemBytes are hard limits on the child.
	CPUSeconds int   `json:"cpuSeconds"`
	MemBytes   int64 `json:"memBytes"`
}

// SandboxFlag is how the hub re-execs itself as a launcher.
const SandboxFlag = "--sandbox-exec"

// systemReadOnly is the smallest set that lets a statically linked Go binary and
// a tool like rsgain run: the binary directories, their libraries, and the
// locale/timezone data every process touches. The hub's own files, the user's
// home and the database are deliberately absent.
var systemReadOnlyDirs = []string{
	"/usr",
	"/lib",
	"/lib64",
	"/bin",
	"/sbin",
	"/proc/self",
}

// Individual files need their own rule: directory access rights applied to a
// file are rejected by the kernel, not silently ignored.
var systemReadOnlyFiles = []string{
	"/etc/ld.so.cache",
	"/etc/localtime",
	"/etc/resolv.conf",
	"/etc/nsswitch.conf",
	"/dev/null",
	"/dev/urandom",
}

// SpecFor builds the sandbox a plugin gets: read-only system paths, read-write
// album and temp, and no network unless the manifest declared it.
func SpecFor(pluginDir, albumDir, tempDir string, network bool) SandboxSpec {
	ro := append([]string{}, systemReadOnlyDirs...)
	ro = append(ro, systemReadOnlyFiles...)
	// The plugin's own directory must be readable and executable, or the exec
	// itself is denied.
	ro = append(ro, pluginDir)

	rw := []string{albumDir, tempDir}

	spec := SandboxSpec{RO: dedupe(ro), RW: dedupe(rw), Network: network, CPUSeconds: 900, MemBytes: 2 << 30}
	return spec
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = filepath.Clean(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// SandboxedCommand builds the command that runs bin under the spec: the hub
// binary, told to confine itself and then exec the plugin.
func SandboxedCommand(ctx context.Context, spec SandboxSpec, bin string, args ...string) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve hub executable: %w", err)
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	argv := append([]string{SandboxFlag, base64.StdEncoding.EncodeToString(encoded), bin}, args...)
	return exec.CommandContext(ctx, self, argv...), nil
}

// RunSandboxed is the child side: apply the spec, then become the plugin.
func RunSandboxed(encoded string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("sandbox: no command given")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}
	var spec SandboxSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}
	if err := applySandbox(spec); err != nil {
		return err
	}

	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}
	return syscall.Exec(bin, argv, os.Environ())
}

func applySandbox(spec SandboxSpec) error {
	// Limits first: they are cheap and do not depend on the kernel's Landlock
	// support, so a plugin cannot fork a memory bomb even on an old kernel.
	limits := []struct {
		resource int
		value    uint64
	}{
		{unix.RLIMIT_CPU, uint64(spec.CPUSeconds)},
		{unix.RLIMIT_NOFILE, 512},
	}
	if spec.MemBytes > 0 {
		limits = append(limits, struct {
			resource int
			value    uint64
		}{unix.RLIMIT_AS, uint64(spec.MemBytes)})
	}
	for _, l := range limits {
		lim := unix.Rlimit{Cur: l.value, Max: l.value}
		if err := unix.Setrlimit(l.resource, &lim); err != nil {
			return fmt.Errorf("sandbox: setrlimit %d: %w", l.resource, err)
		}
	}

	rules := []landlock.Rule{}
	for _, p := range spec.RO {
		if !exists(p) {
			continue
		}
		if isDir(p) {
			rules = append(rules, landlock.RODirs(p))
		} else {
			rules = append(rules, landlock.ROFiles(p))
		}
	}
	for _, p := range spec.RW {
		if !exists(p) {
			continue
		}
		if isDir(p) {
			rules = append(rules, landlock.RWDirs(p))
		} else {
			rules = append(rules, landlock.RWFiles(p))
		}
	}

	// Deliberately not BestEffort: silently degrading to "no confinement" is the
	// failure mode this exists to prevent. An unsupported kernel means the plugin
	// does not run.
	if err := landlock.V4.RestrictPaths(rules...); err != nil {
		return fmt.Errorf("sandbox: landlock paths: %w", err)
	}
	if !spec.Network {
		if err := landlock.V4.RestrictNet(); err != nil {
			return fmt.Errorf("sandbox: landlock network: %w", err)
		}
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// readOnlyFiles is the file half of the system policy, exported for the spec.
func readOnlyFiles() []string { return append([]string{}, systemReadOnlyFiles...) }
