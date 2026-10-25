// SPDX-License-Identifier: MIT

package execbin

import (
	"bytes"
	"compress/gzip"
	"debug/elf"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// testPlatforms are the platforms of the fixture, with their file names.
var testPlatforms = []struct{ platform, file string }{
	{"linux/amd64", "irekey-exec-linux-amd64.gz"},
	{"linux/arm64", "irekey-exec-linux-arm64.gz"},
	{"linux/arm/v7", "irekey-exec-linux-armv7.gz"},
}

// releaseMachines are the platforms a release embeds, with the ELF class
// and machine of their executors.
var releaseMachines = map[string]struct {
	class   elf.Class
	machine elf.Machine
}{
	"linux/amd64":  {elf.ELFCLASS64, elf.EM_X86_64},
	"linux/arm64":  {elf.ELFCLASS64, elf.EM_AARCH64},
	"linux/arm/v7": {elf.ELFCLASS32, elf.EM_ARM},
}

// content is the fake executor of a platform in the fixture.
func content(platform string) []byte {
	return bytes.Repeat([]byte("fake executor for "+platform+"\n"), 64)
}

func compressForTest(tb testing.TB, data []byte) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		tb.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func encodeManifest(tb testing.TB, m Manifest) []byte {
	tb.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		tb.Fatal(err)
	}
	return append(data, '\n')
}

// fixture returns a file system with a compressed fake executor per
// platform and a manifest that matches them.
func fixture(tb testing.TB) (fstest.MapFS, Manifest) {
	tb.Helper()
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Embedded executors\n")}}
	m := Manifest{Format: Format, Version: "v0.0.0-test"}
	for _, p := range testPlatforms {
		raw := content(p.platform)
		gz := compressForTest(tb, raw)
		fsys[p.file] = &fstest.MapFile{Data: gz}
		m.Executors = append(m.Executors, Executor{
			Platform: p.platform,
			File:     p.file,
			SHA256:   digest(raw),
			Size:     int64(len(raw)),
			GzSHA256: digest(gz),
			GzSize:   int64(len(gz)),
		})
	}
	fsys[ManifestName] = &fstest.MapFile{Data: encodeManifest(tb, m)}
	return fsys, m
}

// withManifest stores m as the manifest of fsys.
func withManifest(tb testing.TB, fsys fstest.MapFS, m Manifest) fstest.MapFS {
	tb.Helper()
	fsys[ManifestName] = &fstest.MapFile{Data: encodeManifest(tb, m)}
	return fsys
}

// withRawManifest stores data as the manifest of a new file system.
func withRawManifest(data string) fstest.MapFS {
	return fstest.MapFS{ManifestName: &fstest.MapFile{Data: []byte(data)}}
}

// clone returns a deep copy of m, so that a test can change one executor.
func clone(m Manifest) Manifest {
	m.Executors = slices.Clone(m.Executors)
	return m
}

func TestLoad_Valid(t *testing.T) {
	fsys, want := fixture(t)
	got, ok, err := Load(fsys)
	if err != nil || !ok {
		t.Fatalf("Load = ok %v, error %v; want ok", ok, err)
	}
	if got.Format != want.Format || got.Version != want.Version || !slices.Equal(got.Executors, want.Executors) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingManifest(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"empty":       {},
		"readme only": {"README.md": &fstest.MapFile{Data: []byte("# Embedded executors\n")}},
	} {
		t.Run(name, func(t *testing.T) {
			m, ok, err := Load(fsys)
			if err != nil || ok {
				t.Fatalf("Load = ok %v, error %v; want not ok and no error", ok, err)
			}
			if m.Version != "" || m.Executors != nil {
				t.Errorf("Load returned %+v, want the zero Manifest", m)
			}
		})
	}
}

func TestLoad_ReadError(t *testing.T) {
	// A directory named like the manifest exists but cannot be read as a
	// file: that is an invalid manifest, not a missing one.
	fsys := fstest.MapFS{ManifestName: &fstest.MapFile{Mode: fs.ModeDir}}
	_, ok, err := Load(fsys)
	if !errors.Is(err, ErrInvalidManifest) || ok || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load = ok %v, error %v; want ErrInvalidManifest for a read error", ok, err)
	}
}

// validExecutorJSON is one executor object with every key, as the
// generator writes it.
const validExecutorJSON = `{"platform": "linux/amd64", "file": "irekey-exec-linux-amd64.gz",
	"sha256": "` + zeros + `", "size": 1, "gz_sha256": "` + zeros + `", "gz_size": 1}`

const zeros = "0000000000000000000000000000000000000000000000000000000000000000"

func TestLoad_ValidRawManifest(t *testing.T) {
	// The raw form the strictness cases below are variations of.
	fsys := withRawManifest(`{"format": 1, "version": "v1", "executors": [` + validExecutorJSON + `]}`)
	if _, ok, err := Load(fsys); err != nil || !ok {
		t.Fatalf("Load = ok %v, error %v; want ok", ok, err)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	executor := validExecutorJSON
	for name, data := range map[string]string{
		"truncated":                 `{"format": 1, "version": "v1", "executors": [`,
		"not an object":             `[1, 2, 3]`,
		"string":                    `"manifest"`,
		"trailing data":             `{"format": 1} {}`,
		"trailing garbage":          `{"format": 1} x`,
		"wrong type":                `{"format": "1"}`,
		"fraction":                  `{"format": 1.0}`,
		"empty":                     ``,
		"no format":                 `{"version": "v1"}`,
		"null format":               `{"format": null}`,
		"format in upper case":      `{"Format": 1, "version": "v1", "executors": [` + executor + `]}`,
		"version in upper case":     `{"format": 1, "Version": "v1", "executors": [` + executor + `]}`,
		"duplicate key":             `{"format": 1, "version": "v1", "version": "v2", "executors": [` + executor + `]}`,
		"duplicate format":          `{"format": 1, "format": 1, "version": "v1", "executors": [` + executor + `]}`,
		"unknown key":               `{"format": 1, "version": "v1", "executors": [` + executor + `], "extra": true}`,
		"missing key":               `{"format": 1, "executors": [` + executor + `]}`,
		"null version":              `{"format": 1, "version": null, "executors": [` + executor + `]}`,
		"null executors":            `{"format": 1, "version": "v1", "executors": null}`,
		"executors not an array":    `{"format": 1, "version": "v1", "executors": {}}`,
		"executor not an object":    `{"format": 1, "version": "v1", "executors": [1]}`,
		"executor key in uppercase": `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size"`, `"Size"`, 1) + `]}`,
		"executor duplicate key":    `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1`, `"size": 1, "size": 2`, 1) + `]}`,
		"executor unknown key":      `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1`, `"size": 1, "mode": 493`, 1) + `]}`,
		"executor missing key":      `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1, `, ``, 1) + `]}`,
		"executor null value":       `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1`, `"size": null`, 1) + `]}`,
		"size not an integer":       `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1`, `"size": 1.5`, 1) + `]}`,
		"size as a string":          `{"format": 1, "version": "v1", "executors": [` + strings.Replace(executor, `"size": 1`, `"size": "1"`, 1) + `]}`,
		"executor as a string":      `{"format": 1, "version": "v1", "executors": ["{}"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, ok, err := Load(withRawManifest(data))
			if !errors.Is(err, ErrInvalidManifest) || ok {
				t.Fatalf("Load = ok %v, error %v; want ErrInvalidManifest", ok, err)
			}
		})
	}
}

func TestLoad_UnknownFormat(t *testing.T) {
	for _, format := range []string{"0", "2", "-1"} {
		t.Run(format, func(t *testing.T) {
			// Keys of another format are not reported as unknown keys.
			_, ok, err := Load(withRawManifest(`{"format": ` + format + `, "something_new": true}`))
			if !errors.Is(err, ErrUnknownFormat) || ok {
				t.Fatalf("Load = ok %v, error %v; want ErrUnknownFormat", ok, err)
			}
			if !strings.Contains(err.Error(), "format "+format) {
				t.Errorf("error %q does not name format %s", err, format)
			}
		})
	}
}

func TestLoad_InvalidContent(t *testing.T) {
	_, valid := fixture(t)
	bad := strings.Repeat("0", 63) + "g"
	tests := []struct {
		name   string
		change func(m *Manifest)
	}{
		{"no version", func(m *Manifest) { m.Version = "" }},
		{"version with a space", func(m *Manifest) { m.Version = "v 1" }},
		{"version with a quote", func(m *Manifest) { m.Version = `v1"` }},
		{"version too long", func(m *Manifest) { m.Version = strings.Repeat("v", maxVersionLen+1) }},
		{"no executors", func(m *Manifest) { m.Executors = nil }},
		{"empty platform", func(m *Manifest) { m.Executors[0].Platform = "" }},
		{"platform without arch", func(m *Manifest) { m.Executors[0].Platform = "linux" }},
		{"other operating system", func(m *Manifest) { m.Executors[0].Platform = "plan9/foo" }},
		{"unsupported architecture", func(m *Manifest) { m.Executors[0].Platform = "linux/386" }},
		{"arm version without v", func(m *Manifest) { m.Executors[2].Platform = "linux/arm/7" }},
		{"other arm version", func(m *Manifest) { m.Executors[2].Platform = "linux/arm/v6" }},
		{"platform in upper case", func(m *Manifest) { m.Executors[0].Platform = "Linux/amd64" }},
		{"duplicate platform", func(m *Manifest) { m.Executors[1] = m.Executors[0] }},
		{"file of another platform", func(m *Manifest) { m.Executors[0].File = "irekey-exec-linux-arm64.gz" }},
		{"empty file", func(m *Manifest) { m.Executors[0].File = "" }},
		{"file in a directory", func(m *Manifest) { m.Executors[0].File = "sub/irekey-exec-linux-amd64.gz" }},
		{"file outside", func(m *Manifest) { m.Executors[0].File = "../irekey-exec-linux-amd64.gz" }},
		{"uncompressed file", func(m *Manifest) { m.Executors[0].File = "irekey-exec-linux-amd64" }},
		{"manifest as file", func(m *Manifest) { m.Executors[0].File = ManifestName }},
		{"short digest", func(m *Manifest) { m.Executors[0].SHA256 = "abc" }},
		{"upper-case digest", func(m *Manifest) { m.Executors[0].SHA256 = strings.ToUpper(m.Executors[0].SHA256) }},
		{"non-hex digest", func(m *Manifest) { m.Executors[0].SHA256 = bad }},
		{"bad gz digest", func(m *Manifest) { m.Executors[0].GzSHA256 = bad }},
		{"zero size", func(m *Manifest) { m.Executors[0].Size = 0 }},
		{"negative size", func(m *Manifest) { m.Executors[0].Size = -1 }},
		{"size above 64 MiB", func(m *Manifest) { m.Executors[0].Size = 64<<20 + 1 }},
		{"zero gz size", func(m *Manifest) { m.Executors[0].GzSize = 0 }},
		{"huge gz size", func(m *Manifest) { m.Executors[0].GzSize = 1 << 62 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := clone(valid)
			tt.change(&m)
			fsys := withManifest(t, fstest.MapFS{}, m)
			_, ok, err := Load(fsys)
			if !errors.Is(err, ErrInvalidManifest) || ok {
				t.Fatalf("Load = ok %v, error %v; want ErrInvalidManifest", ok, err)
			}
			// Verify validates a manifest that did not come from Load too.
			if err := m.Verify(fsys); !errors.Is(err, ErrInvalidManifest) {
				t.Errorf("Verify = %v, want ErrInvalidManifest", err)
			}
		})
	}
}

func TestLoad_ManifestSizeLimit(t *testing.T) {
	// The manifest is read with a bound: a valid manifest padded with
	// whitespace to exactly 1 MiB loads, one byte more is refused.
	fsys, m := fixture(t)
	manifest := encodeManifest(t, m)
	padded := func(size int) []byte {
		return append(bytes.Repeat([]byte(" "), size-len(manifest)), manifest...)
	}
	fsys[ManifestName] = &fstest.MapFile{Data: padded(1 << 20)}
	if _, ok, err := Load(fsys); err != nil || !ok {
		t.Fatalf("Load of a 1 MiB manifest = ok %v, error %v; want ok", ok, err)
	}
	fsys[ManifestName] = &fstest.MapFile{Data: padded(1<<20 + 1)}
	_, ok, err := Load(fsys)
	if !errors.Is(err, ErrInvalidManifest) || ok || !strings.Contains(err.Error(), "larger than 1048576 bytes") {
		t.Fatalf("Load of a manifest above 1 MiB = ok %v, error %v; want ErrInvalidManifest for its size", ok, err)
	}
}

func TestLoad_SizeLimit(t *testing.T) {
	// 64 MiB is accepted; the bound is far above the executor's 12 MiB.
	fsys, m := fixture(t)
	m = clone(m)
	m.Executors[0].Size = 64 << 20
	if _, _, err := Load(withManifest(t, fsys, m)); err != nil {
		t.Fatal(err)
	}
}

func TestValidVersion(t *testing.T) {
	for _, v := range []string{"dev", "ci", "v0.1.0", "v1.2.3-rc.1+build.5", "0.0.0_test", strings.Repeat("v", maxVersionLen)} {
		if !ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = false", v)
		}
	}
	for _, v := range []string{"", "v1 2", `v1"`, "v1'", "v1;x", "v1/x", "v1=x", "v1\n", "v\x00",
		// A non-ASCII letter, built at run time so the source stays ASCII.
		"v" + string(rune(0xe9)), strings.Repeat("v", maxVersionLen+1)} {
		if ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = true", v)
		}
	}
}

func TestVerify_Valid(t *testing.T) {
	fsys, m := fixture(t)
	if err := m.Verify(fsys); err != nil {
		t.Fatal(err)
	}
}

func TestVerify_ZeroManifest(t *testing.T) {
	if err := (Manifest{}).Verify(fstest.MapFS{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Verify = %v, want ErrInvalidManifest", err)
	}
}

// mismatchCase changes the files or the manifest of the fixture so that
// one executor no longer matches.
type mismatchCase struct {
	name   string
	change func(fsys fstest.MapFS, m *Manifest)
	want   string // part of the error message
	is     error  // an error the result must also wrap, if any
}

// appendToGzip appends extra to the compressed file of e and records the
// new file in e, so that only decompressing can find the problem.
func appendToGzip(fsys fstest.MapFS, e *Executor, extra []byte) {
	data := append(slices.Clone(fsys[e.File].Data), extra...)
	fsys[e.File] = &fstest.MapFile{Data: data}
	e.GzSHA256, e.GzSize = digest(data), int64(len(data))
}

// emptyGzipMember returns a gzip member with no content.
func emptyGzipMember() []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_ = zw.Close()
	return buf.Bytes()
}

var mismatchCases = []mismatchCase{
	// Exactly one gzip member is accepted, with nothing after it. Reading
	// several members (the compress/gzip default) would accept the empty
	// member; reading one member without checking what follows would accept
	// all three.
	{
		name:   "garbage byte after the gzip stream",
		change: func(fsys fstest.MapFS, m *Manifest) { appendToGzip(fsys, &m.Executors[0], []byte{'x'}) },
		want:   "trailing data after the gzip stream: 1 bytes",
	},
	{
		name:   "zero bytes after the gzip stream",
		change: func(fsys fstest.MapFS, m *Manifest) { appendToGzip(fsys, &m.Executors[1], make([]byte, 16)) },
		want:   "trailing data after the gzip stream: 16 bytes",
	},
	{
		name:   "empty gzip member after the gzip stream",
		change: func(fsys fstest.MapFS, m *Manifest) { appendToGzip(fsys, &m.Executors[2], emptyGzipMember()) },
		want:   "trailing data after the gzip stream: 20 bytes",
	},
	{
		name: "gz digest",
		change: func(fsys fstest.MapFS, m *Manifest) {
			e := m.Executors[1]
			data := slices.Clone(fsys[e.File].Data)
			data[len(data)/2] ^= 0xff
			fsys[e.File] = &fstest.MapFile{Data: data}
		},
		want: "the compressed file has SHA-256",
	},
	{
		name: "gz size larger",
		change: func(fsys fstest.MapFS, m *Manifest) {
			e := m.Executors[0]
			fsys[e.File] = &fstest.MapFile{Data: append(slices.Clone(fsys[e.File].Data), 0)}
		},
		want: "the compressed file has more than",
	},
	{
		name: "gz size smaller",
		change: func(_ fstest.MapFS, m *Manifest) {
			m.Executors[0].GzSize++
		},
		want: "the compressed file has",
	},
	{
		name: "missing file",
		change: func(fsys fstest.MapFS, m *Manifest) {
			delete(fsys, m.Executors[2].File)
		},
		want: "irekey-exec-linux-armv7.gz",
		is:   fs.ErrNotExist,
	},
	{
		name: "swapped files",
		change: func(fsys fstest.MapFS, m *Manifest) {
			// The amd64 entry records the digests of the arm64 file; the
			// file name is still the amd64 one, so the content is wrong.
			a, b := m.Executors[0].File, m.Executors[1].File
			fsys[a], fsys[b] = fsys[b], fsys[a]
		},
		want: "the compressed file has",
	},
	{
		name: "raw digest",
		change: func(_ fstest.MapFS, m *Manifest) {
			m.Executors[2].SHA256 = digest([]byte("another executor"))
		},
		want: "the executor has SHA-256",
	},
	{
		name: "raw size larger than recorded",
		change: func(_ fstest.MapFS, m *Manifest) {
			m.Executors[0].Size--
		},
		want: "the executor has more than",
	},
	{
		name: "raw size smaller than recorded",
		change: func(_ fstest.MapFS, m *Manifest) {
			m.Executors[0].Size++
		},
		want: "the content ends after",
		is:   io.ErrUnexpectedEOF,
	},
	{
		name: "truncated gzip",
		change: func(fsys fstest.MapFS, m *Manifest) {
			// The compressed file and its manifest entry agree, so the
			// failure is found by decompressing.
			e := &m.Executors[0]
			data := fsys[e.File].Data
			data = slices.Clone(data[:len(data)-4])
			fsys[e.File] = &fstest.MapFile{Data: data}
			e.GzSHA256, e.GzSize = digest(data), int64(len(data))
		},
		want: "decompressing",
		is:   io.ErrUnexpectedEOF,
	},
	{
		name: "not gzip",
		change: func(fsys fstest.MapFS, m *Manifest) {
			e := &m.Executors[1]
			data := []byte("plain text, not gzip")
			fsys[e.File] = &fstest.MapFile{Data: data}
			e.GzSHA256, e.GzSize = digest(data), int64(len(data))
		},
		want: "decompressing",
		is:   gzip.ErrHeader,
	},
	{
		name: "gzip checksum",
		change: func(fsys fstest.MapFS, m *Manifest) {
			// Damage the CRC-32 in the trailer: the content decompresses to
			// the recorded size, and the reader reports the checksum.
			e := &m.Executors[1]
			data := slices.Clone(fsys[e.File].Data)
			data[len(data)-8] ^= 0xff
			fsys[e.File] = &fstest.MapFile{Data: data}
			e.GzSHA256 = digest(data)
		},
		want: "decompressing",
		is:   gzip.ErrChecksum,
	},
}

func TestVerify_Mismatch(t *testing.T) {
	for _, tt := range mismatchCases {
		t.Run(tt.name, func(t *testing.T) {
			fsys, m := fixture(t)
			m = clone(m)
			tt.change(fsys, &m)
			checkMismatch(t, m.Verify(fsys), tt)
		})
	}
}

func checkMismatch(t *testing.T, err error, tt mismatchCase) {
	t.Helper()
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("error %v, want ErrMismatch", err)
	}
	if tt.is != nil && !errors.Is(err, tt.is) {
		t.Errorf("error %v, want it to wrap %v", err, tt.is)
	}
	if !strings.Contains(err.Error(), tt.want) {
		t.Errorf("error %q does not contain %q", err, tt.want)
	}
}

// opener is Open or OpenCompressed.
type opener func(fs.FS, string) (io.ReadCloser, Executor, error)

var openers = []struct {
	name string
	open opener
	// want returns the bytes the opener must return for an executor of
	// the fixture.
	want func(fsys fstest.MapFS, e Executor) []byte
}{
	{"Open", Open, func(_ fstest.MapFS, e Executor) []byte { return content(e.Platform) }},
	{"OpenCompressed", OpenCompressed, func(fsys fstest.MapFS, e Executor) []byte { return fsys[e.File].Data }},
}

func TestOpen_Valid(t *testing.T) {
	fsys, m := fixture(t)
	for _, o := range openers {
		for _, want := range m.Executors {
			t.Run(o.name+" "+want.Platform, func(t *testing.T) {
				rc, e, err := o.open(fsys, want.Platform)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = rc.Close() }()
				if e != want {
					t.Errorf("returned the entry %+v, want %+v", e, want)
				}
				got, err := io.ReadAll(rc)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, o.want(fsys, want)) {
					t.Errorf("returned %d bytes that differ from the expected ones", len(got))
				}
			})
		}
	}
}

func TestOpenCompressed_DecompressesToTheExecutor(t *testing.T) {
	fsys, m := fixture(t)
	rc, e, err := OpenCompressed(fsys, m.Executors[2].Platform)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(rc)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if digest(raw) != e.SHA256 || int64(len(raw)) != e.Size {
		t.Error("the compressed stream does not decompress to the executor of the manifest")
	}
}

func TestOpen_UnknownPlatform(t *testing.T) {
	fsys, m := fixture(t)
	// A manifest without the arm64 executor: a supported platform can be
	// missing too.
	partial := clone(m)
	partial.Executors = slices.Delete(partial.Executors, 1, 2)
	partialFS := withManifest(t, fstest.MapFS{}, partial)
	for k, v := range fsys {
		if k != ManifestName {
			partialFS[k] = v
		}
	}
	for _, o := range openers {
		for _, platform := range []string{"linux/386", "linux/arm", "linux/arm/7", "", "windows/amd64", "linux/arm64"} {
			rc, _, err := o.open(partialFS, platform)
			if !errors.Is(err, ErrUnknownPlatform) || rc != nil {
				t.Fatalf("%s(%q) = %v, %v; want ErrUnknownPlatform", o.name, platform, rc, err)
			}
			if !strings.Contains(err.Error(), "linux/amd64, linux/arm/v7") {
				t.Errorf("error %q does not list the available platforms", err)
			}
		}
	}
}

func TestOpen_NotEmbedded(t *testing.T) {
	for _, o := range openers {
		rc, _, err := o.open(fstest.MapFS{}, "linux/amd64")
		if !errors.Is(err, ErrNotEmbedded) || rc != nil {
			t.Fatalf("%s = %v, %v; want ErrNotEmbedded", o.name, rc, err)
		}
	}
}

func TestOpen_InvalidManifest(t *testing.T) {
	for _, o := range openers {
		for name, fsys := range map[string]fstest.MapFS{
			"corrupt":        withRawManifest("{"),
			"unknown format": withRawManifest(`{"format": 2}`),
			"unreadable":     {ManifestName: &fstest.MapFile{Mode: fs.ModeDir}},
		} {
			rc, _, err := o.open(fsys, "linux/amd64")
			if (!errors.Is(err, ErrInvalidManifest) && !errors.Is(err, ErrUnknownFormat)) || rc != nil {
				t.Fatalf("%s, %s manifest = %v, %v; want ErrInvalidManifest or ErrUnknownFormat", o.name, name, rc, err)
			}
		}
	}
}

func TestOpen_Mismatch(t *testing.T) {
	// Open and OpenCompressed verify the whole executor before they return,
	// so a mismatch is an error and no stream is returned.
	for _, o := range openers {
		for _, tt := range mismatchCases {
			t.Run(o.name+" "+tt.name, func(t *testing.T) {
				fsys, m := fixture(t)
				m = clone(m)
				tt.change(fsys, &m)
				withManifest(t, fsys, m)
				var platform string
				for _, e := range m.Executors {
					if _, _, err := e.read(fsys); err != nil {
						platform = e.Platform
					}
				}
				if platform == "" {
					t.Fatal("the case changes no executor")
				}
				rc, _, err := o.open(fsys, platform)
				if rc != nil {
					t.Error("a stream was returned for an executor that does not match")
				}
				checkMismatch(t, err, tt)
			})
		}
	}
}

func TestEmbeddedFS_HoldsTheReadme(t *testing.T) {
	// The README is tracked, so it is embedded in every build; it is what
	// lets the go:embed pattern match before the executors are generated.
	if _, err := fs.Stat(EmbeddedFS(), "README.md"); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedded_VerifiesWhenPresent(t *testing.T) {
	// Runs in every build: without generated files, Embedded reports that
	// nothing is embedded; with them, they must load and verify.
	m, ok, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Log("no executors are embedded (development build)")
		return
	}
	if err := m.Verify(EmbeddedFS()); err != nil {
		t.Fatal(err)
	}
	t.Logf("embedded executors, version %s, verified", m.Version)
}

// requireEmbedded reports whether IREKEY_REQUIRE_EMBEDDED asks for the
// generated executors. Unset or empty means no; any other value must be one
// that strconv.ParseBool accepts, so that a typo fails instead of skipping.
func requireEmbedded(t *testing.T) bool {
	t.Helper()
	v := os.Getenv("IREKEY_REQUIRE_EMBEDDED")
	if v == "" {
		return false
	}
	require, err := strconv.ParseBool(v)
	if err != nil {
		t.Fatalf("IREKEY_REQUIRE_EMBEDDED=%q is not a boolean (use 1 or 0, true or false): %v", v, err)
	}
	return require
}

// TestEmbedded requires the executors of a release to be embedded and to
// match their manifest. It runs only when IREKEY_REQUIRE_EMBEDDED is true,
// after go generate ./internal/execbin, as the CI build job does before it
// builds the operator command line.
func TestEmbedded(t *testing.T) {
	if !requireEmbedded(t) {
		t.Skip("set IREKEY_REQUIRE_EMBEDDED=1 to require the generated executors (go generate ./internal/execbin)")
	}
	fsys := EmbeddedFS()
	m, ok, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("no executors are embedded: run go generate ./internal/execbin")
	}
	if err := m.Verify(fsys); err != nil {
		t.Fatal(err)
	}

	// Exactly the README, the manifest and the listed files are embedded:
	// nothing else ends up in the operator command line.
	want := []string{"README.md", ManifestName}
	var platforms []string
	for _, e := range m.Executors {
		want = append(want, e.File)
		platforms = append(platforms, e.Platform)
	}
	slices.Sort(want)
	var got []string
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			got = append(got, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("embedded files %q, want exactly %q", got, want)
	}

	slices.Sort(platforms)
	wantPlatforms := slices.Sorted(maps.Keys(releaseMachines))
	if !slices.Equal(platforms, wantPlatforms) {
		t.Errorf("embedded platforms %v, want %v", platforms, wantPlatforms)
	}

	// Each executor is a Linux ELF binary for its platform's machine.
	for _, e := range m.Executors {
		rc, _, err := Open(fsys, e.Platform)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		f, err := elf.NewFile(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: %v", e.Platform, err)
		}
		if want := releaseMachines[e.Platform]; f.Class != want.class || f.Machine != want.machine {
			t.Errorf("%s: ELF %v %v, want %v %v", e.Platform, f.Class, f.Machine, want.class, want.machine)
		}
		t.Logf("%s %s sha256 %s, %d bytes, %d compressed, ELF %v %v",
			e.Platform, e.File, e.SHA256, e.Size, e.GzSize, f.Class, f.Machine)
	}
}

func FuzzLoadVerifyOpen(f *testing.F) {
	fsys, m := fixture(f)
	gz := fsys[m.Executors[0].File].Data
	f.Add(fsys[ManifestName].Data, gz)
	// A manifest whose only executor is the file: everything verifies.
	f.Add(encodeManifest(f, Manifest{Format: Format, Version: m.Version, Executors: m.Executors[:1]}), gz)
	f.Add([]byte(`{"format": 1, "version": "v1", "executors": [`+validExecutorJSON+`]}`), []byte{0x1f, 0x8b})
	f.Add([]byte("not json"), []byte{})

	f.Fuzz(func(t *testing.T, manifest, file []byte) {
		// Every supported file name holds the same content.
		fsys := fstest.MapFS{ManifestName: {Data: manifest}}
		for _, name := range platformFiles {
			fsys[name] = &fstest.MapFile{Data: file}
		}
		m, ok, err := Load(fsys)
		if err != nil {
			if !errors.Is(err, ErrInvalidManifest) && !errors.Is(err, ErrUnknownFormat) {
				t.Fatalf("Load error %v wraps no sentinel", err)
			}
			return
		}
		if !ok {
			t.Fatal("Load found no manifest")
		}
		verifyErr := m.Verify(fsys)
		if verifyErr != nil && !errors.Is(verifyErr, ErrMismatch) {
			t.Fatalf("Verify error %v does not wrap ErrMismatch", verifyErr)
		}
		allOpen := true
		for _, want := range m.Executors {
			for _, o := range openers {
				rc, e, err := o.open(fsys, want.Platform)
				if err != nil {
					if !errors.Is(err, ErrMismatch) {
						t.Fatalf("%s error %v does not wrap ErrMismatch", o.name, err)
					}
					allOpen = false
					continue
				}
				data, err := io.ReadAll(rc)
				if err != nil {
					t.Fatal(err)
				}
				sum, size := e.SHA256, e.Size
				if o.name == "OpenCompressed" {
					sum, size = e.GzSHA256, e.GzSize
				}
				// The property that matters: nothing is returned that does
				// not have the digest and size the manifest records.
				if digest(data) != sum || int64(len(data)) != size || e != want {
					t.Fatalf("%s(%s) returned bytes that do not match the manifest", o.name, want.Platform)
				}
			}
		}
		if (verifyErr == nil) != allOpen {
			t.Fatalf("Verify = %v, but every executor opened: %v", verifyErr, allOpen)
		}
	})
}
