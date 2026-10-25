// SPDX-License-Identifier: MIT

//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ownProcessGroup makes cmd start in a process group of its own, whose ID is
// the child's process ID. The processes the child starts (for go build: the
// compiler and the linker) join that group.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interruptGroup sends an interrupt to the process group that cmd leads, so
// it reaches the processes cmd started as well. A group that no longer
// exists is reported as os.ErrProcessDone.
func interruptGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
