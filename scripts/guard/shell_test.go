// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// shellImports is the import block of the shell test sources.
const shellImports = `import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	osexec "os/exec"
	"strings"
)

`

func TestShellFindings(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		// Not reported.
		{"quoted argument vector", `func f() { sess.Run(quote.Join(argv)) }`, 0},
		{"quoting function in a variable", `func f() { cmd := ssh.Quote(argv); sess.Run(cmd) }`, 0},
		{"literal command", `func f() { sess.Run("uptime") }`, 0},
		{"literal with a context", `func f(ctx context.Context) { t.Run(ctx, "uptime") }`, 0},
		{"parameter passed through", `func f(cmd string) { sess.Output(cmd) }`, 0},
		{"result of another call", `func f() { cmd := build(argv); sess.Run(cmd) }`, 0},
		{"exec.Cmd.Run without arguments", `func f() { cmd.Run() }`, 0},
		{"other method", `func f() { sess.Send(fmt.Sprintf("x %s", y)) }`, 0},
		{"other formatting call into a variable", `func f() { msg := fmt.Errorf("x %s", y); sess.Run(msg) }`, 0},
		{"plain exec", `func f(dir string) { exec.Command("git", "-C", dir, "status") }`, 0},
		{"shell with literal script", `func f() { exec.Command("sh", "-c", "echo hi") }`, 0},
		{"shell with a literal in a variable", `func f() { s := "echo hi"; exec.Command("sh", "-c", s) }`, 0},
		{"shell with a package constant", `const probe = "uname -a"

func f() { exec.Command("sh", "-c", probe) }`, 0},
		{"shell with a quoted script", `func f() { exec.Command("bash", "-c", quote(argv)) }`, 0},
		{"shell without -c", `func f(script string) { exec.Command("sh", script) }`, 0},
		{"shell with -c last", `func f() { exec.Command("sh", "-c") }`, 0},
		{"long option that contains c", `func f(rc string) { exec.Command("bash", "--rcfile", rc, "x.sh") }`, 0},
		{"shell name from a variable", `func f(shell, script string) { exec.Command(shell, "-c", script) }`, 0},
		{"Command of another package", `func f(s string) { other.Command("sh", "-c", s) }`, 0},
		// Reported.
		{"Sprintf into Run", `func f(dir string) { sess.Run(fmt.Sprintf("ls %s", dir)) }`, 1},
		{"other formatting function", `func f(dir string) { sess.Run(fmt.Sprint("ls ", dir)) }`, 1},
		{"Sprintln into Run", `func f(dir string) { sess.Run(fmt.Sprintln("ls", dir)) }`, 1},
		{"strings.Join into Run", `func f(argv []string) { sess.Run(strings.Join(argv, " ")) }`, 1},
		{"concatenation into Output", `func f(path string) { sess.Output("cat " + path) }`, 1},
		{"concatenation without literals", `func f(a, b string) { sess.Output(a + b) }`, 1},
		{"parenthesized concatenation", `func f(dir string) { sess.CombinedOutput(("ls " + dir)) }`, 1},
		{"second argument", `func f(ctx context.Context, dir string) { t.Run(ctx, fmt.Sprintf("ls %s", dir)) }`, 1},
		{"command variable", `func f(dir string) {
	cmd := fmt.Sprintf("rm -rf %s", dir)
	sess.Run(cmd)
}`, 1},
		{"variable assigned later", `func f(dir string) { var cmd string; cmd = "ls " + dir; sess.Start(cmd) }`, 1},
		{"variable declared with a value", `func f(dir string) { var cmd = "ls " + dir; sess.Start(cmd) }`, 1},
		{"variable extended", `func f(dir string) { cmd := "ls"; cmd += " " + dir; sess.Run(cmd) }`, 1},
		{"variable through another variable", `func f(dir string) { a := "ls " + dir; b := a; sess.Run(b) }`, 1},
		{"variables assigned each other", `func f(dir string) { a := b; b := a; sess.Run(a) }`, 0},
		{"variable used in a closure", `func f(dir string) { cmd := "ls " + dir; go func() { sess.Run(cmd) }() }`, 1},
		{"strings.Builder", `func f(dir string) { var sb strings.Builder; sb.WriteString("ls " + dir); sess.Run(sb.String()) }`, 1},
		{"bytes.Buffer in a variable", `func f() { var b bytes.Buffer; cmd := b.String(); sess.Run(cmd) }`, 1},
		{"sh -c with computed script", `func f(script string) { exec.Command("sh", "-c", script) }`, 1},
		{"bash -lc", `func f(x string) { exec.Command("bash", "-lc", x) }`, 1},
		{"sh -ec", `func f(x string) { exec.Command("sh", "-ec", x) }`, 1},
		{"shell by full path", `func f(x string) { exec.Command("/usr/bin/bash", "-c", x) }`, 1},
		{"other shells", `func f(x string) {
	exec.Command("dash", "-c", x)
	exec.Command("zsh", "-c", x)
	exec.Command("ksh", "-c", x)
	exec.Command("ash", "-c", x)
}`, 4},
		{"aliased os/exec", `func f(x string) { osexec.Command("sh", "-c", x) }`, 1},
		{"CommandContext", `func f(ctx context.Context, s string) { exec.CommandContext(ctx, "/bin/bash", "-c", s) }`, 1},
		{"options before -c", `func f(s string) { exec.Command("bash", "-o", "pipefail", "-c", s) }`, 1},
		{"through env", "func f(s string) { exec.Command(\"/usr/bin/env\", `sh`, \"-c\", s) }", 1},
		{"script assigned a computed value", `func f(dir string) { s := "ls " + dir; exec.Command("sh", "-c", s) }`, 1},
		{"script extended", `func f(dir string) { s := "ls"; s += dir; exec.Command("sh", "-c", s) }`, 1},
		{"script from a multi-value call", `func f() { s, _ := load(); exec.Command("sh", "-c", s) }`, 1},
		{"two problems", `func f(x string) { sess.Run(fmt.Sprintf("%s", x)); exec.Command("sh", "-c", x) }`, 2},
		{"package-level closure", `var run = func(dir string) { sess.Run("ls " + dir) }`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := parseGo("x.go", []byte(spdxGo+"\n\npackage p\n\n"+shellImports+tt.body+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			got := shellFindings(src)
			if len(got) != tt.want {
				t.Fatalf("got %d findings %v, want %d", len(got), got, tt.want)
			}
			for _, f := range got {
				if !strings.Contains(f.msg, shellAdvice) {
					t.Errorf("message %q does not give the advice", f.msg)
				}
			}
		})
	}
}

func TestShellFindings_ResolvesImports(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"exec not imported", `package p
func f(x string) { exec.Command("sh", "-c", x) }`, 0},
		{"fmt imported under another name", `package p
import f "fmt"
func g(x string) { sess.Run(f.Sprintf("ls %s", x)) }`, 1},
		{"fmt name without the import", `package p
func g(x string) { sess.Run(fmt.Sprintf("ls %s", x)) }`, 0},
		{"dot import", `package p
import . "os/exec"
func f(x string) { Command("sh", "-c", x) }`, 1},
		{"blank import", `package p
import _ "os/exec"
func f(x string) { exec.Command("sh", "-c", x) }`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := parseGo("x.go", []byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if got := shellFindings(src); len(got) != tt.want {
				t.Errorf("got %v, want %d finding(s)", got, tt.want)
			}
		})
	}
}

func TestCheckShell_Tree(t *testing.T) {
	bad := goFile(`func f(dir string) { sess.Run("ls " + dir) }`)
	root := writeTree(t, map[string]string{
		"internal/transport/ssh/quote.go":      bad,
		"internal/transport/ssh/quote_test.go": bad,
		"internal/transport/ssh/session.go":    bad,
		"lab/divergence/main.go":               bad,
		"internal/broken.go":                   "package",
	})
	code, out := runGuard(t, "shell", "-root", root)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, exitFindings, out)
	}
	for _, want := range []string{
		"internal/transport/ssh/session.go:5: Run called with a command assembled from strings",
		"lab/divergence/main.go:5: ",
		"internal/broken.go:1: cannot parse Go file",
		"error: shell guard: 3 finding(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "quote.go") || strings.Contains(out, "quote_test.go") {
		t.Errorf("the quoting file and test files must be skipped:\n%s", out)
	}
}
