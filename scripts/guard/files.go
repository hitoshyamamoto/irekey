// SPDX-License-Identifier: MIT

package main

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

// binaryProbe is how much of a file is searched for a NUL byte to decide
// that it is binary and not scanned (the same heuristic git uses).
const binaryProbe = 8 << 10

// gitTimeout bounds every git invocation.
const gitTimeout = 30 * time.Second

// textFile is one scanned text file.
type textFile struct {
	path string // slash-separated, relative to the root
	data []byte
}

// loadTextFiles reads the text files under root whose relative path
// satisfies keep. Binary files, symbolic links and files listed by git but
// deleted from the work tree are skipped.
func loadTextFiles(root string, keep func(path string) bool) ([]textFile, error) {
	paths, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	var files []textFile
	for _, p := range paths {
		if !keep(p) {
			continue
		}
		data, ok, err := readText(r, p)
		if err != nil {
			return nil, err
		}
		if ok {
			files = append(files, textFile{path: p, data: data})
		}
	}
	return files, nil
}

// readText reads one listed file. ok is false when the file is not a
// regular text file and must be skipped.
func readText(r *os.Root, p string) (data []byte, ok bool, err error) {
	name := filepath.FromSlash(p)
	info, err := r.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		// Listed by git (it is in the index) but deleted from the work tree.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	data, err = r.ReadFile(name)
	if err != nil {
		return nil, false, err
	}
	if isBinary(data) {
		return nil, false, nil
	}
	return data, true, nil
}

// readOptional reads a file relative to root, returning nil without an
// error when it does not exist.
func readOptional(root, rel string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	data, err := r.ReadFile(filepath.FromSlash(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), binaryProbe)], 0) >= 0
}

// listFiles returns the slash-separated paths, relative to root, of the
// files to scan. Inside a git work tree it asks git, so that ignored files
// (build output, local tooling) are never scanned; elsewhere, such as in a
// fixture tree, it walks the directory.
func listFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	if insideWorkTree(ctx, root) {
		return gitFiles(ctx, root)
	}
	return walkFiles(root)
}

func insideWorkTree(ctx context.Context, root string) bool {
	//nolint:gosec // G204: the program is the fixed "git"; root is the directory the caller asked to scan.
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		// Not a repository, or git is not installed.
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// gitFiles lists tracked files plus untracked files that are not ignored,
// so that new files are checked before they are committed.
func gitFiles(ctx context.Context, root string) ([]string, error) {
	//nolint:gosec // G204: the program is the fixed "git"; root is the directory the caller asked to scan.
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

// splitLines splits text into lines, numbered from 1 by their index + 1.
func splitLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}

// inTestdata reports whether a slash-separated path is inside a testdata
// directory, which the go tool never builds.
func inTestdata(p string) bool {
	return strings.Contains("/"+path.Dir(p)+"/", "/testdata/")
}
