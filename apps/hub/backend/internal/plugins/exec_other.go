//go:build !unix

package plugins

import (
	"os/exec"
	"time"
)

// isolateProcess on platforms without process groups: the child is killed, its
// own children may linger until they exit.
func isolateProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = 1500 * time.Millisecond
}
