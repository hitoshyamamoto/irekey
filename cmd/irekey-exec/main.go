// SPDX-License-Identifier: MIT

// Command irekey-exec is the device-side executor of irekey. The operator
// command line runs it on the device over SSH. It never listens on a port
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md). It uses the standard library only so that the
// binary stays small.
//
// This is a placeholder: only the capabilities subcommand exists so far.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hitoshyamamoto/irekey/internal/version"
)

// Exit codes. The executor's full set is settled together with its
// event envelope.
const (
	exitOK    = 0
	exitUsage = 1
	// exitInternal reports that the output could not be written. It
	// matches the operator command line's code for an internal error.
	exitInternal = 15
)

const usage = `Usage: irekey-exec <subcommand>

irekey-exec runs on the device, started by the irekey command line over
SSH; it is not meant to be run by hand. It is in early development: only
the capabilities subcommand exists so far.

Subcommands:
  capabilities  print what this executor supports, as one JSON line

Options:
  -h, --help    show this help
`

// subcommands lists the subcommands this executor supports.
var subcommands = []string{"capabilities"}

// executorInfo identifies the executor binary.
type executorInfo struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

// capabilitiesMessage is the output of irekey-exec capabilities. The
// field order is the order of the JSON object.
type capabilitiesMessage struct {
	Type          string       `json:"type"`
	Executor      executorInfo `json:"executor"`
	BundleFormats []int        `json:"bundle_formats"`
	Subcommands   []string     `json:"subcommands"`
}

// The executor's machine-readable output is newline-delimited JSON on
// stdout, one object per line. The envelope shared by all messages (and the
// error codes they carry) is settled when device-side operations exist.

// writeCapabilities writes the capabilities message as one
// newline-terminated JSON object.
func writeCapabilities(w io.Writer, info version.Info) error {
	return json.NewEncoder(w).Encode(capabilitiesMessage{
		Type: "capabilities",
		Executor: executorInfo{
			Version:  info.Version,
			Commit:   info.Commit,
			Go:       info.Go,
			Platform: info.Platform,
		},
		BundleFormats: []int{}, // bundle format numbers; encoded as [], never null
		Subcommands:   subcommands,
	})
}

// usageFailure reports an invalid command line and returns its exit code.
func usageFailure(stderr io.Writer, msg string) int {
	_, _ = fmt.Fprintf(stderr, "irekey-exec: %s\nRun irekey-exec --help to see the subcommands.\n", msg)
	return exitUsage
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return exitUsage
	}
	switch sub := args[0]; sub {
	case "-h", "--help":
		_, _ = io.WriteString(stdout, usage)
		return exitOK
	case "capabilities":
		if len(args) > 1 {
			return usageFailure(stderr, "the capabilities subcommand takes no arguments")
		}
		if err := writeCapabilities(stdout, version.Get()); err != nil {
			_, _ = fmt.Fprintf(stderr, "irekey-exec: writing the capabilities: %v\n", err)
			return exitInternal
		}
		return exitOK
	default:
		return usageFailure(stderr, fmt.Sprintf("unknown subcommand %q", sub))
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
