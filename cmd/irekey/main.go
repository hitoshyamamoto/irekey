// SPDX-License-Identifier: MIT

// Command irekey is the operator command line of irekey.
//
// This is a placeholder: only the version command exists so far.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/hitoshyamamoto/irekey/internal/version"
)

// Exit codes. They are a contract, documented in the README once the
// full command line exists.
const (
	exitOK    = 0
	exitUsage = 1
)

const usage = `Usage: irekey <command>

irekey is in early development: only the version command exists so far.

Commands:
  version     print the version, commit, Go version and platform

Options:
  -h, --help  show this help
`

// versionLine is the output of irekey version.
func versionLine(info version.Info) string {
	commit := info.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return fmt.Sprintf("irekey %s (commit %s, %s, %s)", info.Version, commit, info.Go, info.Platform)
}

// usageFailure reports an invalid command line and returns its exit code.
func usageFailure(stderr io.Writer, msg string) int {
	_, _ = fmt.Fprintf(stderr, "irekey: %s\nRun irekey --help to see the commands.\n", msg)
	return exitUsage
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return exitUsage
	}
	switch cmd := args[0]; cmd {
	case "-h", "--help", "help":
		_, _ = io.WriteString(stdout, usage)
		return exitOK
	case "version":
		if len(args) > 1 {
			return usageFailure(stderr, "the version command takes no arguments")
		}
		_, _ = fmt.Fprintln(stdout, versionLine(version.Get()))
		return exitOK
	default:
		return usageFailure(stderr, fmt.Sprintf("unknown command %q", cmd))
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
