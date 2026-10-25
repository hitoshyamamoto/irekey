// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// allowedModulesFile lists every Go module irekey may depend on, with its
// license, relative to the scanned root.
const allowedModulesFile = "scripts/guard/allowed-modules.txt"

// goTimeout bounds every go command the guard runs.
const goTimeout = 60 * time.Second

// allowedLicenses are the SPDX identifiers a dependency may carry. Any
// other license needs an architecture decision record and a change here.
var allowedLicenses = []string{"MIT", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "ISC", "MPL-2.0"}

// goModFile is the part of the "go mod edit -json" output the guard reads.
type goModFile struct {
	Require []modVersion
	Replace []struct {
		Old modVersion
		New modVersion
	}
}

type modVersion struct {
	Path    string
	Version string
}

// checkGoMod fails when go.mod requires a module (direct or indirect)
// that the allowlist does not list; when a replace directive substitutes
// a module that the allowlist does not list; when an allowlist entry
// names a license outside the allowed set; and when a replace directive
// points into lab/: the lab holds simulators and fixtures, some under
// licenses that must never reach a Go build.
func checkGoMod(root string) ([]finding, error) {
	gomod, err := readOptional(root, "go.mod")
	if err != nil {
		return nil, err
	}
	if gomod == nil {
		return nil, errors.New("go.mod not found")
	}
	list, err := readOptional(root, allowedModulesFile)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	mod, err := readGoMod(filepath.Join(absRoot, "go.mod"))
	if err != nil {
		return nil, err
	}
	allowed, out := parseModuleAllowlist(allowedModulesFile, list)
	return append(out, goModFindings(absRoot, gomod, mod, allowed)...), nil
}

// readGoMod parses go.mod with the go command, which knows every form of
// the file (blocks, quoting, comments) better than a parser written here.
func readGoMod(file string) (goModFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), goTimeout)
	defer cancel()
	//nolint:gosec // G204: the program is the fixed "go"; file is the go.mod of the tree being scanned.
	cmd := exec.CommandContext(ctx, "go", "mod", "edit", "-json", file)
	// Run outside any module and with the local toolchain, so that reading
	// the file depends on neither the current directory nor the network.
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return goModFile{}, fmt.Errorf("go mod edit -json: %w: %s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		return goModFile{}, fmt.Errorf("go mod edit -json: %w", err)
	}
	var mod goModFile
	if err := json.Unmarshal(out, &mod); err != nil {
		return goModFile{}, fmt.Errorf("go mod edit -json: %w", err)
	}
	return mod, nil
}

func goModFindings(absRoot string, data []byte, mod goModFile, allowed map[string]bool) []finding {
	var out []finding
	for _, r := range mod.Require {
		if !allowed[r.Path] {
			out = append(out, finding{"go.mod", goModLine(data, r.Path, false), fmt.Sprintf(
				"module %s is not listed in %s (add it with its license and a justification)",
				r.Path, allowedModulesFile)})
		}
	}
	for _, r := range mod.Replace {
		line := goModLine(data, r.New.Path, true)
		switch {
		case r.New.Version != "" && !allowed[r.New.Path]:
			// A module replacement: the replacement is the dependency.
			out = append(out, finding{"go.mod", line, fmt.Sprintf(
				"replacement module %s is not listed in %s (add it with its license and a justification)",
				r.New.Path, allowedModulesFile)})
		case r.New.Version == "" && insideLab(absRoot, r.New.Path):
			// No version: a directory on disk.
			out = append(out, finding{"go.mod", line, fmt.Sprintf(
				"replace points into lab/ (%s): lab code must never become a Go dependency", r.New.Path)})
		}
	}
	return out
}

// goModLine finds the go.mod line that names modPath: in a require line,
// or after "=>" in a replace line. It returns 1 when no line matches.
func goModLine(data []byte, modPath string, replace bool) int {
	for i, raw := range splitLines(data) {
		text, _, _ := strings.Cut(raw, "//")
		fields := strings.Fields(text)
		arrow := slices.Index(fields, "=>")
		if replace != (arrow >= 0) {
			continue
		}
		for _, f := range fields[arrow+1:] {
			if strings.Trim(f, "\"`") == modPath {
				return i + 1
			}
		}
	}
	return 1
}

func insideLab(absRoot, target string) bool {
	p := target
	if !filepath.IsAbs(p) {
		p = filepath.Join(absRoot, filepath.FromSlash(p))
	}
	rel, err := filepath.Rel(filepath.Join(absRoot, "lab"), filepath.Clean(p))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// parseModuleAllowlist reads lines of the form
// "<module path> <license expression> # <justification>", where the
// expression is SPDX identifiers joined by AND or OR. It returns the
// listed modules and a finding for every malformed or disallowed entry.
func parseModuleAllowlist(p string, data []byte) (map[string]bool, []finding) {
	allowed := map[string]bool{}
	var out []finding
	for i, raw := range splitLines(data) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, comment, found := strings.Cut(line, "#")
		fields := strings.Fields(entry)
		switch {
		case len(fields) < 2:
			out = append(out, finding{p, i + 1, "expected <module path> <license expression> # <justification>"})
			continue
		case !found || strings.TrimSpace(comment) == "":
			out = append(out, finding{p, i + 1, "entry " + fields[0] + " needs a justification comment"})
		case allowed[fields[0]]:
			out = append(out, finding{p, i + 1, "module " + fields[0] + " is listed twice"})
		}
		// A listed module counts as listed even when its entry has a
		// problem, so that the problem is reported once, here.
		allowed[fields[0]] = true
		if msg := checkLicenseExpr(fields[1:]); msg != "" {
			out = append(out, finding{p, i + 1, fields[0] + ": " + msg})
		}
	}
	return allowed, out
}

// checkLicenseExpr validates "ID", "ID OR ID", "ID AND ID OR ID", ...
// It returns an empty string when the expression is valid.
func checkLicenseExpr(tokens []string) string {
	for i, tok := range tokens {
		if i%2 == 1 {
			if tok != "AND" && tok != "OR" {
				return fmt.Sprintf("expected AND or OR in the license expression, found %q", tok)
			}
			continue
		}
		if !slices.Contains(allowedLicenses, tok) {
			return fmt.Sprintf("license %q is not allowed (allowed: %s; anything else needs an "+
				"architecture decision record first)", tok, strings.Join(allowedLicenses, ", "))
		}
	}
	if len(tokens)%2 == 0 {
		return "license expression ends with an operator"
	}
	return ""
}
