// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func paths(files []textFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.path)
	}
	return out
}

func TestLoadTextFiles_WalkSkipsGitBinaryAndLinks(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.txt":            "text\n",
		"sub/b.md":         "text\n",
		"sub/.git":         "gitdir: elsewhere\n",
		".git/config":      publicIPv4 + "\n",
		"image.bin":        "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
		"late-nul.txt":     string(make([]byte, binaryProbe)) + "text",
		"sub/deep/c.go":    goFile(""),
		"sub/deep/.keepme": "",
	})
	if err := os.Symlink("a.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	files, err := loadTextFiles(root, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "sub/b.md", "sub/deep/.keepme", "sub/deep/c.go"}
	if got := paths(files); !slices.Equal(got, want) {
		t.Errorf("files = %q, want %q", got, want)
	}
}

func TestLoadTextFiles_Filter(t *testing.T) {
	root := writeTree(t, map[string]string{"a.go": "x", "b.sh": "y"})
	files, err := loadTextFiles(root, func(p string) bool { return p == "b.sh" })
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(files); !slices.Equal(got, []string{"b.sh"}) {
		t.Errorf("files = %q, want [b.sh]", got)
	}
}

func TestLoadTextFiles_GitSkipsIgnoredAndDeletedFiles(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root := writeTree(t, map[string]string{
		".gitignore":    "/build/\n*.log\n",
		"kept.txt":      "kept\n",
		"build/out.txt": publicIPv4 + "\n",
		"debug.log":     publicIPv4 + "\n",
		"gone.txt":      "soon deleted\n",
		"dir/new.md":    "untracked, not ignored\n",
	})
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), git, append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %q: %v\n%s", args, err, out)
		}
	}
	gitRun("init", "-q")
	gitRun("add", "gone.txt", "kept.txt")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	listed, err := listFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".gitignore", "dir/new.md", "gone.txt", "kept.txt"}; !slices.Equal(listed, want) {
		t.Errorf("listFiles = %q, want %q", listed, want)
	}
	files, err := loadTextFiles(root, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := paths(files), []string{".gitignore", "dir/new.md", "kept.txt"}; !slices.Equal(got, want) {
		t.Errorf("loadTextFiles = %q, want %q", got, want)
	}
	if code, out := runGuard(t, "realdata", "-root", root); code != exitClean {
		t.Errorf("realdata scanned an ignored file: exit %d\n%s", code, out)
	}

	// A subdirectory of a work tree lists only its own files, relative to it.
	sub, err := listFiles(filepath.Join(root, "dir"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sub, []string{"new.md"}) {
		t.Errorf("listFiles(dir) = %q, want [new.md]", sub)
	}
}

func TestReadOptional(t *testing.T) {
	root := writeTree(t, map[string]string{"x/a.txt": "hello"})
	data, err := readOptional(root, "x/a.txt")
	if err != nil || string(data) != "hello" {
		t.Errorf("readOptional(existing) = %q, %v", data, err)
	}
	data, err = readOptional(root, "x/missing.txt")
	if err != nil || data != nil {
		t.Errorf("readOptional(missing) = %q, %v; want nil, nil", data, err)
	}
	if _, err := readOptional(filepath.Join(root, "nope"), "a.txt"); err == nil {
		t.Error("readOptional with a missing root: want an error")
	}
	if _, err := readOptional(root, "../outside.txt"); err == nil {
		t.Error("readOptional outside the root: want an error")
	}
}

func TestInTestdata(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"testdata/a.txt", true},
		{"internal/x/testdata/a.txt", true},
		{"internal/x/testdata/deep/a.txt", true},
		{"testdata.go", false},
		{"internal/mytestdata/a.txt", false},
		{"internal/x/a.txt", false},
	}
	for _, tt := range tests {
		if got := inTestdata(tt.path); got != tt.want {
			t.Errorf("inTestdata(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
