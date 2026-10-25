// SPDX-License-Identifier: MIT

// Command guard runs the repository guards that keep the public
// repository free of unsafe code patterns and of real-world data.
//
// Usage:
//
//	go run ./scripts/guard <guard> [-root DIR]
//
// The scripts/check-*.sh wrappers call it, one guard each. DIR defaults
// to the current directory. Allowlists are read from DIR/scripts/guard/,
// so a fixture tree can bring its own.
//
// Exit status: 0 when the tree is clean; 1 when there are findings,
// printed as "path:line: message" on stderr with paths relative to DIR;
// 2 on a usage or internal error.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
)

const (
	exitClean    = 0
	exitFindings = 1
	exitError    = 2
)

// finding is one guard violation at a line of a file.
type finding struct {
	path string // slash-separated, relative to the scanned root
	line int
	msg  string
}

func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s", f.path, f.line, f.msg)
}

// guard is one check run against a root directory.
type guard struct {
	name    string
	summary string
	check   func(root string) ([]finding, error)
}

var guards = []guard{
	{"spdx", "Go and shell files carry the SPDX license line", checkSPDX},
	{"hostkey", "no Go code disables SSH host-key verification", checkHostKey},
	{"shell", "no Go code assembles shell commands from strings", checkShell},
	{"realdata", "no real IP or MAC addresses, no unlisted domains", checkRealData},
	{"language", "no non-ASCII letters outside the allowlist", checkLanguage},
	{"gomod", "every Go module is listed with an allowed license", checkGoMod},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run executes one guard and returns the process exit status.
func run(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitError
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		usage(stderr)
		return exitClean
	}
	g, ok := lookup(args[0])
	if !ok {
		printf(stderr, "error: unknown guard %q\n", args[0])
		usage(stderr)
		return exitError
	}

	flags := flag.NewFlagSet(g.name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "directory to scan (allowlists are read from its scripts/guard/)")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitClean
		}
		return exitError
	}
	if flags.NArg() > 0 {
		printf(stderr, "error: unexpected argument %q\n", flags.Arg(0))
		return exitError
	}

	findings, err := g.check(*root)
	if err != nil {
		printf(stderr, "error: %s: %v\n", g.name, err)
		return exitError
	}
	if len(findings) == 0 {
		return exitClean
	}
	sortFindings(findings)
	for _, f := range findings {
		printf(stderr, "%s\n", f)
	}
	printf(stderr, "error: %s guard: %d finding(s)\n", g.name, len(findings))
	return exitFindings
}

func lookup(name string) (guard, bool) {
	for _, g := range guards {
		if g.name == name {
			return g, true
		}
	}
	return guard{}, false
}

func usage(w io.Writer) {
	printf(w, "usage: go run ./scripts/guard <guard> [-root DIR]\n\nguards:\n")
	for _, g := range guards {
		printf(w, "  %-9s %s\n", g.name, g.summary)
	}
}

// printf writes a diagnostic. A failed write to stderr cannot be reported
// anywhere, so its error is dropped.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func sortFindings(fs []finding) {
	slices.SortStableFunc(fs, func(a, b finding) int {
		return cmp.Or(cmp.Compare(a.path, b.path), cmp.Compare(a.line, b.line))
	})
}
