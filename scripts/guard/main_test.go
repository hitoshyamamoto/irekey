// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Values the guards must reject are assembled at run time, so that the
// files of this repository stay clean under the same guards.
var (
	publicIPv4  = "8.8." + "4.4"
	privateIPv4 = "10.0." + "0.1"
	linkLocalV6 = "fe80" + "::1"
	globalMAC   = "00:1a:2b" + ":3c:4d:5e"
	zeroMAC     = "00:00:00" + ":00:00:00"
	realDomain  = "globex" + ".com"
	eAcute      = string([]byte{0xc3, 0xa9})
)

// writeTree creates files (slash-separated path to content) in a new
// temporary directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runGuard runs the command with args and returns its exit status and
// its stderr output.
func runGuard(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := run(args, &buf)
	return code, buf.String()
}

// goFile returns a Go source file with the license line and a package
// clause before body.
func goFile(body string) string {
	return spdxGo + "\n\npackage p\n\n" + body + "\n"
}

func TestRun_UsageErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name string
		args []string
		want int
		out  string
	}{
		{"no arguments", nil, exitError, "usage:"},
		{"help", []string{"help"}, exitClean, "guards:"},
		{"dash help", []string{"-h"}, exitClean, "realdata"},
		{"unknown guard", []string{"bogus"}, exitError, `unknown guard "bogus"`},
		{"flag help", []string{"spdx", "-h"}, exitClean, "-root"},
		{"bad flag", []string{"spdx", "-nope"}, exitError, "-nope"},
		{"missing flag value", []string{"spdx", "-root"}, exitError, "root"},
		{"extra argument", []string{"spdx", "-root", ".", "extra"}, exitError, `unexpected argument "extra"`},
		{"missing root", []string{"spdx", "-root", missing}, exitError, "error: spdx:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out := runGuard(t, tt.args...)
			if code != tt.want {
				t.Errorf("exit = %d, want %d; output:\n%s", code, tt.want, out)
			}
			if !strings.Contains(out, tt.out) {
				t.Errorf("output does not contain %q:\n%s", tt.out, out)
			}
		})
	}
}

func TestRun_PrintsSortedFindings(t *testing.T) {
	root := writeTree(t, map[string]string{
		"b.go":     "package b\n",
		"a/z.sh":   "echo hi\n",
		"a/ok.go":  goFile(""),
		"notes.md": "# Notes\n",
	})
	code, out := runGuard(t, "spdx", "-root", root)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d; output:\n%s", code, exitFindings, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	prefixes := []string{"a/z.sh:1: ", "a/z.sh:2: ", "b.go:1: ", "error: spdx guard: 3 finding(s)"}
	if len(lines) != len(prefixes) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(prefixes), out)
	}
	for i, p := range prefixes {
		if !strings.HasPrefix(lines[i], p) {
			t.Errorf("line %d = %q, want prefix %q", i+1, lines[i], p)
		}
	}
}

func TestRun_CleanTreeIsSilent(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": goFile("")})
	code, out := runGuard(t, "spdx", "-root", root)
	if code != exitClean || out != "" {
		t.Errorf("exit = %d, output %q; want 0 and no output", code, out)
	}
}

// TestRepositoryIsClean runs every guard against this repository, so
// that "go test ./..." fails as soon as a file breaks a rule.
func TestRepositoryIsClean(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, g := range guards {
		t.Run(g.name, func(t *testing.T) {
			if code, out := runGuard(t, g.name, "-root", root); code != exitClean {
				t.Errorf("guard %s: exit = %d, want 0:\n%s", g.name, code, out)
			}
		})
	}
}
