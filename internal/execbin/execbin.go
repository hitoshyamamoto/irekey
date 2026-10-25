// SPDX-License-Identifier: MIT

package execbin

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"
)

//go:generate go run ./internal/gen -version dev -out bin

// ManifestName is the name of the manifest, at the root of the file system
// that holds the executors.
const ManifestName = "manifest.json"

// Format is the manifest format that Load reads and the generator writes.
const Format = 1

// maxSize bounds the sizes a manifest may record. Reading an executor
// allocates the decompressed size the manifest records before it reads,
// and Open and OpenCompressed hold a whole executor in memory, so a size is
// bounded before it is trusted. 64 MiB leaves ample room above the
// executor's limit of 12 MiB.
const maxSize = 64 << 20

// maxManifestSize bounds the manifest that Load reads, before it is
// decoded. The generator's manifest for three executors is about 1 KiB, so
// 1 MiB leaves ample room.
const maxManifestSize = 1 << 20

// maxVersionLen bounds the length of a version.
const maxVersionLen = 128

// platformFiles maps each platform an executor is built for to the name of
// its compressed file: the executor's release asset name with .gz appended.
var platformFiles = map[string]string{
	"linux/amd64":  "irekey-exec-linux-amd64.gz",
	"linux/arm64":  "irekey-exec-linux-arm64.gz",
	"linux/arm/v7": "irekey-exec-linux-armv7.gz",
}

// The keys of a manifest and of each executor in it, exactly as the
// generator writes them.
var (
	manifestKeys = []string{"format", "version", "executors"}
	executorKeys = []string{"platform", "file", "sha256", "size", "gz_sha256", "gz_size"}
)

// files holds the directory bin: its README and, once they are generated,
// the manifest and the compressed executors. Generated files are ignored by
// git, so they do not mark the build as modified (vcs.modified).
//
//go:embed bin
var files embed.FS

var (
	// ErrNotEmbedded reports a file system without a manifest: the
	// executors were not generated.
	ErrNotEmbedded = errors.New("no executors are embedded (the manifest is missing)")
	// ErrInvalidManifest reports a manifest that cannot be read or decoded,
	// or that does not describe a valid set of executors.
	ErrInvalidManifest = errors.New("invalid executor manifest")
	// ErrUnknownFormat reports a manifest in a format this version does not
	// read.
	ErrUnknownFormat = errors.New("unknown executor manifest format")
	// ErrMismatch reports an executor whose compressed file or decompressed
	// content does not have the size and digest the manifest records,
	// including a file that is missing or cannot be decompressed.
	ErrMismatch = errors.New("executor does not match the manifest")
	// ErrUnknownPlatform reports a platform the manifest has no executor
	// for.
	ErrUnknownPlatform = errors.New("no executor for the platform")
)

// Manifest describes a set of executors. It is stored as manifest.json,
// next to the compressed executors.
type Manifest struct {
	// Format is the manifest format; Load accepts only Format.
	Format int `json:"format"`
	// Version is the version the executors were built with; see
	// ValidVersion.
	Version string `json:"version"`
	// Executors lists at most one executor per platform. The generator
	// sorts them by file name.
	Executors []Executor `json:"executors"`
}

// Executor describes one executor and its compressed file.
type Executor struct {
	// Platform is the operating system and architecture, with the ARM
	// version for 32-bit ARM: linux/amd64, linux/arm64 or linux/arm/v7.
	Platform string `json:"platform"`
	// File is the name of the gzip-compressed executor, next to the
	// manifest. It is fixed by the platform: irekey-exec-linux-amd64.gz,
	// irekey-exec-linux-arm64.gz or irekey-exec-linux-armv7.gz.
	File string `json:"file"`
	// SHA256 is the lowercase hexadecimal SHA-256 digest of the
	// uncompressed executor. It identifies the executor, and it is the
	// value a release's SHA256SUMS will list for the asset
	// irekey-exec-linux-<arch>.
	SHA256 string `json:"sha256"`
	// Size is the size of the uncompressed executor in bytes.
	Size int64 `json:"size"`
	// GzSHA256 is the lowercase hexadecimal SHA-256 digest of File.
	GzSHA256 string `json:"gz_sha256"`
	// GzSize is the size of File in bytes.
	GzSize int64 `json:"gz_size"`
}

// ValidVersion reports whether v can be the version of a set of executors:
// 1 to 128 characters, each a letter, a digit or one of . - + _. The
// generator passes the version inside -ldflags, which the go command splits
// at spaces and in which it interprets quotes, so no other character is
// allowed.
func ValidVersion(v string) bool {
	if v == "" || len(v) > maxVersionLen {
		return false
	}
	for i := range len(v) {
		c := v[i]
		if !isLower(c) && !isDigit(c) && (c < 'A' || c > 'Z') && c != '.' && c != '-' && c != '+' && c != '_' {
			return false
		}
	}
	return true
}

// EmbeddedFS returns the embedded files, with the manifest and the
// compressed executors at its root when they were generated. Pass it to
// Load, Manifest.Verify, Open and OpenCompressed.
func EmbeddedFS() fs.FS {
	sub, err := fs.Sub(files, "bin")
	if err != nil {
		// Unreachable: fs.Sub fails only for an invalid directory name,
		// and "bin" is a valid one.
		panic("execbin: " + err.Error())
	}
	return sub
}

// Embedded loads the manifest of the executors embedded in this binary. ok
// is false, with a nil error, when no executor is embedded: the files were
// not generated, as in a development build. Embedded reads only the
// manifest; Manifest.Verify, Open and OpenCompressed check the executors
// themselves.
func Embedded() (m Manifest, ok bool, err error) {
	return Load(EmbeddedFS())
}

// Load reads and validates the manifest at the root of fsys, which may be
// at most 1 MiB. ok is false, with a nil error, when fsys has no manifest. Errors wrap
// ErrInvalidManifest or ErrUnknownFormat, and the other results are then
// zero values. Load does not read the executors; see Manifest.Verify.
func Load(fsys fs.FS) (m Manifest, ok bool, err error) {
	data, err := readFile(fsys, ManifestName, maxManifestSize)
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("%w: reading %s: %w", ErrInvalidManifest, ManifestName, err)
	}
	if len(data) > maxManifestSize {
		return Manifest{}, false, invalidf("%s is larger than %d bytes", ManifestName, maxManifestSize)
	}
	m, err = parse(data)
	if err != nil {
		return Manifest{}, false, err
	}
	return m, true, nil
}

// invalidf returns an error wrapping ErrInvalidManifest.
func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %w", ErrInvalidManifest, fmt.Errorf(format, args...))
}

// parse decodes and validates a manifest. The format is read first, so that
// a manifest in another format is reported as such and not as a set of
// unknown keys. Keys must match exactly and appear once.
func parse(data []byte) (Manifest, error) {
	top, err := object(data)
	if err != nil {
		return Manifest{}, invalidf("%w", err)
	}
	rawFormat, ok := top["format"]
	if !ok {
		return Manifest{}, invalidf("no format")
	}
	var m Manifest
	if err := member(rawFormat, &m.Format); err != nil {
		return Manifest{}, invalidf("format: %w", err)
	}
	if m.Format != Format {
		return Manifest{}, fmt.Errorf("%w %d (this version reads format %d)", ErrUnknownFormat, m.Format, Format)
	}
	if err := exactKeys(top, manifestKeys); err != nil {
		return Manifest{}, invalidf("%w", err)
	}
	if err := member(top["version"], &m.Version); err != nil {
		return Manifest{}, invalidf("version: %w", err)
	}
	var executors []json.RawMessage
	if err := member(top["executors"], &executors); err != nil {
		return Manifest{}, invalidf("executors: %w", err)
	}
	for i, raw := range executors {
		e, err := parseExecutor(raw)
		if err != nil {
			return Manifest{}, invalidf("executor %d: %w", i, err)
		}
		m.Executors = append(m.Executors, e)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// parseExecutor decodes one executor object of a manifest.
func parseExecutor(data []byte) (Executor, error) {
	members, err := object(data)
	if err != nil {
		return Executor{}, err
	}
	if err := exactKeys(members, executorKeys); err != nil {
		return Executor{}, err
	}
	var e Executor
	for _, f := range []struct {
		key string
		v   any
	}{
		{"platform", &e.Platform},
		{"file", &e.File},
		{"sha256", &e.SHA256},
		{"size", &e.Size},
		{"gz_sha256", &e.GzSHA256},
		{"gz_size", &e.GzSize},
	} {
		if err := member(members[f.key], f.v); err != nil {
			return Executor{}, fmt.Errorf("%s: %w", f.key, err)
		}
	}
	return e, nil
}

// object decodes a JSON object into its members. Unlike decoding into a
// struct with encoding/json, which matches keys regardless of case and keeps
// the last of duplicate keys, it keeps keys exactly as written and rejects
// duplicates. It also rejects any data after the object.
func object(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	members := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected %v instead of a key", tok)
		}
		if _, dup := members[key]; dup {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		members[key] = raw
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the JSON object")
	}
	return members, nil
}

// exactKeys checks that members has exactly the keys want.
func exactKeys(members map[string]json.RawMessage, want []string) error {
	for _, k := range want {
		if _, ok := members[k]; !ok {
			return fmt.Errorf("missing key %q", k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(members)) {
		if !slices.Contains(want, k) {
			return fmt.Errorf("unknown key %q", k)
		}
	}
	return nil
}

// member decodes one member value into v. null is refused: encoding/json
// would leave v unchanged.
func member(raw json.RawMessage, v any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null is not allowed")
	}
	return json.Unmarshal(raw, v)
}

// validate checks that m describes a usable set of executors: a valid
// version, and executors for distinct supported platforms, each with the
// file name of its platform, well-formed digests and plausible sizes.
func (m Manifest) validate() error {
	if m.Format != Format {
		return invalidf("format %d, expected %d", m.Format, Format)
	}
	if !ValidVersion(m.Version) {
		return invalidf("version %q: use 1 to %d letters, digits and . - + _", m.Version, maxVersionLen)
	}
	if len(m.Executors) == 0 {
		return invalidf("no executors")
	}
	seen := make(map[string]bool, len(m.Executors))
	for i, e := range m.Executors {
		file, supported := platformFiles[e.Platform]
		switch {
		case !supported:
			return invalidf("executor %d: unsupported platform %q (supported: %s)", i, e.Platform, supportedPlatforms())
		case seen[e.Platform]:
			return invalidf("executor %d: platform %s is listed twice", i, e.Platform)
		case e.File != file:
			return invalidf("%s: file %q, expected %q", e.Platform, e.File, file)
		case !validDigest(e.SHA256):
			return invalidf("%s: sha256 %q is not a lowercase hexadecimal SHA-256 digest", e.Platform, e.SHA256)
		case !validDigest(e.GzSHA256):
			return invalidf("%s: gz_sha256 %q is not a lowercase hexadecimal SHA-256 digest", e.Platform, e.GzSHA256)
		case e.Size <= 0 || e.Size > maxSize:
			return invalidf("%s: size %d is not between 1 and %d", e.Platform, e.Size, maxSize)
		case e.GzSize <= 0 || e.GzSize > maxSize:
			return invalidf("%s: gz_size %d is not between 1 and %d", e.Platform, e.GzSize, maxSize)
		}
		seen[e.Platform] = true
	}
	return nil
}

// supportedPlatforms lists the supported platforms, sorted.
func supportedPlatforms() string {
	return strings.Join(slices.Sorted(maps.Keys(platformFiles)), ", ")
}

// Verify checks every executor of m against fsys, in order: first the size
// and digest of its compressed file, then the size and digest of the
// decompressed content, which must come from exactly one gzip member. It returns the first failure, wrapping ErrMismatch,
// or ErrInvalidManifest when m itself is not valid.
func (m Manifest) Verify(fsys fs.FS) error {
	if err := m.validate(); err != nil {
		return err
	}
	for _, e := range m.Executors {
		if _, _, err := e.read(fsys); err != nil {
			return err
		}
	}
	return nil
}

// Open returns the decompressed executor for platform (such as
// linux/arm/v7) from fsys, with its manifest entry.
//
// Open verifies before it returns: it reads the whole executor, checks the
// size and digest of the compressed file and of the decompressed content,
// and only then returns a stream over the verified bytes, which it holds in
// memory. Every byte read from the stream has therefore been verified, and
// a mismatch is reported by Open, not while reading. Every error wraps one
// of ErrNotEmbedded, ErrInvalidManifest, ErrUnknownFormat,
// ErrUnknownPlatform and ErrMismatch.
func Open(fsys fs.FS, platform string) (io.ReadCloser, Executor, error) {
	e, _, raw, err := verified(fsys, platform)
	if err != nil {
		return nil, Executor{}, err
	}
	return io.NopCloser(bytes.NewReader(raw)), e, nil
}

// OpenCompressed returns the gzip-compressed executor for platform from
// fsys, exactly as stored, with its manifest entry. This is the form that is
// streamed to a device and decompressed there.
//
// Like Open, it verifies before it returns: the size and digest of the
// compressed file, which must hold exactly one gzip member, and also the
// size and digest of its decompressed content, so the stream decompresses
// to the executor the manifest identifies. It holds the compressed file in memory. Errors are those of
// Open.
func OpenCompressed(fsys fs.FS, platform string) (io.ReadCloser, Executor, error) {
	e, gz, _, err := verified(fsys, platform)
	if err != nil {
		return nil, Executor{}, err
	}
	return io.NopCloser(bytes.NewReader(gz)), e, nil
}

// verified loads the manifest of fsys and returns the entry for platform
// with its compressed file and decompressed content, both checked.
func verified(fsys fs.FS, platform string) (e Executor, gz, raw []byte, err error) {
	m, ok, err := Load(fsys)
	if err != nil {
		return Executor{}, nil, nil, err
	}
	if !ok {
		return Executor{}, nil, nil, ErrNotEmbedded
	}
	i := slices.IndexFunc(m.Executors, func(x Executor) bool { return x.Platform == platform })
	if i < 0 {
		var available []string
		for _, x := range m.Executors {
			available = append(available, x.Platform)
		}
		return Executor{}, nil, nil, fmt.Errorf("%w %q (available: %s)", ErrUnknownPlatform, platform, strings.Join(available, ", "))
	}
	e = m.Executors[i]
	gz, raw, err = e.read(fsys)
	if err != nil {
		return Executor{}, nil, nil, err
	}
	return e, gz, raw, nil
}

// read returns the compressed file of e from fsys and its decompressed
// content, after checking them in order: the size and digest of the
// compressed file, then the size and digest of the decompressed content.
// Errors wrap ErrMismatch.
func (e Executor) read(fsys fs.FS) (gz, raw []byte, err error) {
	gz, err = readFile(fsys, e.File, e.GzSize)
	if err != nil {
		return nil, nil, e.mismatchf("%w", err)
	}
	if n := int64(len(gz)); n != e.GzSize {
		return nil, nil, e.mismatchf("the compressed file has %s, the manifest records %d", bytesCount(n, e.GzSize), e.GzSize)
	}
	if sum := digest(gz); sum != e.GzSHA256 {
		return nil, nil, e.mismatchf("the compressed file has SHA-256 %s, the manifest records %s", sum, e.GzSHA256)
	}

	// The file must hold exactly one gzip member and nothing after it; the
	// generator writes one member. This is defense in depth: the digest of
	// the compressed file, checked above, already fixes every byte of it.
	br := bytes.NewReader(gz)
	zr, err := gzip.NewReader(br)
	if err != nil {
		return nil, nil, e.mismatchf("decompressing: %w", err)
	}
	defer func() { _ = zr.Close() }()
	zr.Multistream(false)
	// The size was validated against maxSize, so it can be allocated up
	// front.
	raw = make([]byte, e.Size)
	if n, err := io.ReadFull(zr, raw); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil, e.mismatchf("decompressing: the content ends after %d bytes, the manifest records %d: %w", n, e.Size, err)
		}
		return nil, nil, e.mismatchf("decompressing: %w", err)
	}
	// The content must end here. Reading on to the end of the member also
	// makes the reader check the gzip trailer (CRC-32 and length).
	var extra [1]byte
	n, err := io.ReadFull(zr, extra[:])
	if n > 0 {
		return nil, nil, e.mismatchf("the executor has more than %d bytes, the manifest records %d", e.Size, e.Size)
	}
	if !errors.Is(err, io.EOF) {
		return nil, nil, e.mismatchf("decompressing: %w", err)
	}
	// A bytes.Reader is an io.ByteReader, so with Multistream(false) the
	// gzip reader stops right after the member (compress/gzip); whatever
	// is left follows the gzip stream.
	if rest := br.Len(); rest > 0 {
		return nil, nil, e.mismatchf("trailing data after the gzip stream: %d bytes", rest)
	}
	if sum := digest(raw); sum != e.SHA256 {
		return nil, nil, e.mismatchf("the executor has SHA-256 %s, the manifest records %s", sum, e.SHA256)
	}
	return gz, raw, nil
}

// mismatchf returns an error wrapping ErrMismatch for e.
func (e Executor) mismatchf(format string, args ...any) error {
	return fmt.Errorf("%w: %s (%s): %w", ErrMismatch, e.Platform, e.File, fmt.Errorf(format, args...))
}

// readFile reads the named file from fsys, but at most limit+1 bytes, which
// is enough to tell that a file is larger than limit.
func readFile(fsys fs.FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit+1))
}

// bytesCount describes a size that was read with a limit of want+1 bytes.
func bytesCount(got, want int64) string {
	if got > want {
		return fmt.Sprintf("more than %d bytes", want)
	}
	return fmt.Sprintf("%d bytes", got)
}

// digest returns the lowercase hexadecimal SHA-256 digest of data.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// validDigest reports whether s is a lowercase hexadecimal SHA-256 digest.
func validDigest(s string) bool {
	if len(s) != 2*sha256.Size {
		return false
	}
	for i := range len(s) {
		if !isDigit(s[i]) && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}

func isLower(c byte) bool { return 'a' <= c && c <= 'z' }

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
