// SPDX-License-Identifier: MIT

// Command gen builds the device-side executor, irekey-exec, for each Linux
// platform and writes the files that package execbin embeds: each executor
// gzip-compressed, and manifest.json with the SHA-256 digest and size of
// every executor and of its compressed file. With -dist it also writes the
// uncompressed executors under their release asset names
// (irekey-exec-linux-amd64, irekey-exec-linux-arm64, irekey-exec-linux-armv7).
//
// It fails if the linux/amd64 executor is larger than 12 MiB, and warns
// above the 8 MiB target.
//
// go generate ./internal/execbin runs it with -version dev -out bin. From the
// module root:
//
//	go run ./internal/execbin/internal/gen -version VERSION -out internal/execbin/bin [-dist dist]
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/hitoshyamamoto/irekey/internal/execbin"
)

const (
	// executorPkg is the executor's main package, relative to the module
	// root.
	executorPkg = "./cmd/irekey-exec"
	// versionVar is the variable that -ldflags -X sets to the version.
	versionVar = "github.com/hitoshyamamoto/irekey/internal/version.Version"
	// assetPrefix starts the name of every executor file.
	assetPrefix = "irekey-exec-"
	// defaultPlatforms are the device platforms irekey supports.
	defaultPlatforms = "linux/amd64,linux/arm64,linux/arm/7"

	// sizeLimit and sizeTarget apply to the stripped linux/amd64 executor,
	// as in scripts/check-exec-deps.sh: above the limit the generator fails,
	// above the target it warns.
	sizeLimit  = 12 << 20 // 12 MiB
	sizeTarget = 8 << 20  // 8 MiB

	// envTimeout bounds go env; buildTimeout bounds each go build.
	envTimeout   = time.Minute
	buildTimeout = 5 * time.Minute
	// waitDelay bounds how long a canceled go command may take to stop;
	// after it, os/exec kills the go process (only that process).
	waitDelay = 10 * time.Second
)

// Exit codes.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = `Usage: gen -version VERSION -out DIR [-dist DIR] [-platforms LIST]

gen builds the device-side executor (` + executorPkg + `) for each platform
and writes to the -out directory each executor gzip-compressed and
manifest.json with their SHA-256 digests and sizes, for package execbin to
embed. Run it with go generate ./internal/execbin, or with go run from the
module root.

Options:
`

// errUsage reports an invalid command line whose message has been printed.
var errUsage = errors.New("invalid command line")

// goSettings are set for every go command the generator runs, after the
// caller's environment, so that they win (os/exec uses the last value of a
// duplicate key). They pin the go command settings that change the
// executor's bytes; see go help environment. GOENV=off disables the go
// env file, which would otherwise supply a value for a variable set to the
// empty string here (such as GOFLAGS). GOWORK=off keeps a go.work file
// outside the repository from changing what is built. The build flags are
// given on the command line instead of in GOFLAGS.
//
// The toolchain is not pinned. Because of GOENV=off, a GOTOOLCHAIN set in
// the go env file is ignored; a GOTOOLCHAIN set in the environment is
// honored; otherwise the go command selects the toolchain as usual, from
// go.mod: it switches to the toolchain line of go.mod when the go command
// is older. So the go command that runs the generator and the one that
// builds the executors can differ.
var goSettings = []string{
	"GOENV=off",
	"GOFLAGS=",
	"GOEXPERIMENT=",
	"GOFIPS140=off",
	"GOWORK=off",
}

// platform is one target of the executor.
type platform struct {
	goos   string // always linux
	goarch string // amd64, arm64 or arm
	goarm  string // the ARM version for arm, always 7; empty otherwise
}

// String returns the platform as the manifest records it: linux/amd64,
// linux/arm64, linux/arm/v7.
func (p platform) String() string {
	s := p.goos + "/" + p.goarch
	if p.goarm != "" {
		s += "/v" + p.goarm
	}
	return s
}

// asset returns the name of the uncompressed executor for p, which is its
// release asset name, such as irekey-exec-linux-armv7.
func (p platform) asset() string {
	s := assetPrefix + p.goos + "-" + p.goarch
	if p.goarm != "" {
		s += "v" + p.goarm
	}
	return s
}

// env returns the settings that select p for the go command, without cgo.
// The instruction set level is set for every architecture, so that the
// caller's environment cannot build an executor that older devices cannot
// run: GOAMD64=v1 and GOARM64=v8.0 are the go command's defaults, and
// GOARM=7 is its default for a go command that was not built on an ARM
// system (go help environment).
func (p platform) env() []string {
	env := []string{"CGO_ENABLED=0", "GOOS=" + p.goos, "GOARCH=" + p.goarch}
	switch p.goarch {
	case "amd64":
		env = append(env, "GOAMD64=v1")
	case "arm64":
		env = append(env, "GOARM64=v8.0")
	case "arm":
		env = append(env, "GOARM="+p.goarm)
	}
	return env
}

// parsePlatform parses one -platforms element: linux/amd64, linux/arm64 or
// linux/arm/7 (also written linux/arm/v7), the platforms package execbin
// accepts.
func parsePlatform(s string) (platform, error) {
	parts := strings.Split(s, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return platform{}, fmt.Errorf("platform %q: want linux/amd64, linux/arm64 or linux/arm/7", s)
	}
	p := platform{goos: parts[0], goarch: parts[1]}
	if p.goos != "linux" {
		return platform{}, fmt.Errorf("platform %q: the executor runs on linux only", s)
	}
	switch p.goarch {
	case "amd64", "arm64":
		if len(parts) == 3 {
			return platform{}, fmt.Errorf("platform %q: %s takes no version", s, p.goarch)
		}
	case "arm":
		if len(parts) != 3 || strings.TrimPrefix(parts[2], "v") != "7" {
			return platform{}, fmt.Errorf("platform %q: the supported 32-bit ARM platform is linux/arm/7", s)
		}
		p.goarm = "7"
	default:
		return platform{}, fmt.Errorf("platform %q: unsupported architecture (supported: amd64, arm64, arm)", s)
	}
	return p, nil
}

// parsePlatforms parses the comma-separated -platforms value.
func parsePlatforms(s string) ([]platform, error) {
	var out []platform
	seen := map[string]bool{}
	for f := range strings.SplitSeq(s, ",") {
		p, err := parsePlatform(strings.TrimSpace(f))
		if err != nil {
			return nil, err
		}
		if seen[p.String()] {
			return nil, fmt.Errorf("platform %s is listed twice", p)
		}
		seen[p.String()] = true
		out = append(out, p)
	}
	return out, nil
}

// checkVersion accepts the versions that package execbin accepts in a
// manifest (execbin.ValidVersion).
func checkVersion(v string) error {
	if v == "" {
		return errors.New("-version is required")
	}
	if !execbin.ValidVersion(v) {
		return fmt.Errorf("-version %q: use at most 128 letters, digits and . - + _", v)
	}
	return nil
}

// options are the parsed command line.
type options struct {
	version   string
	out       string
	dist      string // empty: no uncompressed executors
	platforms []platform
}

// newOptions validates the flag values.
func newOptions(version, out, dist, platforms string) (options, error) {
	if err := checkVersion(version); err != nil {
		return options{}, err
	}
	if out == "" {
		return options{}, errors.New("-out is required")
	}
	ps, err := parsePlatforms(platforms)
	if err != nil {
		return options{}, err
	}
	return options{version: version, out: out, dist: dist, platforms: ps}, nil
}

// checkDirs creates the -out and -dist directories and checks that -dist
// is neither -out nor inside it: go:embed takes the whole -out directory,
// so the uncompressed executors would be embedded too. The check compares
// the directories as the operating system resolves them, once they exist,
// so a symbolic link cannot hide one inside the other: a link followed by
// "..", or a link to a directory that did not exist yet.
func checkDirs(o options) error {
	if o.dist == "" {
		return nil
	}
	out, err := realDir(o.out)
	if err != nil {
		return err
	}
	dist, err := realDir(o.dist)
	if err != nil {
		return err
	}
	// Both paths are absolute, so Rel fails only for paths on different
	// volumes, and such a path is not inside out.
	rel, err := filepath.Rel(out, dist)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("-dist %s is the -out directory %s or inside it; use a directory outside -out", o.dist, o.out)
	}
	return nil
}

// realDir creates dir if needed and returns its absolute path with every
// symbolic link resolved. A relative dir is appended to the working
// directory without cleaning it lexically (filepath.Abs would), so that a
// ".." after a symbolic link applies to the link's target, as it does for
// the operating system.
func realDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd + string(filepath.Separator) + dir
	}
	return filepath.EvalSymlinks(dir)
}

// parseArgs parses the command line. Problems are printed to stderr and
// reported as errUsage; -h is reported as flag.ErrHelp.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	fset := flag.NewFlagSet("gen", flag.ContinueOnError)
	fset.SetOutput(stderr)
	fset.Usage = func() {
		_, _ = io.WriteString(fset.Output(), usage)
		fset.PrintDefaults()
	}
	version := fset.String("version", "", "version to build into the executors and record in the manifest (required)")
	out := fset.String("out", "", "directory for the manifest and the compressed executors (required)")
	dist := fset.String("dist", "", "directory for the uncompressed executors under their release asset names (optional)")
	platforms := fset.String("platforms", defaultPlatforms, "comma-separated platforms: linux/amd64, linux/arm64, linux/arm/7")
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, err
		}
		// The flag package has printed the problem and the usage.
		return options{}, errUsage
	}
	if fset.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "gen: unexpected arguments: %s\n", strings.Join(fset.Args(), " "))
		return options{}, errUsage
	}
	o, err := newOptions(*version, *out, *dist, *platforms)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "gen: %v\nRun gen -h to see the options.\n", err)
		return options{}, errUsage
	}
	return o, nil
}

// builder builds the executor for p, with version built in, into the file
// out.
type builder func(ctx context.Context, p platform, version, out string) error

// goBuilder returns a builder that runs the go command found on PATH in the
// module root, which it finds with go env GOMOD. go generate and go run put
// the GOROOT/bin of the go command that started them first on PATH, so that
// go command runs the builds; which toolchain it then uses is described at
// goSettings.
func goBuilder(ctx context.Context) (builder, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("finding the go command: %w", err)
	}
	root, err := moduleRoot(ctx, goBin)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, p platform, version, out string) error {
		// The go command's own temporary files (its work directory) go next
		// to out, inside the generator's temporary directory, so they are
		// removed with it even when the go command is interrupted or killed
		// and cannot remove them itself.
		tmp, err := os.MkdirTemp(filepath.Dir(out), "go-tmp-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		ctx, cancel := context.WithTimeout(ctx, buildTimeout)
		defer cancel()
		cmd := buildCommand(ctx, goBin, root, p, version, out, tmp)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build: %w\n%s", err, bytes.TrimSpace(output.Bytes()))
		}
		return nil
	}, nil
}

// buildCommand returns the go build command for the executor on p, writing
// its temporary files to tmp. It is an argument vector; no shell is
// involved.
func buildCommand(ctx context.Context, goBin, root string, p platform, version, out, tmp string) *exec.Cmd {
	//nolint:gosec // G204: the program is the go command; the arguments are a vector and the version is checked by checkVersion.
	cmd := exec.CommandContext(ctx, goBin, "build", "-trimpath",
		"-ldflags", "-s -w -X "+versionVar+"="+version,
		"-o", out, executorPkg)
	cmd.Dir = root
	cmd.Env = slices.Concat(os.Environ(), goSettings, p.env(), []string{"GOTMPDIR=" + tmp})
	interruptOnCancel(cmd)
	return cmd
}

// interruptOnCancel makes cmd start in a process group of its own and, when
// its context is done, sends an interrupt to that group instead of killing
// only cmd's process (the os/exec default). A plain go build installs no
// interrupt handler (in the go command, only go run, go test and go doc
// -http do), so an interrupt or a kill sent to the go process alone ends it
// at once, and the compiler, assembler or linker processes it started can
// keep running briefly after it. The group interrupt reaches those
// processes too. If the go process has not exited after waitDelay, os/exec
// kills it, and only it. On systems without process groups (Windows), the
// go process is killed. Either way, the go command's work directory is in
// the generator's temporary directory (GOTMPDIR, see goBuilder), which the
// generator removes.
func interruptOnCancel(cmd *exec.Cmd) {
	ownProcessGroup(cmd)
	cmd.Cancel = func() error { return interruptGroup(cmd) }
	cmd.WaitDelay = waitDelay
}

// moduleRoot returns the directory of the main module's go.mod.
func moduleRoot(ctx context.Context, goBin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, envTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "env", "GOMOD")
	cmd.Env = slices.Concat(os.Environ(), goSettings)
	interruptOnCancel(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return rootFromGOMOD(strings.TrimSpace(string(out)))
}

// rootFromGOMOD returns the module root for the output of go env GOMOD.
func rootFromGOMOD(gomod string) (string, error) {
	if gomod == "" || gomod == os.DevNull {
		return "", errors.New("go env GOMOD: not inside a module; run the generator inside the irekey module")
	}
	return filepath.Dir(gomod), nil
}

// compress returns data gzip-compressed at the best compression level, with
// a header that records nothing about the build: no modification time (the
// MTIME field is 0), no file name, comment or extra field, and the default
// operating system byte of compress/gzip (255, unknown). Compressing the
// same bytes with the same Go toolchain therefore gives the same result.
func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	// These are the values NewWriterLevel starts from; they are set here
	// because the output depends on them.
	zw.ModTime = time.Time{}
	zw.Name = ""
	zw.Comment = ""
	zw.Extra = nil
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// digest returns the lowercase hexadecimal SHA-256 digest of data.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// checkSize enforces the executor's size budget on the linux/amd64 build,
// the platform the budget is defined for.
func checkSize(p platform, size int, stderr io.Writer) error {
	if p.goos != "linux" || p.goarch != "amd64" {
		return nil
	}
	switch {
	case size > sizeLimit:
		return fmt.Errorf("the %s executor is %d bytes, above the limit of %d bytes (12 MiB)", p, size, sizeLimit)
	case size > sizeTarget:
		_, _ = fmt.Fprintf(stderr, "gen: warning: the %s executor is %d bytes, above the target of %d bytes (8 MiB)\n",
			p, size, sizeTarget)
	}
	return nil
}

// built is one executor, built and compressed.
type built struct {
	entry execbin.Executor
	asset string // the release asset name
	raw   []byte
	gz    []byte
}

// buildAll builds, checks and compresses the executor for every platform,
// sorted by file name. The builds go to a temporary directory, which is
// removed on every return, including a cancellation by a signal.
func buildAll(ctx context.Context, o options, build builder, stderr io.Writer) ([]built, error) {
	tmp, err := os.MkdirTemp("", "irekey-gen-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	dir, err := os.OpenRoot(tmp)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()

	var out []built
	for _, p := range o.platforms {
		name := p.asset()
		if err := build(ctx, p, o.version, filepath.Join(tmp, name)); err != nil {
			return nil, fmt.Errorf("building the executor for %s: %w", p, err)
		}
		raw, err := dir.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading the executor for %s: %w", p, err)
		}
		if err := checkSize(p, len(raw), stderr); err != nil {
			return nil, err
		}
		gz, err := compress(raw)
		if err != nil {
			return nil, fmt.Errorf("compressing the executor for %s: %w", p, err)
		}
		out = append(out, built{
			entry: execbin.Executor{
				Platform: p.String(),
				File:     name + ".gz",
				SHA256:   digest(raw),
				Size:     int64(len(raw)),
				GzSHA256: digest(gz),
				GzSize:   int64(len(gz)),
			},
			asset: name,
			raw:   raw,
			gz:    gz,
		})
	}
	slices.SortFunc(out, func(a, b built) int { return strings.Compare(a.entry.File, b.entry.File) })
	return out, nil
}

// marshalManifest encodes m as indented JSON with a final newline.
func marshalManifest(m execbin.Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// openDir creates dir if needed and opens it as a root, so that every file
// written stays inside it.
func openDir(dir string) (*os.Root, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return os.OpenRoot(dir)
}

// writeFile writes data to name in dir through a temporary file that is
// flushed to disk and then renamed into place, so name never holds partial
// content. The temporary name starts with a dot, which go:embed leaves out
// of a directory.
func writeFile(dir *os.Root, name string, data []byte, perm fs.FileMode) error {
	tmp := "." + name + ".tmp"
	_ = dir.Remove(tmp) // left by an interrupted run, if any
	f, err := dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err == nil {
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		err = dir.Rename(tmp, name)
	}
	if err != nil {
		_ = dir.Remove(tmp)
		return fmt.Errorf("writing %s: %w", filepath.Join(dir.Name(), name), err)
	}
	return nil
}

// unlisted returns the entries of dir whose names start with assetPrefix
// (irekey-exec-) and that keep does not name, sorted by name. With
// regularOnly, it returns regular files only.
func unlisted(dir *os.Root, keep map[string]bool, regularOnly bool) ([]string, error) {
	entries, err := fs.ReadDir(dir.FS(), ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, assetPrefix) && !keep[name] && (!regularOnly || e.Type().IsRegular()) {
			names = append(names, name)
		}
	}
	return names, nil
}

// removeStale removes the regular files in dir whose names start with
// irekey-exec- and that keep does not name: executors left by an earlier
// run for other platforms. In -out, go:embed would embed them; in -dist,
// they would be published with the release. Every other file is left
// alone (-dist also holds the command line binaries, irekey-<os>-<arch>).
func removeStale(dir *os.Root, keep map[string]bool) error {
	names, err := unlisted(dir, keep, true)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := dir.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

// write stores the executors and the manifest in o.out, and the
// uncompressed executors in o.dist when it is set. The old manifest is
// removed first and the new one written last, so a run that fails halfway
// leaves no manifest (execbin then reports that nothing is embedded) rather
// than one that does not match the files next to it.
func write(o options, m execbin.Manifest, executors []built) error {
	manifest, err := marshalManifest(m)
	if err != nil {
		return err
	}
	out, err := openDir(o.out)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if err := out.Remove(execbin.ManifestName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	keep := map[string]bool{}
	for _, b := range executors {
		if err := writeFile(out, b.entry.File, b.gz, 0o644); err != nil {
			return err
		}
		keep[b.entry.File] = true
	}
	if err := removeStale(out, keep); err != nil {
		return err
	}
	if o.dist != "" {
		dist, err := openDir(o.dist)
		if err != nil {
			return err
		}
		defer func() { _ = dist.Close() }()
		assets := map[string]bool{}
		for _, b := range executors {
			if err := writeFile(dist, b.asset, b.raw, 0o755); err != nil {
				return err
			}
			assets[b.asset] = true
		}
		if err := removeStale(dist, assets); err != nil {
			return err
		}
	}
	return writeFile(out, execbin.ManifestName, manifest, 0o644)
}

// readBack checks what was written the way its users read it: the manifest
// must load from o.out and match the compressed executors; every release
// asset in o.dist must have the digest the manifest records; and neither
// directory may hold an irekey-exec- entry the manifest does not list.
func readBack(o options) error {
	out, err := os.OpenRoot(o.out)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	m, ok, err := execbin.Load(out.FS())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has no manifest after writing it", o.out)
	}
	if err := m.Verify(out.FS()); err != nil {
		return err
	}
	files, assets := map[string]bool{}, map[string]bool{}
	for _, e := range m.Executors {
		files[e.File] = true
		assets[strings.TrimSuffix(e.File, ".gz")] = true
	}
	if err := checkListed(out, o.out, files); err != nil {
		return err
	}
	if o.dist == "" {
		return nil
	}
	dist, err := os.OpenRoot(o.dist)
	if err != nil {
		return err
	}
	defer func() { _ = dist.Close() }()
	for _, e := range m.Executors {
		name := strings.TrimSuffix(e.File, ".gz")
		data, err := dist.ReadFile(name)
		if err != nil {
			return err
		}
		if sum := digest(data); sum != e.SHA256 {
			return fmt.Errorf("%s has SHA-256 %s, the manifest records %s", filepath.Join(o.dist, name), sum, e.SHA256)
		}
	}
	return checkListed(dist, o.dist, assets)
}

// checkListed fails if dir (named name) holds an entry whose name starts
// with irekey-exec- and that listed does not name.
func checkListed(dir *os.Root, name string, listed map[string]bool) error {
	extra, err := unlisted(dir, listed, false)
	if err != nil {
		return err
	}
	if len(extra) > 0 {
		return fmt.Errorf("%s holds %s, which the manifest does not list; remove it", name, strings.Join(extra, ", "))
	}
	return nil
}

// generate builds the executors with build and writes them, then reads
// them back and prints one line per executor to stdout.
func generate(ctx context.Context, o options, build builder, stdout, stderr io.Writer) (execbin.Manifest, error) {
	if err := checkDirs(o); err != nil {
		return execbin.Manifest{}, err
	}
	executors, err := buildAll(ctx, o, build, stderr)
	if err != nil {
		return execbin.Manifest{}, err
	}
	m := execbin.Manifest{Format: execbin.Format, Version: o.version}
	for _, b := range executors {
		m.Executors = append(m.Executors, b.entry)
	}
	if err := write(o, m, executors); err != nil {
		return execbin.Manifest{}, err
	}
	if err := readBack(o); err != nil {
		return execbin.Manifest{}, fmt.Errorf("checking the written files: %w", err)
	}
	for _, e := range m.Executors {
		_, _ = fmt.Fprintf(stdout, "%s %s sha256=%s size=%d gz_sha256=%s gz_size=%d\n",
			e.Platform, e.File, e.SHA256, e.Size, e.GzSHA256, e.GzSize)
	}
	return m, nil
}

// run is the generator without the process around it: it returns the exit
// code. newBuilder returns the builder, so that tests can replace the go
// command.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, newBuilder func(context.Context) (builder, error)) int {
	o, err := parseArgs(args, stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return exitOK
	case err != nil:
		return exitUsage
	}
	build, err := newBuilder(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "gen: %v\n", err)
		return exitFailure
	}
	if _, err := generate(ctx, o, build, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "gen: %v\n", err)
		return exitFailure
	}
	return exitOK
}

func main() {
	// An interrupt or a termination request cancels the context: the process
	// group of the running go build is interrupted (see interruptOnCancel),
	// and run returns after removing the temporary directory, which holds
	// the go command's work directory too.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, goBuilder)
	stop()
	os.Exit(code)
}
