//go:build unix

package plugins

import (
	"os/exec"
	"syscall"
	"time"
)

// isolateProcess puts the plugin in its own process group so a timeout can kill
// it and everything it spawned, and bounds how long Wait blocks on inherited
// pipes after a kill.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 1500 * time.Millisecond
}
