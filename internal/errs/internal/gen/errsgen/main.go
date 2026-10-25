// SPDX-License-Identifier: MIT

// Command errsgen generates the error-code constants of package errs and
// docs/errors.md from the registry, internal/errs/codes.yaml, and checks
// how a repository uses the codes.
//
// Usage:
//
//	errsgen -in codes.yaml [-go FILE] [-md FILE]
//	errsgen -check DIR
//
// The first form validates the registry and writes the files that are
// asked for; go generate ./internal/errs runs it. The second checks the
// repository rooted at DIR (see the Check function of the generator
// package); scripts/check-errors.sh runs it.
//
// Exit status: 0 when the files were written or the repository is clean; 1
// when the registry is invalid or the check has findings, printed as
// "path:line: message" on stderr; 2 on a usage or internal error.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/hitoshyamamoto/irekey/internal/errs/internal/gen"
)

const (
	exitClean    = 0
	exitFindings = 1
	exitError    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run executes errsgen and returns the process exit status.
func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("errsgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	in := flags.String("in", "", "registry to read, such as codes.yaml")
	goOut := flags.String("go", "", "Go file to write")
	mdOut := flags.String("md", "", "Markdown documentation to write")
	root := flags.String("check", "", "check the repository rooted at `DIR` instead of generating")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitClean
		}
		return exitError
	}
	switch {
	case flags.NArg() > 0:
		printf(stderr, "error: unexpected argument %q\n", flags.Arg(0))
		return exitError
	case *root != "" && (*in != "" || *goOut != "" || *mdOut != ""):
		printf(stderr, "error: -check cannot be combined with -in, -go or -md\n")
		return exitError
	case *root != "":
		return check(*root, stderr)
	case *in == "":
		printf(stderr, "error: -in or -check is required\n")
		flags.Usage()
		return exitError
	}
	return generate(*in, *goOut, *mdOut, stderr)
}

func check(root string, stderr io.Writer) int {
	findings, err := gen.Check(root)
	if err != nil {
		printf(stderr, "error: errors: %v\n", err)
		return exitError
	}
	if len(findings) == 0 {
		return exitClean
	}
	for _, f := range findings {
		printf(stderr, "%s\n", f)
	}
	// The same summary line as the guards in scripts/guard, which
	// scripts/test-guards.sh looks for.
	printf(stderr, "error: errors guard: %d finding(s)\n", len(findings))
	return exitFindings
}

func generate(in, goOut, mdOut string, stderr io.Writer) int {
	reg, err := gen.Load(in)
	if err != nil {
		printf(stderr, "error: %v\n", err)
		return exitFindings
	}
	if err := gen.Validate(reg); err != nil {
		var ve *gen.ValidationError
		if errors.As(err, &ve) {
			for _, p := range ve.Problems {
				printf(stderr, "%s:%d: %s\n", in, max(p.Line, 1), p.Msg)
			}
		}
		printf(stderr, "error: %s is invalid; nothing was written\n", in)
		return exitFindings
	}
	if goOut != "" {
		src, err := gen.RenderGo(reg)
		if err == nil {
			err = writeIfChanged(goOut, src)
		}
		if err != nil {
			printf(stderr, "error: %v\n", err)
			return exitError
		}
	}
	if mdOut != "" {
		if err := writeIfChanged(mdOut, gen.RenderMarkdown(reg)); err != nil {
			printf(stderr, "error: %v\n", err)
			return exitError
		}
	}
	return exitClean
}

// writeIfChanged writes a file unless it already has the content, so that
// running the generator again changes nothing.
func writeIfChanged(name string, data []byte) error {
	name = filepath.Clean(name)
	old, err := os.ReadFile(name)
	switch {
	case err == nil && bytes.Equal(old, data):
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	//nolint:gosec // G306: generated source and documentation are committed files that everyone may read.
	return os.WriteFile(name, data, 0o644)
}

// printf writes a diagnostic. A failed write to stderr cannot be reported
// anywhere, so its error is dropped.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
