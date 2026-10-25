// SPDX-License-Identifier: MIT

package gen

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// tree is a fixture repository: file contents by slash-separated path.
type tree map[string]string

// goodTree returns a fixture repository that passes the check: the fixture
// registry with up-to-date generated files, non-test code and a test that
// produce the active code NET-004, and files that may spell codes out.
func goodTree(t *testing.T) tree {
	t.Helper()
	reg := parseFixture(t)
	src, err := RenderGo(reg)
	if err != nil {
		t.Fatal(err)
	}
	return tree{
		RegistryFile: fixtureYAML,
		GoFile:       string(src),
		DocsFile:     string(RenderMarkdown(reg)),
		"internal/app/activate.go": "package app\n\n" +
			"import \"github.com/hitoshyamamoto/irekey/internal/errs\"\n\n" +
			"func lost(err error) error { return errs.New(errs.NET004, \"activate\", err) }\n",
		"internal/app/activate_test.go": "package app\n\n" +
			"import (\n\t\"testing\"\n\n\t\"github.com/hitoshyamamoto/irekey/internal/errs\"\n)\n\n" +
			"func TestLost(t *testing.T) {\n\tif errs.CodeOf(lost(nil)) != errs.NET004 {\n\t\tt.Fatal(\"NET-004 expected\")\n\t}\n}\n",
		// Package errs and its generator define the codes.
		"internal/errs/extra.go": "package errs\n\nvar example = \"USR-001\"\n",
		// The go tool never builds testdata.
		"internal/app/testdata/sample.go": "package sample\n\nvar s = \"USR-001\"\n",
		// The specification defines codes, and only Markdown is checked,
		// outside test data. FOO-001 has no registered prefix.
		"docs/spec/masterprompt.md":  "USR-999 is a code of the future.\n",
		"docs/notes.txt":             "USR-999 in a text file is not checked.\n",
		"internal/app/testdata/x.md": "USR-999 in test data is not checked.\n",
		"README.md":                  "A lost connection is NET-004. FOO-001 is not an irekey code.\n",
		"docs/guide.md":              "A lost connection is NET-004. CWE-020 is not an irekey code.\n",
	}
}

func writeTree(t *testing.T, files tree) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(files[p]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func check(t *testing.T, root string) []string {
	t.Helper()
	findings, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.String()
	}
	return out
}

const errsImportLine = "import \"github.com/hitoshyamamoto/irekey/internal/errs\"\n\n"

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		change func(files tree)
		want   []string // prefixes of the findings, in order
	}{
		{"clean", func(tree) {}, nil},

		{"string literal", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar code = \"USR-001\"\n"
		}, []string{"internal/app/literal.go:3: string literal contains the error code USR-001: use the generated constant errs.USR001"}},
		{"constant concatenation", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar code = errs.Code(\"USR\" + (\"-\" + \"001\"))\n"
		}, []string{"internal/app/literal.go:3: constant string expression contains the error code USR-001: use the generated constant errs.USR001"}},
		{"concatenation of an unregistered code", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar code = \"USR-\" + \"999\"\n"
		}, []string{"internal/app/literal.go:3: constant string expression contains USR-999, which is not a registered error code"}},
		{"concatenation with a variable", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar code = \"USR-001 \" + suffix\n"
		}, []string{"internal/app/literal.go:3: string literal contains the error code USR-001"}},
		{"concatenation without a code", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar a, b = \"x\" + \"y\", 1 + 2\n"
		}, nil},
		{"concatenation in a test", func(f tree) {
			f["internal/app/message_test.go"] = "package app\n\nvar want = \"NET\" + \"-004\"\n"
		}, nil},
		{"raw string literal", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar msg = `failed with RO-002 today`\n"
		}, []string{"internal/app/literal.go:3: string literal contains the error code RO-002"}},
		{"literal of an unregistered code", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar code = \"USR-999\"\n"
		}, []string{"internal/app/literal.go:3: string literal contains USR-999, which is not a registered error code"}},
		{"literal with an unregistered prefix", func(f tree) {
			f["internal/app/literal.go"] = "package app\n\nvar ref = \"CWE-020 and LOC-001\"\n"
		}, nil},
		{"literal in a test", func(f tree) {
			f["internal/app/message_test.go"] = "package app\n\nvar want = \"NET-004: activate: lost\"\n"
		}, nil},

		{"used but reserved", func(f tree) {
			f["internal/app/adopt.go"] = "package app\n\n" + errsImportLine + "var unknown = errs.USR001\n"
		}, []string{"internal/app/adopt.go:5: USR-001 is used here but reserved: mark it active"}},
		{"reserved and used only by a test", func(f tree) {
			f["internal/app/adopt_test.go"] = "package app\n\n" + errsImportLine + "var unknown = errs.USR001\n"
		}, nil},
		{"used but not registered", func(f tree) {
			f["internal/app/adopt.go"] = "package app\n\n" + errsImportLine + "var unknown = errs.USR999\n"
		}, []string{"internal/app/adopt.go:5: errs.USR999 is not a registered error code"}},
		{"active but unused", func(f tree) {
			delete(f, "internal/app/activate.go")
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no non-test code outside internal/errs/ uses errs.NET004: set it back to reserved"}},
		{"active but untested", func(f tree) {
			delete(f, "internal/app/activate_test.go")
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no test outside internal/errs/ refers to errs.NET004: add a test that produces the code and refers to the constant"}},
		{"use inside internal/errs does not count", func(f tree) {
			delete(f, "internal/app/activate.go")
			f["internal/errs/internal/x/x.go"] = "package x\n\n" + errsImportLine + "var c = errs.NET004\n"
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no non-test code"}},
		{"use in testdata does not count", func(f tree) {
			delete(f, "internal/app/activate.go")
			f["internal/app/testdata/x.go"] = "package x\n\n" + errsImportLine + "var c = errs.NET004\n"
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no non-test code"}},

		{"renamed import", func(f tree) {
			f["internal/app/activate.go"] = "package app\n\nimport e \"github.com/hitoshyamamoto/irekey/internal/errs\"\n\nvar c = e.NET004\n"
		}, nil},
		{"blank import", func(f tree) {
			f["internal/app/activate.go"] = "package app\n\nimport _ \"github.com/hitoshyamamoto/irekey/internal/errs\"\n\nvar c = errs.NET004\n"
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no non-test code"}},
		{"two names for the package", func(f tree) {
			f["internal/app/two.go"] = "package app\n\nimport (\n\ta \"github.com/hitoshyamamoto/irekey/internal/errs\"\n" +
				"\tb \"github.com/hitoshyamamoto/irekey/internal/errs\"\n)\n\nvar c, d = a.USR001, b.USR006\n"
		}, []string{
			"internal/app/two.go:8: USR-001 is used here but reserved",
			"internal/app/two.go:8: USR-006 is used here but reserved",
		}},
		{"another package named errs", func(f tree) {
			f["internal/app/activate.go"] = "package app\n\nimport \"example.com/errs\"\n\nvar c = errs.NET004\n"
		}, []string{"internal/errs/codes.yaml:41: NET-004 is active, but no non-test code"}},
		{"selector that is not a code", func(f tree) {
			f["internal/app/kind.go"] = "package app\n\n" + errsImportLine + "var k, s = errs.KindError, errs.StateUnknown\n"
		}, nil},
		{"dot import", func(f tree) {
			f["internal/app/dot.go"] = "package app\n\nimport . \"github.com/hitoshyamamoto/irekey/internal/errs\"\n\nvar c = USR001\n"
		}, []string{"internal/app/dot.go:3: imports github.com/hitoshyamamoto/irekey/internal/errs with a dot"}},
		{"unparsable Go file", func(f tree) {
			f["internal/app/broken.go"] = "package app\n\nfunc {\n"
		}, []string{"internal/app/broken.go:3: cannot parse Go file"}},

		{"root Markdown mentions an unregistered code", func(f tree) {
			f["CHANGELOG.md"] = "# Changelog\n\n- Added BUG-777.\n"
			f["examples/README.md"] = "See RO-999.\n"
		}, []string{
			"CHANGELOG.md:3: mentions BUG-777, which is not a registered error code",
			"examples/README.md:1: mentions RO-999, which is not a registered error code",
		}},
		{"docs mention an unregistered code", func(f tree) {
			f["docs/adr/0099-example.md"] = "# Example\n\nSee USR-999 and NET-004, then BUG-777.\n"
		}, []string{
			"docs/adr/0099-example.md:3: mentions USR-999, which is not a registered error code",
			"docs/adr/0099-example.md:3: mentions BUG-777, which is not a registered error code",
		}},

		{"stale generated Go", func(f tree) {
			f[GoFile] += "// edited by hand\n"
		}, []string{"internal/errs/zz_generated_codes.go:1: out of date with internal/errs/codes.yaml: run go generate ./internal/errs"}},
		{"stale documentation", func(f tree) {
			f[DocsFile] = strings.Replace(f[DocsFile], "unknown device", "unknown thing", 1)
		}, []string{"docs/errors.md:1: out of date with internal/errs/codes.yaml"}},
		{"missing generated files", func(f tree) {
			delete(f, GoFile)
			delete(f, DocsFile)
		}, []string{
			"docs/errors.md:1: missing: run go generate ./internal/errs",
			"internal/errs/zz_generated_codes.go:1: missing: run go generate ./internal/errs",
		}},

		{"invalid registry", func(f tree) {
			f[RegistryFile] = strings.Replace(fixtureYAML, "exit: 12", "exit: 7", 1)
		}, []string{"internal/errs/codes.yaml:41: NET-004: exit 7 (failed after the switch; rolled back successfully) needs device_state previous-release, not unknown"}},
		{"registry without categories", func(f tree) {
			f[RegistryFile] = "categories: []\n"
		}, []string{"internal/errs/codes.yaml:1: the registry lists no categories"}},
		{"undecodable registry", func(f tree) {
			f[RegistryFile] = "categories: []\ncodes: [\n"
		}, []string{"internal/errs/codes.yaml:2: yaml: line 2: did not find expected node content"}},
		{"registry error without a line", func(f tree) {
			f[RegistryFile] = ""
		}, []string{"internal/errs/codes.yaml:1: the registry is empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := goodTree(t)
			tt.change(files)
			got := check(t, writeTree(t, files))
			if len(got) != len(tt.want) {
				t.Fatalf("findings:\n%s\nwant %d starting with:\n%s", strings.Join(got, "\n"), len(tt.want), strings.Join(tt.want, "\n"))
			}
			for i := range got {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("finding %d = %q\nwant it to start with %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCheckInGitWorkTree checks that inside a git work tree the files come
// from git: tracked and untracked files are checked, ignored files are not,
// and files deleted from the work tree or that are not regular files are
// skipped.
func TestCheckInGitWorkTree(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	files := goodTree(t)
	files[".gitignore"] = "ignored/\n"
	files["ignored/literal.go"] = "package ignored\n\nvar c = \"USR-001\"\n"
	files["internal/app/untracked.go"] = "package app\n\nvar c = \"USR-006\"\n"
	files["internal/app/deleted.go"] = "package app\n\nvar c = \"BUG-003\"\n"
	root := writeTree(t, files)
	if err := os.Symlink("activate.go", filepath.Join(root, "internal", "app", "link.go")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "internal/app/deleted.go"},
	} {
		cmd := exec.CommandContext(ctx, git, append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if err := os.Remove(filepath.Join(root, "internal", "app", "deleted.go")); err != nil {
		t.Fatal(err)
	}

	got := check(t, root)
	want := []string{"internal/app/untracked.go:3: string literal contains the error code USR-006"}
	if len(got) != 1 || !strings.HasPrefix(got[0], want[0]) {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCheckErrors(t *testing.T) {
	if _, err := Check(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Check succeeded on a missing root")
	}
	if _, err := Check(t.TempDir()); err == nil {
		t.Error("Check succeeded without a registry")
	}

	files := goodTree(t)
	delete(files, DocsFile)
	root := writeTree(t, files)
	if err := os.Mkdir(filepath.Join(root, filepath.FromSlash(DocsFile)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(root); err == nil {
		t.Error("Check succeeded with a directory in place of docs/errors.md")
	}
}

func TestCheckUnreadableFiles(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("the superuser can read files without permission")
	}
	root := writeTree(t, goodTree(t))
	file := filepath.Join(root, "internal", "app", "activate.go")
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(root); err == nil {
		t.Error("Check succeeded with an unreadable Go file")
	}
	if err := os.Chmod(file, 0o644); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(root, "internal", "app")
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := Check(root); err == nil {
		t.Error("Check succeeded with an unreadable directory")
	}
}

func TestInTestdata(t *testing.T) {
	for p, want := range map[string]bool{
		"testdata/a.go":            true,
		"internal/x/testdata/a.go": true,
		"internal/x/a.go":          false,
		"internal/testdatax/a.go":  false,
		"a.go":                     false,
	} {
		if got := inTestdata(p); got != want {
			t.Errorf("inTestdata(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestWalkFilesSkipsGitMetadata(t *testing.T) {
	root := writeTree(t, tree{
		".git/HEAD":     "ref: refs/heads/main\n",
		"sub/.git":      "gitdir: ../elsewhere\n",
		"sub/a.go":      "package sub\n",
		"docs/guide.md": "guide\n",
	})
	got, err := walkFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/guide.md", "sub/a.go"}; !slices.Equal(got, want) {
		t.Errorf("walkFiles = %q, want %q", got, want)
	}
}
