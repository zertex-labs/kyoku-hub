//go:build !linux

package plugins

import (
	"context"
	"fmt"
	"os/exec"
)

// SandboxSpec is the confinement a plugin runs under. On platforms without
// Landlock there is no way to enforce one, so plugins do not run at all.
type SandboxSpec struct{}

const SandboxFlag = "--sandbox-exec"

func SpecFor(_, _, _ string, _ bool) SandboxSpec { return SandboxSpec{} }

func SandboxedCommand(ctx context.Context, spec SandboxSpec, bin string, args ...string) (*exec.Cmd, error) {
	return nil, fmt.Errorf("plugins need kernel-enforced confinement; this platform has none")
}

func RunSandboxed(encoded string, argv []string) error {
	return fmt.Errorf("plugins are not supported on this platform")
}
