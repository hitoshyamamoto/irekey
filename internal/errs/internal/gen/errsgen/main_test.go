// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitoshyamamoto/irekey/internal/errs/internal/gen"
)

// registry is a small valid registry.
const registry = `categories:
  - prefix: USR
    title: operator input
    covers: unknown names
codes:
  - code: USR-001
    category: USR
    title: unknown name
    condition: unknown device
    exit: 1
    device_state: unchanged
    kind: error
    status: reserved
    since: "0.1.0"
`

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runErrsgen(args ...string) (int, string) {
	var stderr bytes.Buffer
	status := run(args, &stderr)
	return status, stderr.String()
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "codes.yaml")
	goOut := filepath.Join(dir, "zz_generated_codes.go")
	mdOut := filepath.Join(dir, "errors.md")
	write(t, in, registry)

	if status, out := runErrsgen("-in", in, "-go", goOut, "-md", mdOut); status != exitClean || out != "" {
		t.Fatalf("errsgen = %d, %q", status, out)
	}
	reg, err := gen.Load(in)
	if err != nil {
		t.Fatal(err)
	}
	wantGo, err := gen.RenderGo(reg)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{goOut: wantGo, mdOut: gen.RenderMarkdown(reg)} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s does not hold the rendered registry (%v)", path, err)
		}
	}

	// A second run finds the files up to date and does not write them.
	old := time.Unix(1_000_000_000, 0)
	for _, p := range []string{goOut, mdOut} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if status, out := runErrsgen("-in", in, "-go", goOut, "-md", mdOut); status != exitClean {
		t.Fatalf("second run = %d, %q", status, out)
	}
	for _, p := range []string{goOut, mdOut} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("%s was written again although it was up to date", p)
		}
	}

	// Validation alone writes nothing.
	if status, out := runErrsgen("-in", in); status != exitClean || out != "" {
		t.Errorf("errsgen -in = %d, %q", status, out)
	}
}

func TestGenerateInvalidRegistry(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "codes.yaml")
	goOut := filepath.Join(dir, "zz_generated_codes.go")
	write(t, in, strings.Replace(registry, "exit: 1", "exit: 7", 1))

	status, out := runErrsgen("-in", in, "-go", goOut)
	if status != exitFindings ||
		!strings.Contains(out, in+":6: USR-001: exit 7") ||
		!strings.Contains(out, "is invalid; nothing was written") {
		t.Errorf("errsgen = %d, %q", status, out)
	}
	if _, err := os.Stat(goOut); err == nil {
		t.Error("the Go file was written for an invalid registry")
	}

	write(t, in, "extra: 1\n")
	if status, out := runErrsgen("-in", in); status != exitFindings || !strings.Contains(out, "field extra not found") {
		t.Errorf("errsgen on an undecodable registry = %d, %q", status, out)
	}
}

func TestGenerateWriteErrors(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "codes.yaml")
	write(t, in, registry)
	missing := filepath.Join(dir, "missing", "out")
	for _, flag := range []string{"-go", "-md"} {
		if status, out := runErrsgen("-in", in, flag, missing); status != exitError || !strings.HasPrefix(out, "error: ") {
			t.Errorf("errsgen %s into a missing directory = %d, %q", flag, status, out)
		}
	}
	// An output path that cannot be read is not overwritten blindly.
	if status, _ := runErrsgen("-in", in, "-md", dir); status != exitError {
		t.Errorf("errsgen -md onto a directory = %d", status)
	}
}

func TestCheckMode(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "internal", "errs", "codes.yaml"), registry)
	reg, err := gen.Load(filepath.Join(root, "internal", "errs", "codes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := gen.RenderGo(reg)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "internal", "errs", "zz_generated_codes.go"), string(src))
	write(t, filepath.Join(root, "docs", "errors.md"), string(gen.RenderMarkdown(reg)))

	if status, out := runErrsgen("-check", root); status != exitClean || out != "" {
		t.Fatalf("errsgen -check on a clean tree = %d, %q", status, out)
	}

	write(t, filepath.Join(root, "cmd", "x", "main.go"), "package main\n\nvar c = \"USR-001\"\n")
	status, out := runErrsgen("-check", root)
	want := "cmd/x/main.go:3: string literal contains the error code USR-001: use the generated constant errs.USR001\n" +
		"error: errors guard: 1 finding(s)\n"
	if status != exitFindings || out != want {
		t.Errorf("errsgen -check = %d, %q\nwant %d, %q", status, out, exitFindings, want)
	}

	if status, out := runErrsgen("-check", filepath.Join(root, "missing")); status != exitError || !strings.HasPrefix(out, "error: errors: ") {
		t.Errorf("errsgen -check on a missing root = %d, %q", status, out)
	}
}

func TestUsage(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		status int
		want   string
	}{
		{"help", []string{"-h"}, exitClean, "Usage of errsgen"},
		{"unknown flag", []string{"-x"}, exitError, "flag provided but not defined"},
		{"no mode", nil, exitError, "error: -in or -check is required"},
		{"argument", []string{"-in", "codes.yaml", "extra"}, exitError, `error: unexpected argument "extra"`},
		{"check with in", []string{"-check", ".", "-in", "codes.yaml"}, exitError, "error: -check cannot be combined"},
		{"check with go", []string{"-check", ".", "-go", "x.go"}, exitError, "error: -check cannot be combined"},
		{"check with md", []string{"-check", ".", "-md", "x.md"}, exitError, "error: -check cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, out := runErrsgen(tt.args...)
			if status != tt.status || !strings.Contains(out, tt.want) {
				t.Errorf("errsgen %v = %d, %q; want %d and %q", tt.args, status, out, tt.status, tt.want)
			}
		})
	}
}
