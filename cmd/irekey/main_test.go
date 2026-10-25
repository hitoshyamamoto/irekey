// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hitoshyamamoto/irekey/internal/version"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		code       int
		stdout     string // exact
		stderr     string // exact
		stderrPart string // substring, when stderr is not exact
	}{
		{name: "no arguments", args: nil, code: exitUsage, stderr: usage},
		{name: "-h", args: []string{"-h"}, code: exitOK, stdout: usage},
		{name: "--help", args: []string{"--help"}, code: exitOK, stdout: usage},
		{name: "help", args: []string{"help"}, code: exitOK, stdout: usage},
		{name: "version", args: []string{"version"}, code: exitOK, stdout: versionLine(version.Get()) + "\n"},
		{
			name: "version with an argument", args: []string{"version", "extra"}, code: exitUsage,
			stderr: "irekey: the version command takes no arguments\nRun irekey --help to see the commands.\n",
		},
		{
			name: "unknown command", args: []string{"deploy"}, code: exitUsage,
			stderr: "irekey: unknown command \"deploy\"\nRun irekey --help to see the commands.\n",
		},
		{name: "unknown flag", args: []string{"--bogus"}, code: exitUsage, stderrPart: `unknown command "--bogus"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if stdout.String() != tt.stdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.stdout)
			}
			if tt.stderrPart != "" {
				if !strings.Contains(stderr.String(), tt.stderrPart) {
					t.Errorf("stderr = %q, want containing %q", stderr.String(), tt.stderrPart)
				}
			} else if stderr.String() != tt.stderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.stderr)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	for _, want := range []string{"Usage: irekey <command>", "early development", "  version "} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
}

func TestVersionLine(t *testing.T) {
	tests := []struct {
		info version.Info
		want string
	}{
		{
			version.Info{Version: "v0.1.0", Commit: "0123456789abcdef0123456789abcdef01234567", Go: "go1.27.3", Platform: "linux/amd64"},
			"irekey v0.1.0 (commit 0123456789ab, go1.27.3, linux/amd64)",
		},
		{
			version.Info{Version: "dev", Commit: "unknown", Go: "go1.27.3", Platform: "linux/arm64"},
			"irekey dev (commit unknown, go1.27.3, linux/arm64)",
		},
		{
			version.Info{Version: "dev", Commit: "0123456789ab", Go: "go1.27.3", Platform: "darwin/arm64"},
			"irekey dev (commit 0123456789ab, go1.27.3, darwin/arm64)",
		},
	}
	for _, tt := range tests {
		if got := versionLine(tt.info); got != tt.want {
			t.Errorf("versionLine(%+v) = %q, want %q", tt.info, got, tt.want)
		}
	}
}
