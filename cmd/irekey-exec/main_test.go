// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hitoshyamamoto/irekey/internal/version"
)

func TestCapabilities(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"capabilities"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Fatalf("output is not exactly one newline-terminated line: %q", out)
	}
	if !strings.HasPrefix(out, `{"type":"capabilities","executor":{"version":`) {
		t.Errorf("field order changed: %q", out)
	}
	if !strings.Contains(out, `"bundle_formats":[]`) {
		t.Errorf("bundle_formats is not an empty array: %q", out)
	}

	var got struct {
		Type     string `json:"type"`
		Executor struct {
			Version  string `json:"version"`
			Commit   string `json:"commit"`
			Go       string `json:"go"`
			Platform string `json:"platform"`
		} `json:"executor"`
		BundleFormats []int    `json:"bundle_formats"`
		Subcommands   []string `json:"subcommands"`
	}
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	info := version.Get()
	if got.Type != "capabilities" {
		t.Errorf("type = %q", got.Type)
	}
	if got.Executor.Version != info.Version || got.Executor.Commit != info.Commit ||
		got.Executor.Go != info.Go || got.Executor.Platform != info.Platform {
		t.Errorf("executor = %+v, want from %+v", got.Executor, info)
	}
	if got.BundleFormats == nil || len(got.BundleFormats) != 0 {
		t.Errorf("bundle_formats = %#v, want []", got.BundleFormats)
	}
	if !reflect.DeepEqual(got.Subcommands, []string{"capabilities"}) {
		t.Errorf("subcommands = %v", got.Subcommands)
	}
}

func TestWriteCapabilitiesFixed(t *testing.T) {
	var b bytes.Buffer
	info := version.Info{
		Version: "v0.1.0", Commit: "0123456789abcdef0123456789abcdef01234567",
		CommitTime: "2026-10-25T00:57:00Z", Modified: true, Go: "go1.27.3", Platform: "linux/arm64",
	}
	if err := writeCapabilities(&b, info); err != nil {
		t.Fatal(err)
	}
	want := `{"type":"capabilities","executor":{"version":"v0.1.0","commit":"0123456789abcdef0123456789abcdef01234567",` +
		`"go":"go1.27.3","platform":"linux/arm64"},"bundle_formats":[],"subcommands":["capabilities"]}` + "\n"
	if b.String() != want {
		t.Errorf("got  %s\nwant %s", b.String(), want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestCapabilitiesWriteError(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"capabilities"}, failingWriter{}, &stderr); code != exitInternal {
		t.Errorf("exit code = %d, want %d", code, exitInternal)
	}
	if want := "irekey-exec: writing the capabilities: disk full\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{name: "no arguments", args: nil, code: exitUsage, stderr: usage},
		{name: "-h", args: []string{"-h"}, code: exitOK, stdout: usage},
		{name: "--help", args: []string{"--help"}, code: exitOK, stdout: usage},
		{
			name: "unknown subcommand", args: []string{"activate"}, code: exitUsage,
			stderr: "irekey-exec: unknown subcommand \"activate\"\nRun irekey-exec --help to see the subcommands.\n",
		},
		{
			name: "unknown flag", args: []string{"--bogus"}, code: exitUsage,
			stderr: "irekey-exec: unknown subcommand \"--bogus\"\nRun irekey-exec --help to see the subcommands.\n",
		},
		{
			name: "capabilities with an argument", args: []string{"capabilities", "--json"}, code: exitUsage,
			stderr: "irekey-exec: the capabilities subcommand takes no arguments\nRun irekey-exec --help to see the subcommands.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if stdout.String() != tt.stdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.stdout)
			}
			if stderr.String() != tt.stderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.stderr)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	for _, want := range []string{"Usage: irekey-exec <subcommand>", "early development", "  capabilities "} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
}
