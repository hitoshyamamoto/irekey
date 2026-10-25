// SPDX-License-Identifier: MIT

package gen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitTimeout bounds every git invocation.
const gitTimeout = 30 * time.Second

// listFiles returns the slash-separated paths, relative to root, of the
// files to check. Inside a git work tree it asks git for the tracked files
// and the untracked ones that are not ignored, so that build output is never
// read and new files are checked before they are committed; elsewhere, such
// as in a fixture tree, it walks the directory. This is how the guards in
// scripts/guard list files too.
func listFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	if insideWorkTree(ctx, root) {
		return gitFiles(ctx, root)
	}
	return walkFiles(root)
}

func insideWorkTree(ctx context.Context, root string) bool {
	//nolint:gosec // G204: the program is the fixed "git"; root is the directory the caller asked to check.
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		// Not a repository, or git is not installed.
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func gitFiles(ctx context.Context, root string) ([]string, error) {
	//nolint:gosec // G204: the program is the fixed "git"; root is the directory the caller asked to check.
	out, err := exec.CommandContext(ctx, "git", "-C", root,
		"ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git ls-files: %w: %s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	// Unmerged index entries appear once per stage.
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

func walkFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			// A directory in a clone, a file in a linked worktree.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

// readRegular reads a listed file. ok is false when the file is not a
// regular file, or is listed by git but deleted from the work tree.
func readRegular(rt *os.Root, p string) (data []byte, ok bool, err error) {
	name := filepath.FromSlash(p)
	info, err := rt.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	data, err = rt.ReadFile(name)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// inTestdata reports whether a slash-separated path is inside a testdata
// directory, which the go tool never builds.
func inTestdata(p string) bool {
	return strings.Contains("/"+path.Dir(p)+"/", "/testdata/")
}
