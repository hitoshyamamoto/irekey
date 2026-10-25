// SPDX-License-Identifier: MIT

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testGoMod = `module github.com/hitoshyamamoto/irekey

go 1.27

toolchain go1.27.3

require golang.org/x/crypto v0.40.0 // SSH client

require (
	// A comment line inside a block.
	"golang.org/x/sys" v0.35.0 // indirect
	pgregory.net/rapid v1.2.0
)

replace (
	golang.org/x/sys => golang.org/x/sys v0.34.0
	example.com/sim => ./lab/sim
)
`

func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not installed")
	}
}

func TestReadGoMod(t *testing.T) {
	requireGo(t)
	root := writeTree(t, map[string]string{"go.mod": testGoMod})
	mod, err := readGoMod(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var req []string
	for _, r := range mod.Require {
		req = append(req, r.Path+"@"+r.Version)
	}
	if got, want := strings.Join(req, " "), "golang.org/x/crypto@v0.40.0 golang.org/x/sys@v0.35.0 pgregory.net/rapid@v1.2.0"; got != want {
		t.Errorf("requires = %q, want %q", got, want)
	}
	if len(mod.Replace) != 2 || mod.Replace[0].New.Version != "v0.34.0" || mod.Replace[1].New.Path != "./lab/sim" {
		t.Errorf("replaces = %+v", mod.Replace)
	}

	bad := writeTree(t, map[string]string{"go.mod": "module\nrequire x\n"})
	if _, err := readGoMod(filepath.Join(bad, "go.mod")); err == nil || !strings.Contains(err.Error(), "go mod edit -json") {
		t.Errorf("malformed go.mod: err = %v", err)
	}
}

func TestGoModFindings(t *testing.T) {
	root := filepath.FromSlash("/repo")
	all := map[string]bool{"golang.org/x/crypto": true, "golang.org/x/sys": true, "pgregory.net/rapid": true}
	req := func(paths ...string) []modVersion {
		var out []modVersion
		for _, p := range paths {
			out = append(out, modVersion{Path: p, Version: "v1.0.0"})
		}
		return out
	}
	type replace = struct{ Old, New modVersion }
	repl := func(newPath, newVersion string) goModFile {
		return goModFile{Replace: []replace{{Old: modVersion{Path: "example.com/x"}, New: modVersion{Path: newPath, Version: newVersion}}}}
	}
	tests := []struct {
		name    string
		mod     goModFile
		allowed map[string]bool
		want    []string // substrings, one per finding
	}{
		{"nothing required", goModFile{}, nil, nil},
		{"all listed", goModFile{Require: req("golang.org/x/crypto", "golang.org/x/sys", "pgregory.net/rapid")}, all, nil},
		{"unlisted direct and indirect", goModFile{Require: req("golang.org/x/crypto", "golang.org/x/sys", "pgregory.net/rapid")},
			map[string]bool{"golang.org/x/sys": true},
			[]string{"go.mod:7: module golang.org/x/crypto is not listed", "go.mod:12: module pgregory.net/rapid"}},
		{"unlisted replacement module", repl("example.com/fork", "v1.0.0"), all,
			[]string{"replacement module example.com/fork is not listed"}},
		{"listed replacement module", repl("golang.org/x/sys", "v0.34.0"), all, nil},
		{"replace into lab", repl("./lab/images/sim", ""), nil, []string{"replace points into lab/ (./lab/images/sim)"}},
		{"replace lab itself", repl("./lab", ""), nil, []string{"replace points into lab/"}},
		{"replace into lab through dot-dot", repl("./internal/../lab/sim", ""), nil, []string{"replace points into lab/"}},
		{"replace into lab with an absolute path", repl(filepath.Join(root, "lab", "sim"), ""), nil, []string{"replace points into lab/"}},
		{"replace to another local path", repl("./third_party/x", ""), nil, nil},
		{"replace to a sibling named like lab", repl("./labs/x", ""), nil, nil},
		{"replace to a parent directory", repl("../lab/x", ""), nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goModFindings(root, []byte(testGoMod), tt.mod, tt.allowed)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %d finding(s) %q", got, len(tt.want), tt.want)
			}
			for i, f := range got {
				if !strings.Contains(f.String(), tt.want[i]) {
					t.Errorf("finding %d = %q, want it to contain %q", i, f, tt.want[i])
				}
			}
		})
	}
}

func TestGoModLine(t *testing.T) {
	data := []byte(testGoMod)
	tests := []struct {
		path    string
		replace bool
		want    int
	}{
		{"golang.org/x/crypto", false, 7},
		{"golang.org/x/sys", false, 11},
		{"pgregory.net/rapid", false, 12},
		{"golang.org/x/sys", true, 16},
		{"./lab/sim", true, 17},
		{"example.com/missing", false, 1},
	}
	for _, tt := range tests {
		if got := goModLine(data, tt.path, tt.replace); got != tt.want {
			t.Errorf("goModLine(%q, %v) = %d, want %d", tt.path, tt.replace, got, tt.want)
		}
	}
}

func TestParseModuleAllowlist(t *testing.T) {
	gpl := "GPL" + "-3.0-only"
	tests := []struct {
		name   string
		line   string
		listed bool
		want   string // substring of the finding; "" for none
	}{
		{"single license", "golang.org/x/crypto BSD-3-Clause # SSH client", true, ""},
		{"dual license", "example.com/a MIT OR Apache-2.0 # needed for x", true, ""},
		{"conjunction", "example.com/a MIT AND ISC # needed for x", true, ""},
		{"three terms", "example.com/a MPL-2.0 OR MIT AND BSD-2-Clause # x", true, ""},
		{"copyleft", "example.com/a " + gpl + " # x", true, `license "` + gpl + `" is not allowed`},
		{"copyleft alternative", "example.com/a MIT OR " + gpl + " # x", true, "is not allowed"},
		{"lowercase operator", "example.com/a MIT or ISC # x", true, `expected AND or OR`},
		{"exception clause", "example.com/a Apache-2.0 WITH LLVM-exception # x", true, `found "WITH"`},
		{"parentheses", "example.com/a (MIT OR ISC) # x", true, `"(MIT" is not allowed`},
		{"trailing operator", "example.com/a MIT OR # x", true, "ends with an operator"},
		{"no justification", "example.com/a MIT", true, "needs a justification"},
		{"empty justification", "example.com/a MIT #  ", true, "needs a justification"},
		{"no license", "example.com/a # x", false, "expected <module path> <license expression>"},
		{"comment", "# example.com/a MIT", false, ""},
		{"blank", "   ", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, got := parseModuleAllowlist("list.txt", []byte(tt.line))
			if listed := allowed["example.com/a"] || allowed["golang.org/x/crypto"]; listed != tt.listed {
				t.Errorf("listed = %v, want %v", listed, tt.listed)
			}
			switch {
			case tt.want == "" && len(got) != 0:
				t.Errorf("findings = %v, want none", got)
			case tt.want != "" && (len(got) != 1 || !strings.Contains(got[0].msg, tt.want)):
				t.Errorf("findings = %v, want one containing %q", got, tt.want)
			}
		})
	}
}

func TestParseModuleAllowlist_Duplicate(t *testing.T) {
	data := "example.com/a MIT # x\nexample.com/a MIT # again\n"
	_, got := parseModuleAllowlist("list.txt", []byte(data))
	if len(got) != 1 || got[0].line != 2 || !strings.Contains(got[0].msg, "listed twice") {
		t.Errorf("findings = %v, want one duplicate on line 2", got)
	}
}

func TestCheckGoMod_Tree(t *testing.T) {
	requireGo(t)
	gomod := "module example.com/m\n\ngo 1.27\n\nrequire (\n\texample.com/ok v1.0.0\n\texample.com/new v1.0.0 // indirect\n)\n"
	tests := []struct {
		name  string
		files map[string]string
		want  int
		out   string
	}{
		{"no go.mod", map[string]string{"a.txt": "x"}, exitError, "go.mod not found"},
		{"malformed go.mod", map[string]string{"go.mod": "module\n"}, exitError, "go mod edit -json"},
		{"no requires, no allowlist", map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n"}, exitClean, ""},
		{"all listed", map[string]string{
			"go.mod":           gomod,
			allowedModulesFile: "# header\nexample.com/ok MIT # x\nexample.com/new ISC # y\n",
		}, exitClean, ""},
		{"one unlisted", map[string]string{
			"go.mod":           gomod,
			allowedModulesFile: "example.com/ok MIT # x\n",
		}, exitFindings, "go.mod:7: module example.com/new is not listed"},
		{"require block without a space", map[string]string{
			"go.mod": "module example.com/m\n\ngo 1.27\n\nrequire(\n\texample.com/unlisted v1.0.0\n)\n",
		}, exitFindings, "go.mod:6: module example.com/unlisted is not listed"},
		{"listed module replaced by an unlisted one", map[string]string{
			"go.mod":           "module example.com/m\n\ngo 1.27\n\nrequire example.com/ok v1.0.0\n\nreplace example.com/ok => example.com/fork v1.0.0\n",
			allowedModulesFile: "example.com/ok MIT # x\n",
		}, exitFindings, "go.mod:7: replacement module example.com/fork is not listed"},
		{"replace into lab", map[string]string{
			"go.mod": "module example.com/m\n\ngo 1.27\n\nreplace example.com/sim => ./lab/sim\n",
		}, exitFindings, "go.mod:5: replace points into lab/"},
		{"disallowed license", map[string]string{
			"go.mod":           gomod,
			allowedModulesFile: "example.com/ok MIT # x\nexample.com/new " + "AGPL" + "-3.0-only # y\n",
		}, exitFindings, allowedModulesFile + ":2: example.com/new: license"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeTree(t, tt.files)
			code, out := runGuard(t, "gomod", "-root", root)
			if code != tt.want || !strings.Contains(out, tt.out) {
				t.Errorf("exit = %d, want %d; output does not contain %q:\n%s", code, tt.want, tt.out, out)
			}
		})
	}
}
