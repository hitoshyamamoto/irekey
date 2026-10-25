// SPDX-License-Identifier: MIT

//go:build !unix

package main

import "os/exec"

// ownProcessGroup does nothing: process groups are a Unix facility.
func ownProcessGroup(*exec.Cmd) {}

// interruptGroup kills the process: sending an interrupt is not implemented
// on Windows (os.Process.Signal).
func interruptGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
