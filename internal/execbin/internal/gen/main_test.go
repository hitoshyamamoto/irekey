// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitoshyamamoto/irekey/internal/execbin"
)

// defaults are the platforms of defaultPlatforms.
var defaults = []platform{
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
	{goos: "linux", goarch: "arm", goarm: "7"},
}

// fakeBuilder stands in for the go command: it writes data(p, version) as
// the executor and records every call.
type fakeBuilder struct {
	mu    sync.Mutex
	calls []string
	data  func(p platform, version string) []byte
}

func (f *fakeBuilder) build(_ context.Context, p platform, version, out string) error {
	f.mu.Lock()
	f.calls = append(f.calls, p.String()+" "+version)
	f.mu.Unlock()
	return os.WriteFile(out, f.data(p, version), 0o600)
}

// fakeExecutor is the default content of a fake executor.
func fakeExecutor(p platform, version string) []byte {
	return bytes.Repeat([]byte("fake irekey-exec "+version+" for "+p.String()+"\n"), 500)
}

func newFake() *fakeBuilder { return &fakeBuilder{data: fakeExecutor} }

// sized returns fake executor content where the executor for target has
// size bytes.
func sized(target string, size int) func(platform, string) []byte {
	return func(p platform, version string) []byte {
		if p.String() == target {
			return make([]byte, size)
		}
		return fakeExecutor(p, version)
	}
}

func testOptions(t *testing.T, dist bool) options {
	t.Helper()
	o := options{version: "v1.2.3", out: filepath.Join(t.TempDir(), "bin"), platforms: defaults}
	if dist {
		o.dist = filepath.Join(t.TempDir(), "dist")
	}
	return o
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// listDir returns the names of the files in dir, sorted.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestPlatformNames(t *testing.T) {
	tests := []struct {
		p          platform
		str, asset string
		env        []string
	}{
		{defaults[0], "linux/amd64", "irekey-exec-linux-amd64",
			[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1"}},
		{defaults[1], "linux/arm64", "irekey-exec-linux-arm64",
			[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64", "GOARM64=v8.0"}},
		{defaults[2], "linux/arm/v7", "irekey-exec-linux-armv7",
			[]string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm", "GOARM=7"}},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.str {
			t.Errorf("String() = %q, want %q", got, tt.str)
		}
		if got := tt.p.asset(); got != tt.asset {
			t.Errorf("asset() = %q, want %q", got, tt.asset)
		}
		if got := tt.p.env(); !slices.Equal(got, tt.env) {
			t.Errorf("env() = %q, want %q", got, tt.env)
		}
	}
}

func TestParsePlatforms(t *testing.T) {
	got, err := parsePlatforms(defaultPlatforms)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, defaults) {
		t.Errorf("parsePlatforms(default) = %v, want %v", got, defaults)
	}
	got, err = parsePlatforms(" linux/arm/v7 , linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	want := []platform{{"linux", "arm", "7"}, {"linux", "arm64", ""}}
	if !slices.Equal(got, want) {
		t.Errorf("parsePlatforms = %v, want %v", got, want)
	}

	for _, s := range []string{
		"", ",", "linux", "linux/", "/amd64", "linux/amd64/v1/x",
		"darwin/arm64", "windows/amd64", "linux/386", "linux/riscv64",
		"linux/amd64/v2", "linux/arm64/v8", "linux/arm", "linux/arm/8", "linux/arm/v", "linux/arm/7,",
		// Package execbin accepts only linux/arm/v7 for 32-bit ARM.
		"linux/arm/6", "linux/arm/v5",
		"linux/amd64,linux/amd64", "linux/arm/7,linux/arm/v7",
	} {
		if p, err := parsePlatforms(s); err == nil {
			t.Errorf("parsePlatforms(%q) = %v, want an error", s, p)
		}
	}
}

func TestCheckVersion(t *testing.T) {
	for _, v := range []string{"dev", "ci", "v0.1.0", "v1.2.3-rc.1+build.5", "0.0.0_test"} {
		if err := checkVersion(v); err != nil {
			t.Errorf("checkVersion(%q) = %v", v, err)
		}
	}
	for _, v := range []string{"", "v1 2", `v1"`, "v1'", "v1;x", "v1/x", "v1=x", "v1\n",
		// A non-ASCII letter, built at run time so the source stays ASCII.
		"v" + string(rune(0xe9)), strings.Repeat("v", 129)} {
		if err := checkVersion(v); err == nil {
			t.Errorf("checkVersion(%q) = nil, want an error", v)
		}
	}
}

func TestNewOptions(t *testing.T) {
	o, err := newOptions("v1.0.0", "bin", "dist", defaultPlatforms)
	if err != nil {
		t.Fatal(err)
	}
	if o.version != "v1.0.0" || o.out != "bin" || o.dist != "dist" || !slices.Equal(o.platforms, defaults) {
		t.Errorf("newOptions = %+v", o)
	}
	if _, err := newOptions("v1.0.0", "bin", "", defaultPlatforms); err != nil {
		t.Errorf("without -dist: %v", err)
	}
	// Where -dist is relative to -out is checked by generate (checkDirs).
	for name, args := range map[string][4]string{
		"no version":     {"", "bin", "", defaultPlatforms},
		"bad version":    {"v 1", "bin", "", defaultPlatforms},
		"no out":         {"v1", "", "", defaultPlatforms},
		"bad platforms":  {"v1", "bin", "", "linux/mips"},
		"empty platform": {"v1", "bin", "", ""},
	} {
		if _, err := newOptions(args[0], args[1], args[2], args[3]); err == nil {
			t.Errorf("%s: newOptions(%q) = nil error", name, args)
		}
	}
}

func TestParseArgs(t *testing.T) {
	var stderr bytes.Buffer
	o, err := parseArgs([]string{"-version", "ci", "-out", "bin", "-dist", "dist", "-platforms", "linux/amd64"}, &stderr)
	if err != nil {
		t.Fatalf("parseArgs: %v\n%s", err, stderr.String())
	}
	if o.version != "ci" || o.out != "bin" || o.dist != "dist" || !slices.Equal(o.platforms, defaults[:1]) {
		t.Errorf("parseArgs = %+v", o)
	}

	stderr.Reset()
	if _, err := parseArgs([]string{"-h"}, &stderr); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: error %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(stderr.String(), "Usage: gen") || !strings.Contains(stderr.String(), "-platforms") {
		t.Errorf("-h printed %q, want the usage and the options", stderr.String())
	}

	for name, tt := range map[string]struct {
		args []string
		want string
	}{
		"unknown flag":    {[]string{"-version", "v1", "-out", "bin", "-x"}, "flag provided but not defined"},
		"extra arguments": {[]string{"-version", "v1", "-out", "bin", "more"}, "unexpected arguments: more"},
		"no version":      {[]string{"-out", "bin"}, "-version is required"},
	} {
		stderr.Reset()
		if _, err := parseArgs(tt.args, &stderr); !errors.Is(err, errUsage) {
			t.Errorf("%s: error %v, want errUsage", name, err)
		}
		if !strings.Contains(stderr.String(), tt.want) {
			t.Errorf("%s: printed %q, want %q", name, stderr.String(), tt.want)
		}
	}
}

func TestCompress_IsDeterministic(t *testing.T) {
	data := bytes.Repeat([]byte("irekey-exec\x00\x01\x02"), 10000)
	first, err := compress(data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compress(slices.Clone(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two compressions of the same input differ")
	}

	// The header records nothing about the build (RFC 1952: FLG at byte 3,
	// MTIME at bytes 4-7, OS at byte 9).
	if first[3] != 0 {
		t.Errorf("FLG = %#x, want 0 (no name, comment or extra field)", first[3])
	}
	if !bytes.Equal(first[4:8], []byte{0, 0, 0, 0}) {
		t.Errorf("MTIME = % x, want 0", first[4:8])
	}
	zr, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if !zr.ModTime.IsZero() || zr.Name != "" || zr.Comment != "" || zr.Extra != nil || zr.OS != 255 {
		t.Errorf("header = %+v, want no time, name, comment or extra and OS 255", zr.Header)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("decompressing does not give the input back")
	}
}

func TestGenerate_WritesEmbeddedFilesAndReleaseAssets(t *testing.T) {
	o := testOptions(t, true)
	// The README of the directory, and an unrelated file, are kept.
	if err := os.MkdirAll(o.out, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(o.out, name), []byte("kept\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fake := newFake()
	var stdout, stderr bytes.Buffer
	m, err := generate(t.Context(), o, fake.build, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected warnings: %s", stderr.String())
	}
	wantCalls := []string{"linux/amd64 v1.2.3", "linux/arm64 v1.2.3", "linux/arm/v7 v1.2.3"}
	if !slices.Equal(fake.calls, wantCalls) {
		t.Errorf("builds %q, want %q", fake.calls, wantCalls)
	}

	wantFiles := []string{"README.md", "irekey-exec-linux-amd64.gz", "irekey-exec-linux-arm64.gz",
		"irekey-exec-linux-armv7.gz", "manifest.json", "notes.txt"}
	if got := listDir(t, o.out); !slices.Equal(got, wantFiles) {
		t.Errorf("-out holds %q, want %q", got, wantFiles)
	}
	wantAssets := []string{"irekey-exec-linux-amd64", "irekey-exec-linux-arm64", "irekey-exec-linux-armv7"}
	if got := listDir(t, o.dist); !slices.Equal(got, wantAssets) {
		t.Errorf("-dist holds %q, want %q", got, wantAssets)
	}

	// The manifest: executors sorted by file name, indented, with a final
	// newline, and the digests of the uncompressed executors and of the
	// compressed files.
	var entries []string
	for _, p := range []platform{defaults[0], defaults[1], defaults[2]} {
		raw := fakeExecutor(p, "v1.2.3")
		gz, err := compress(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, filepath.Join(o.out, p.asset()+".gz")); !bytes.Equal(got, gz) {
			t.Errorf("%s.gz differs from the compressed executor", p.asset())
		}
		if got := readFile(t, filepath.Join(o.dist, p.asset())); !bytes.Equal(got, raw) {
			t.Errorf("dist/%s differs from the executor", p.asset())
		}
		entries = append(entries, fmt.Sprintf(`    {
      "platform": %q,
      "file": "%s.gz",
      "sha256": %q,
      "size": %d,
      "gz_sha256": %q,
      "gz_size": %d
    }`, p.String(), p.asset(), digest(raw), len(raw), digest(gz), len(gz)))
	}
	want := "{\n  \"format\": 1,\n  \"version\": \"v1.2.3\",\n  \"executors\": [\n" +
		strings.Join(entries, ",\n") + "\n  ]\n}\n"
	if got := string(readFile(t, filepath.Join(o.out, "manifest.json"))); got != want {
		t.Errorf("manifest.json:\n%s\nwant:\n%s", got, want)
	}

	if runtime.GOOS != "windows" {
		for _, name := range wantAssets {
			info, err := os.Stat(filepath.Join(o.dist, name))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o100 == 0 {
				t.Errorf("dist/%s has mode %v, want it executable", name, info.Mode())
			}
		}
	}

	// What execbin reads back is what generate returned, and it verifies.
	loaded, ok, err := execbin.Load(os.DirFS(o.out))
	if err != nil || !ok {
		t.Fatalf("execbin.Load = ok %v, error %v", ok, err)
	}
	if loaded.Version != m.Version || !slices.Equal(loaded.Executors, m.Executors) {
		t.Errorf("loaded %+v, generate returned %+v", loaded, m)
	}
	if err := loaded.Verify(os.DirFS(o.out)); err != nil {
		t.Error(err)
	}
	for _, e := range m.Executors {
		if !strings.Contains(stdout.String(), e.Platform+" "+e.File+" sha256="+e.SHA256) {
			t.Errorf("stdout %q does not report %s", stdout.String(), e.Platform)
		}
	}
}

func TestGenerate_IsReproducible(t *testing.T) {
	first, second := testOptions(t, false), testOptions(t, false)
	for _, o := range []options{first, second} {
		if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range listDir(t, first.out) {
		if !bytes.Equal(readFile(t, filepath.Join(first.out, name)), readFile(t, filepath.Join(second.out, name))) {
			t.Errorf("%s differs between two runs", name)
		}
	}
}

func TestGenerate_RemovesStaleExecutors(t *testing.T) {
	o := testOptions(t, false)
	if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	o.platforms = defaults[:1]
	o.version = "v1.2.4"
	m, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"irekey-exec-linux-amd64.gz", "manifest.json"}
	if got := listDir(t, o.out); !slices.Equal(got, want) {
		t.Errorf("-out holds %q, want %q", got, want)
	}
	if len(m.Executors) != 1 || m.Version != "v1.2.4" {
		t.Errorf("manifest %+v, want one executor of v1.2.4", m)
	}
}

func TestGenerate_RemovesStaleAssetsFromDist(t *testing.T) {
	o := testOptions(t, true)
	if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	// Files that are not executors stay, among them the command line
	// binaries the CI build job writes next to them.
	kept := []string{"irekey-linux-amd64", "irekey-windows-amd64.exe", "SHA256SUMS", "notes.txt"}
	for _, name := range kept {
		if err := os.WriteFile(filepath.Join(o.dist, name), []byte("kept\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A stray compressed executor is an executor file too.
	if err := os.WriteFile(filepath.Join(o.dist, "irekey-exec-linux-amd64.gz"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	o.platforms = defaults[:1]
	if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(slices.Values(append([]string{"irekey-exec-linux-amd64"}, kept...)))
	if got := listDir(t, o.dist); !slices.Equal(got, want) {
		t.Errorf("-dist holds %q, want %q", got, want)
	}
	for _, name := range kept {
		if string(readFile(t, filepath.Join(o.dist, name))) != "kept\n" {
			t.Errorf("%s was changed", name)
		}
	}
}

func TestGenerate_SizeBudget(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		size     int
		fails    bool
		warns    bool
	}{
		{"at the target", "linux/amd64", sizeTarget, false, false},
		{"above the target", "linux/amd64", sizeTarget + 1, false, true},
		{"at the limit", "linux/amd64", sizeLimit, false, true},
		{"above the limit", "linux/amd64", sizeLimit + 1, true, false},
		// The budget is defined for linux/amd64 only.
		{"other platform above the limit", "linux/arm64", sizeLimit + 1, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, false)
			o.platforms = defaults[:2]
			fake := &fakeBuilder{data: sized(tt.platform, tt.size)}
			var stderr bytes.Buffer
			_, err := generate(t.Context(), o, fake.build, io.Discard, &stderr)
			if tt.fails {
				if err == nil || !strings.Contains(err.Error(), "above the limit of 12582912 bytes") {
					t.Fatalf("error %v, want the size limit", err)
				}
				if _, statErr := os.Stat(o.out); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("-out was written although the build failed (%v)", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			warned := strings.Contains(stderr.String(), "above the target of 8388608 bytes")
			if warned != tt.warns {
				t.Errorf("warning printed: %v, want %v (stderr %q)", warned, tt.warns, stderr.String())
			}
		})
	}
}

func TestGenerate_FailureKeepsEarlierFiles(t *testing.T) {
	// A failed build writes nothing, so the files of an earlier run stay
	// as they were and still match their manifest.
	o := testOptions(t, false)
	if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, filepath.Join(o.out, "manifest.json"))

	failing := func(_ context.Context, p platform, _, _ string) error {
		if p.goarch == "arm" {
			return errors.New("compiler exploded")
		}
		return nil
	}
	_, err := generate(t.Context(), o, failing, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("generate succeeded although a build failed")
	}
	// The amd64 build "succeeded" without writing a file; that is reported
	// before the arm build runs.
	if !strings.Contains(err.Error(), "reading the executor for linux/amd64") {
		t.Errorf("error %q, want the missing amd64 executor", err)
	}

	onlyArm := o
	onlyArm.platforms = defaults[2:]
	_, err = generate(t.Context(), onlyArm, failing, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "building the executor for linux/arm/v7: compiler exploded") {
		t.Errorf("error %v, want the build failure", err)
	}
	if after := readFile(t, filepath.Join(o.out, "manifest.json")); !bytes.Equal(after, before) {
		t.Error("the manifest changed although the build failed")
	}
	m, _, err := execbin.Load(os.DirFS(o.out))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Verify(os.DirFS(o.out)); err != nil {
		t.Error(err)
	}
}

func TestGenerate_FailedWriteLeavesNoManifest(t *testing.T) {
	// A run that fails while writing, after an earlier run succeeded,
	// leaves no manifest: execbin then reports that nothing is embedded,
	// instead of loading a manifest that does not match the files next to
	// it. This needs the old manifest removed first and the new one written
	// last.
	for _, tt := range []struct {
		name     string
		breakRun func(t *testing.T, o options)
	}{
		{"writing a compressed executor fails", func(t *testing.T, o options) {
			// A non-empty directory where a compressed executor goes makes
			// renaming the new file into place fail.
			gz := filepath.Join(o.out, "irekey-exec-linux-arm64.gz")
			if err := os.Remove(gz); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(gz, "x"), 0o750); err != nil {
				t.Fatal(err)
			}
		}},
		{"creating -dist fails, after the compressed executors are written", func(t *testing.T, o options) {
			if err := os.RemoveAll(o.dist); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(o.dist, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, true)
			if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
				t.Fatal(err)
			}
			tt.breakRun(t, o)
			// write is called directly: checkDirs would refuse a -dist that
			// is a file before anything is written.
			executors, err := buildAll(t.Context(), o, newFake().build, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			m := execbin.Manifest{Format: execbin.Format, Version: "v9.9.9"}
			for _, b := range executors {
				m.Executors = append(m.Executors, b.entry)
			}
			if err := write(o, m, executors); err == nil {
				t.Fatal("write succeeded")
			}
			if _, err := os.Stat(filepath.Join(o.out, execbin.ManifestName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a manifest is left after the failed run (%v)", err)
			}
			if _, ok, err := execbin.Load(os.DirFS(o.out)); ok || err != nil {
				t.Errorf("execbin.Load = ok %v, error %v; want nothing embedded", ok, err)
			}
		})
	}
}

func TestGenerate_UnwritableDirectories(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(o *options){
		"out is a file":  func(o *options) { o.out = file },
		"dist is a file": func(o *options) { o.dist = file },
		"out below a file": func(o *options) {
			o.out = filepath.Join(file, "bin")
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := testOptions(t, true)
			change(&o)
			if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err == nil {
				t.Fatal("generate succeeded")
			}
		})
	}
}

func TestWriteFile_ReplacesAndCleansUp(t *testing.T) {
	dir, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	// A temporary file left by an interrupted run does not block writing.
	if err := dir.WriteFile(".a.gz.tmp", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"first", "second"} {
		if err := writeFile(dir, "a.gz", []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := dir.ReadFile("a.gz")
		if err != nil || string(got) != data {
			t.Fatalf("a.gz = %q, %v; want %q", got, err, data)
		}
	}
	// A failed rename (the target is a non-empty directory) is reported and
	// leaves no temporary file.
	if err := dir.MkdirAll("b.gz/x", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dir, "b.gz", []byte("data"), 0o644); err == nil {
		t.Error("writeFile over a non-empty directory succeeded")
	}
	entries := listDir(t, dir.Name())
	if want := []string{"a.gz", "b.gz"}; !slices.Equal(entries, want) {
		t.Errorf("directory holds %q, want %q", entries, want)
	}
}

func TestReadBack_DetectsDamage(t *testing.T) {
	o := testOptions(t, true)
	if _, err := generate(t.Context(), o, newFake().build, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err != nil {
		t.Fatal(err)
	}

	// An executor entry that the manifest does not list, in either
	// directory and of any type, fails the read-back.
	for _, stray := range []string{
		filepath.Join(o.dist, "irekey-exec-darwin-arm64"),
		filepath.Join(o.dist, "irekey-exec-linux-amd64.gz"),
		filepath.Join(o.out, "irekey-exec-linux-riscv64.gz"),
		filepath.Join(o.out, "irekey-exec-linux-amd64"),
	} {
		if err := os.WriteFile(stray, []byte("stray"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := readBack(o)
		if err == nil || !strings.Contains(err.Error(), filepath.Base(stray)+", which the manifest does not list") {
			t.Errorf("with %s, readBack = %v, want it reported as unlisted", stray, err)
		}
		if err := os.Remove(stray); err != nil {
			t.Fatal(err)
		}
	}
	strayDir := filepath.Join(o.dist, "irekey-exec-old")
	if err := os.Mkdir(strayDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err == nil || !strings.Contains(err.Error(), "irekey-exec-old") {
		t.Errorf("with a directory %s, readBack = %v, want it reported", strayDir, err)
	}
	if err := os.Remove(strayDir); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err != nil {
		t.Fatalf("after removing the strays: %v", err)
	}

	asset := filepath.Join(o.dist, "irekey-exec-linux-arm64")
	if err := os.WriteFile(asset, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err == nil || !strings.Contains(err.Error(), "irekey-exec-linux-arm64 has SHA-256") {
		t.Errorf("readBack = %v, want the asset digest mismatch", err)
	}
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err == nil {
		t.Error("readBack succeeded with a missing asset")
	}

	gz := filepath.Join(o.out, "irekey-exec-linux-amd64.gz")
	if err := os.WriteFile(gz, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); !errors.Is(err, execbin.ErrMismatch) {
		t.Errorf("readBack = %v, want ErrMismatch", err)
	}

	if err := os.WriteFile(filepath.Join(o.out, "manifest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); !errors.Is(err, execbin.ErrInvalidManifest) {
		t.Errorf("readBack = %v, want ErrInvalidManifest", err)
	}
	if err := os.Remove(filepath.Join(o.out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := readBack(o); err == nil || !strings.Contains(err.Error(), "has no manifest") {
		t.Errorf("readBack = %v, want the missing manifest", err)
	}

	o.dist = filepath.Join(t.TempDir(), "missing")
	if err := readBack(o); err == nil {
		t.Error("readBack succeeded with a missing -dist directory")
	}
	o.out = filepath.Join(t.TempDir(), "missing")
	if err := readBack(o); err == nil {
		t.Error("readBack succeeded with a missing -out directory")
	}
}

func TestRun(t *testing.T) {
	fake := newFake()
	factory := func(context.Context) (builder, error) { return fake.build, nil }
	out := filepath.Join(t.TempDir(), "bin")

	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"-version", "dev", "-out", out}, &stdout, &stderr, factory); code != exitOK {
		t.Fatalf("exit code %d, want %d; stderr %q", code, exitOK, stderr.String())
	}
	if lines := strings.Count(stdout.String(), "\n"); lines != 3 {
		t.Errorf("stdout has %d lines, want one per executor:\n%s", lines, stdout.String())
	}

	tests := []struct {
		name    string
		args    []string
		factory func(context.Context) (builder, error)
		code    int
		stderr  string
	}{
		{"help", []string{"-h"}, factory, exitOK, "Usage: gen"},
		{"usage", []string{"-out", out}, factory, exitUsage, "-version is required"},
		{"no go command", []string{"-version", "dev", "-out", out},
			func(context.Context) (builder, error) { return nil, errors.New("no go here") }, exitFailure, "gen: no go here"},
		{"build fails", []string{"-version", "dev", "-out", out},
			func(context.Context) (builder, error) {
				return func(context.Context, platform, string, string) error { return errors.New("broken") }, nil
			}, exitFailure, "building the executor for linux/amd64: broken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(t.Context(), tt.args, io.Discard, &stderr, tt.factory); code != tt.code {
				t.Errorf("exit code %d, want %d", code, tt.code)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr %q, want %q", stderr.String(), tt.stderr)
			}
		})
	}
}

func TestBuildCommand(t *testing.T) {
	cmd := buildCommand(t.Context(), "/usr/local/go/bin/go", "/src/irekey", defaults[2], "v1.2.3", "/tmp/out/irekey-exec-linux-armv7", "/tmp/out/go-tmp")
	wantArgs := []string{"/usr/local/go/bin/go", "build", "-trimpath",
		"-ldflags", "-s -w -X github.com/hitoshyamamoto/irekey/internal/version.Version=v1.2.3",
		"-o", "/tmp/out/irekey-exec-linux-armv7", "./cmd/irekey-exec"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Errorf("args %q, want %q", cmd.Args, wantArgs)
	}
	if cmd.Dir != "/src/irekey" {
		t.Errorf("dir %q, want the module root", cmd.Dir)
	}
	// The platform settings come last, so they win over the caller's
	// environment.
	wantTail := []string{"GOENV=off", "GOFLAGS=", "GOEXPERIMENT=", "GOFIPS140=off", "GOWORK=off",
		"CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm", "GOARM=7", "GOTMPDIR=/tmp/out/go-tmp"}
	if tail := cmd.Env[len(cmd.Env)-len(wantTail):]; !slices.Equal(tail, wantTail) {
		t.Errorf("environment ends with %q, want %q", tail, wantTail)
	}
	// A canceled build is interrupted, then killed after the delay.
	if cmd.Cancel == nil || cmd.WaitDelay != waitDelay {
		t.Errorf("Cancel set: %v, WaitDelay %v; want an interrupt and %v", cmd.Cancel != nil, cmd.WaitDelay, waitDelay)
	}
}

func TestRootFromGOMOD(t *testing.T) {
	for _, gomod := range []string{"", os.DevNull} {
		if _, err := rootFromGOMOD(gomod); err == nil {
			t.Errorf("rootFromGOMOD(%q) = nil error", gomod)
		}
	}
	gomod := filepath.Join(t.TempDir(), "go.mod")
	if root, err := rootFromGOMOD(gomod); err != nil || root != filepath.Dir(gomod) {
		t.Errorf("rootFromGOMOD(%q) = %q, %v", gomod, root, err)
	}
}

func TestModuleRoot(t *testing.T) {
	goBin := lookGo(t)
	root, err := moduleRoot(t.Context(), goBin)
	if err != nil {
		t.Fatal(err)
	}
	gomod := readFile(t, filepath.Join(root, "go.mod"))
	if !bytes.HasPrefix(gomod, []byte("module github.com/hitoshyamamoto/irekey\n")) {
		t.Errorf("%s/go.mod is not the irekey module", root)
	}
	if _, err := moduleRoot(t.Context(), filepath.Join(t.TempDir(), "no-go")); err == nil {
		t.Error("moduleRoot with a missing go command succeeded")
	}
}

func lookGo(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not available:", err)
	}
	return goBin
}

func TestGoBuilder_NoGoCommand(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := goBuilder(t.Context()); err == nil || !strings.Contains(err.Error(), "finding the go command") {
		t.Errorf("goBuilder = %v, want an error for the missing go command", err)
	}
}

func TestGoBuilder_BuildsTheExecutor(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the executor with the go command")
	}
	lookGo(t)
	build, err := goBuilder(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "irekey-exec")
	if err := build(t.Context(), defaults[0], "v0.0.0-gentest", out); err != nil {
		t.Fatal(err)
	}
	data := readFile(t, out)
	if !bytes.HasPrefix(data, []byte("\x7fELF")) {
		t.Error("the executor is not an ELF binary")
	}
	if !bytes.Contains(data, []byte("v0.0.0-gentest")) {
		t.Error("the version set with -ldflags is not in the executor")
	}

	// A canceled build fails and says so.
	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if err := build(ctx, defaults[1], "v0.0.0-gentest", out+"-arm64"); err == nil || !strings.Contains(err.Error(), "go build") {
		t.Errorf("canceled build: %v, want a go build error", err)
	}
}

// helperEnv makes the test binary act as a child process instead of
// running the tests (see TestMain).
const helperEnv = "IREKEY_GEN_TEST_HELPER"

// markEnv names the file a helper creates when it is interrupted.
const markEnv = "IREKEY_GEN_TEST_MARK"

// TestMain lets the test binary stand in for child processes, as selected
// by IREKEY_GEN_TEST_HELPER:
//   - "interrupt" prints "ready", then waits for an interrupt; on one, it
//     creates the file named by IREKEY_GEN_TEST_MARK, if any, and exits 3.
//   - "parent" first starts an "interrupt" helper as its own child, the way
//     go build starts the compiler, then behaves like "interrupt" without
//     the file.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "interrupt":
		waitForInterrupt(os.Getenv(markEnv))
	case "parent":
		// The child gives up on its own after 30 seconds.
		child := exec.CommandContext(context.Background(), os.Args[0])
		child.Env = append(os.Environ(), helperEnv+"=interrupt")
		stdout, err := child.StdoutPipe()
		if err == nil {
			err = child.Start()
		}
		if err != nil {
			os.Exit(5)
		}
		if line, _ := bufio.NewReader(stdout).ReadString('\n'); line != "ready\n" {
			os.Exit(6)
		}
		_ = os.Unsetenv(markEnv)
		waitForInterrupt("")
	}
	os.Exit(m.Run())
}

// waitForInterrupt prints "ready" and exits 3 on an interrupt, after
// creating mark if it is not empty; it gives up after 30 seconds.
func waitForInterrupt(mark string) {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	fmt.Println("ready")
	select {
	case <-interrupted:
		if mark != "" {
			_ = os.WriteFile(mark, nil, 0o600)
		}
		os.Exit(3)
	case <-time.After(30 * time.Second):
		os.Exit(4)
	}
}

func TestInterruptOnCancel_ReachesTheProcessesItStarted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no process groups and no interrupt to send")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The helper's own child creates the mark when it is interrupted, as a
	// compiler started by go build would be stopped.
	mark := filepath.Join(t.TempDir(), "child-interrupted")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), helperEnv+"=parent", markEnv+"="+mark)
	interruptOnCancel(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); line != "ready\n" {
		t.Fatalf("helper printed %q, %v; want ready", line, err)
	}
	cancel()
	_ = cmd.Wait()
	// The helper exits with 3 only when it receives the interrupt; a kill
	// would end it without an exit code.
	if code := cmd.ProcessState.ExitCode(); code != 3 {
		t.Errorf("exit code %d, want 3 (stopped by an interrupt)", code)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(mark); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the helper's child was not interrupted: it would be left running")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// unclean joins elements with the path separator without cleaning the
// result: filepath.Join would remove "link/.." lexically.
func unclean(elem ...string) string {
	return strings.Join(elem, string(filepath.Separator))
}

// symlink creates a symbolic link or skips the test where it cannot.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skip("cannot create a symbolic link:", err)
	}
}

func TestCheckDirs_DistOutsideOut(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bin")
	for name, dist := range map[string]string{
		"no -dist":              "",
		"sibling":               filepath.Join(dir, "dist"),
		"sibling with a prefix": filepath.Join(dir, "bin-dist"),
		"out inside dist":       dir,
	} {
		if err := checkDirs(options{out: out, dist: dist}); err != nil {
			t.Errorf("%s: checkDirs = %v, want nil", name, err)
		}
	}
	// checkDirs creates both directories.
	for _, d := range []string{out, filepath.Join(dir, "dist")} {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			t.Errorf("%s was not created: %v", d, err)
		}
	}
}

func TestCheckDirs_DistInsideOut(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(filepath.Join(out, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	symlink(t, out, filepath.Join(dir, "outlink"))
	// A link into a subdirectory of -out, followed by "..": filepath.Abs
	// would clean "sublink/.." away lexically, but the operating system
	// resolves it to -out itself.
	symlink(t, filepath.Join(out, "sub"), filepath.Join(dir, "sublink"))
	// A dangling link to the -out directory, which does not exist yet:
	// creating -out makes the link point into it.
	symlink(t, filepath.Join(dir, "later"), filepath.Join(dir, "laterlink"))

	for _, tt := range []struct {
		name, out, dist string
	}{
		{"same directory", out, out},
		{"same directory, spelled differently", out, filepath.Join(out, "sub", "..")},
		{"subdirectory", out, filepath.Join(out, "raw")},
		{"link to out", out, filepath.Join(dir, "outlink")},
		{"inside a link to out", out, filepath.Join(dir, "outlink", "raw")},
		{"out through a link", filepath.Join(dir, "outlink"), filepath.Join(out, "raw")},
		{"link then ..", out, unclean(dir, "sublink", "..")},
		{"link then .. then a subdirectory", out, unclean(dir, "sublink", "..", "raw")},
		{"dangling link to out", filepath.Join(dir, "later"), filepath.Join(dir, "laterlink", "raw")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDirs(options{out: tt.out, dist: tt.dist})
			if err == nil || !strings.Contains(err.Error(), "inside it") {
				t.Errorf("checkDirs(-out %s, -dist %s) = %v, want it refused", tt.out, tt.dist, err)
			}
		})
	}

	t.Run("relative paths", func(t *testing.T) {
		t.Chdir(dir)
		for _, dist := range []string{"out", unclean("sublink", ".."), unclean("outlink", "raw")} {
			if err := checkDirs(options{out: "out", dist: dist}); err == nil {
				t.Errorf("checkDirs(-out out, -dist %s) = nil, want it refused", dist)
			}
		}
		if err := checkDirs(options{out: "out", dist: "dist"}); err != nil {
			t.Errorf("checkDirs(-out out, -dist dist) = %v", err)
		}
	})

	t.Run("generate refuses before building", func(t *testing.T) {
		fake := newFake()
		_, err := generate(t.Context(), options{version: "v1", out: out, dist: filepath.Join(dir, "outlink", "raw"),
			platforms: defaults}, fake.build, io.Discard, io.Discard)
		if err == nil || len(fake.calls) > 0 {
			t.Errorf("generate = %v after %d builds, want a refusal before any build", err, len(fake.calls))
		}
	})
}

func TestCheckDirs_Unusable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, o := range map[string]options{
		"out below a file":  {out: filepath.Join(file, "bin"), dist: filepath.Join(dir, "dist")},
		"dist below a file": {out: filepath.Join(dir, "bin"), dist: filepath.Join(file, "dist")},
	} {
		if err := checkDirs(o); err == nil {
			t.Errorf("%s: checkDirs = nil, want an error", name)
		}
	}
}

// buildDigest builds the linux/amd64 executor with build and returns its
// SHA-256 digest.
func buildDigest(t *testing.T, build builder) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "irekey-exec")
	if err := build(t.Context(), defaults[0], "v0.0.0-gentest", out); err != nil {
		t.Fatal(err)
	}
	return digest(readFile(t, out))
}

func TestGoBuilder_IgnoresTheCallersGoSettings(t *testing.T) {
	// Settings in the caller's environment or go env file that would change
	// the executor's bytes have no effect: the manifest always describes
	// the build the generator defines.
	if testing.Short() {
		t.Skip("builds the executor with the go command")
	}
	lookGo(t)
	build, err := goBuilder(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	baseline := buildDigest(t, build)

	envFile := filepath.Join(t.TempDir(), "go.env")
	if err := os.WriteFile(envFile, []byte("GOFLAGS=-tags=fromenvfile\nGOEXPERIMENT=noregabi\nGOFIPS140=latest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		env  []string // KEY=VALUE pairs
	}{
		{"GOFLAGS", []string{"GOFLAGS=-tags=leaktest -gcflags=all=-N"}},
		{"GOEXPERIMENT", []string{"GOEXPERIMENT=noregabi"}},
		{"GOFIPS140", []string{"GOFIPS140=latest"}},
		{"go env file", []string{"GOENV=" + envFile}},
		{"instruction set level", []string{"GOAMD64=v3"}},
		{"cgo", []string{"CGO_ENABLED=1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, kv := range tt.env {
				key, value, _ := strings.Cut(kv, "=")
				t.Setenv(key, value)
			}
			if got := buildDigest(t, build); got != baseline {
				t.Errorf("with %q the executor has SHA-256 %s, without it %s", tt.env, got, baseline)
			}
		})
	}
}
