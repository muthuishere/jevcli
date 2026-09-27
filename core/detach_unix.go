//go:build !windows

package core

import (
	"os/exec"
	"syscall"
)

// detachAttr starts the child in its own session so it outlives the hook's process group.
func detachAttr(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
