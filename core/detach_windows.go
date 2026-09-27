//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

// detachAttr starts the child as a detached process with no console.
func detachAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
}
