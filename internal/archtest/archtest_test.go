// SPDX-License-Identifier: MIT

package archtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// module is the import path of the irekey module.
const module = "github.com/hitoshyamamoto/irekey"

// executor is the entry point of the device-side executor.
const executor = "cmd/irekey-exec"

// listTimeout bounds all the go commands the tests run together.
const listTimeout = 2 * time.Minute

// listFields are the fields of go list -json that the rules read.
const listFields = "ImportPath,Imports,TestImports,XTestImports,Deps,Error,DepsErrors"

// executorTrees are the packages the executor is built from, with their
// subpackages, relative to the module root.
var executorTrees = []string{executor, "internal/exec"}

// buildConfig is one way of building the module.
type buildConfig struct {
	env         []string // platform and cgo settings
	constraints string   // build constraints to satisfy, passed to -tags
	only        []string // when set, the trees whose packages are kept
}

// platform is a target the module is built for.
type platform struct {
	env  []string
	only []string // when set, only these trees are built for the target
}

// platforms are the targets the rules check. The operator binary runs on
// Linux, Windows and macOS; the executor runs on Linux on amd64, arm64 and
// armv7. go list resolves build constraints for one platform at a time, so
// a file such as pipe_windows.go is seen only in the Windows listing.
var platforms = []platform{
	{env: []string{"GOOS=linux", "GOARCH=amd64"}},
	{env: []string{"GOOS=windows", "GOARCH=amd64"}},
	{env: []string{"GOOS=darwin", "GOARCH=amd64"}},
	{env: []string{"GOOS=linux", "GOARCH=arm64"}, only: executorTrees},
	{env: []string{"GOOS=linux", "GOARCH=arm", "GOARM=7"}, only: executorTrees},
}

// variants are listed for every platform: the release build, without cgo,
// and a build with cgo enabled and every test build constraint set. With cgo
// disabled, go list leaves out the files that import "C"; without the
// constraints, it leaves out the e2e, integration and fault injection files.
var variants = []buildConfig{
	{env: []string{"CGO_ENABLED=0"}},
	{env: []string{"CGO_ENABLED=1"}, constraints: "e2e,integration,faultinject"},
}

// buildConfigs returns every variant on every platform. Their listings are
// merged, so a rule sees a file when at least one configuration compiles
// it. Files that no configuration compiles are not checked: files for other
// operating systems or architectures, and files behind other build
// constraints (for example "ignore", or "!cgo" together with a test
// constraint).
func buildConfigs() []buildConfig {
	var out []buildConfig
	for _, p := range platforms {
		for _, v := range variants {
			out = append(out, buildConfig{
				env:         slices.Concat(p.env, v.env),
				constraints: v.constraints,
				only:        p.only,
			})
		}
	}
	return out
}

// keep reports whether a package listed under c is merged into the listing.
func (c buildConfig) keep(path string) bool {
	return c.only == nil || withinAny(path, mods(c.only))
}

// commonEnv applies to every go command the tests run. GOWORK=off keeps a
// go.work file outside the repository from changing what the module
// contains.
var commonEnv = []string{"GOWORK=off"}

// pkg is the part of a go list -json record that the rules read.
type pkg struct {
	ImportPath   string
	Imports      []string    // imports of the non-test files
	TestImports  []string    // imports of the package's own _test.go files
	XTestImports []string    // imports of its external _test package
	Deps         []string    // every package the non-test build depends on
	Error        *pkgError   // the package could not be loaded
	DepsErrors   []*pkgError // a dependency could not be loaded
}

// pkgError is the part of a go list error that the tests print.
type pkgError struct {
	Err string
}

// listing maps import paths to the packages of the module.
type listing map[string]*pkg

// errNoGo reports that the go command is not available.
var errNoGo = errors.New("go command not found")

// loadModule lists the module's packages once per test binary.
var loadModule = sync.OnceValues(func() (listing, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	return listModule(ctx)
})

// load returns the listing of the module, or skips the test when the go
// command is not available.
func load(t *testing.T) listing {
	t.Helper()
	l, err := loadModule()
	if errors.Is(err, errNoGo) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// listModule runs go list on every package of the module, once per build
// configuration, and merges the results. The pattern is always ./..., which
// leaves out the packages that have no file for the platform; a narrower
// pattern that matches nothing would come back as a record named after the
// pattern. A package that imports one with no file for a platform cannot be
// loaded there and fails the listing, as it would fail to build: constrain
// the importing file to the platforms of the imported package.
func listModule(ctx context.Context) (listing, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoGo, err)
	}
	out, err := runGo(ctx, goBin, "", nil, "env", "GOMOD")
	if err != nil {
		return nil, err
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return nil, errors.New("go env GOMOD: the tests are not running inside a module")
	}
	l := listing{}
	for _, c := range buildConfigs() {
		args := []string{"list", "-e", "-json=" + listFields}
		if c.constraints != "" {
			args = append(args, "-tags", c.constraints)
		}
		args = append(args, "./...")
		out, err := runGo(ctx, goBin, filepath.Dir(gomod), c.env, args...)
		if err != nil {
			return nil, err
		}
		where := strings.Join(c.env, " ") + " go " + strings.Join(args, " ")
		all, err := decodeList(out)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		var pkgs []*pkg
		for _, p := range all {
			if !within(p.ImportPath, module) {
				return nil, fmt.Errorf("%s: printed %s, which is outside the module %s", where, p.ImportPath, module)
			}
			if c.keep(p.ImportPath) {
				pkgs = append(pkgs, p)
			}
		}
		if err := loadErrors(pkgs); err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		l.add(pkgs...)
	}
	if len(l) == 0 {
		return nil, errors.New("go list printed no packages")
	}
	return l, nil
}

// runGo runs the go command in dir and returns its standard output.
func runGo(ctx context.Context, goBin, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), commonEnv...), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

// decodeList decodes the stream of JSON objects printed by go list -json.
func decodeList(data []byte) ([]*pkg, error) {
	var pkgs []*pkg
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var p pkg
		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return pkgs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decoding the output of go list: %w", err)
		}
		pkgs = append(pkgs, &p)
	}
}

// loadErrors fails when go list reports a package that it could not load,
// because the imports of such a package are incomplete.
func loadErrors(pkgs []*pkg) error {
	var problems []string
	for _, p := range pkgs {
		if p.Error != nil {
			problems = append(problems, p.ImportPath+": "+p.Error.Err)
		}
		for _, e := range p.DepsErrors {
			problems = append(problems, p.ImportPath+": dependency: "+e.Err)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("packages that cannot be loaded:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

// add merges packages into the listing. A package listed by several build
// configurations keeps the union of its imports and dependencies.
func (l listing) add(pkgs ...*pkg) {
	for _, p := range pkgs {
		q, ok := l[p.ImportPath]
		if !ok {
			q = &pkg{ImportPath: p.ImportPath}
			l[p.ImportPath] = q
		}
		q.Imports = union(q.Imports, p.Imports)
		q.TestImports = union(q.TestImports, p.TestImports)
		q.XTestImports = union(q.XTestImports, p.XTestImports)
		q.Deps = union(q.Deps, p.Deps)
	}
}

// sorted returns the packages of the listing in import path order.
func (l listing) sorted() []*pkg {
	var pkgs []*pkg
	for _, path := range slices.Sorted(maps.Keys(l)) {
		pkgs = append(pkgs, l[path])
	}
	return pkgs
}

// missing returns the packages, given relative to the module root, that are
// not in the listing.
func (l listing) missing(rels []string) []string {
	var out []string
	for _, rel := range rels {
		if l[mod(rel)] == nil {
			out = append(out, rel)
		}
	}
	return out
}

// via returns " (via a -> b -> c)", the shortest chain of non-test imports
// from one package to another through packages of the module, or "" when the
// listing has no such chain.
func (l listing) via(from, to string) string {
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		p := l[cur]
		if p == nil {
			continue
		}
		for _, imp := range p.Imports {
			if _, seen := prev[imp]; seen {
				continue
			}
			prev[imp] = cur
			if imp != to {
				queue = append(queue, imp)
				continue
			}
			var chain []string
			for at := to; at != ""; at = prev[at] {
				chain = append(chain, short(at))
			}
			slices.Reverse(chain)
			return " (via " + strings.Join(chain, " -> ") + ")"
		}
	}
	return ""
}

// union returns the sorted set of the elements of a and b.
func union(a, b []string) []string {
	s := slices.Concat(a, b)
	slices.Sort(s)
	return slices.Compact(s)
}

// mod returns the import path of a package of the module given relative to
// the module root.
func mod(rel string) string {
	return module + "/" + rel
}

// mods applies mod to every element of rels.
func mods(rels []string) []string {
	out := make([]string, len(rels))
	for i, rel := range rels {
		out[i] = mod(rel)
	}
	return out
}

// short returns path relative to the module root when it is inside the
// module, so that messages stay readable.
func short(path string) string {
	if rel, ok := strings.CutPrefix(path, module+"/"); ok {
		return rel
	}
	return path
}

// within reports whether path is root or one of its subpackages.
func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

// withinAny reports whether path is within one of roots.
func withinAny(path string, roots []string) bool {
	return slices.ContainsFunc(roots, func(root string) bool { return within(path, root) })
}

// names joins names for a message: "a", "a and b", "a, b and c".
func names(list []string) string {
	if len(list) < 2 {
		return strings.Join(list, "")
	}
	return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
}

// standard reports whether path belongs to the standard library, that is,
// whether its first element has no dot. Standard library packages vendored
// under vendor/ and the cgo pseudo-package "C" count as standard.
func standard(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// directImports returns the imports of the non-test files of p.
func directImports(p *pkg) []string {
	return p.Imports
}

// allImports returns the imports of the non-test and test files of p,
// except p itself, which its external test package imports.
func allImports(p *pkg) []string {
	all := union(union(p.Imports, p.TestImports), p.XTestImports)
	return slices.DeleteFunc(all, func(imp string) bool { return imp == p.ImportPath })
}

// importersOutside reports every package outside the allowed trees that
// imports target or one of its subpackages, following edges. The trees are
// given relative to the module root; the target's own tree is always allowed.
func (l listing) importersOutside(edges func(*pkg) []string, target string, allowed ...string) []string {
	allow := mods(append([]string{target}, allowed...))
	var out []string
	for _, p := range l.sorted() {
		if withinAny(p.ImportPath, allow) {
			continue
		}
		for _, imp := range edges(p) {
			if within(imp, mod(target)) {
				out = append(out, fmt.Sprintf("%s imports %s; only %s may import %s",
					short(p.ImportPath), short(imp), names(allowed), target))
			}
		}
	}
	return out
}

// importsInto reports every import, following edges, from the tree of from
// into one of the trees of targets, all given relative to the module root.
// why is appended to each message.
func (l listing) importsInto(edges func(*pkg) []string, from, why string, targets ...string) []string {
	var out []string
	for _, p := range l.sorted() {
		if !within(p.ImportPath, mod(from)) {
			continue
		}
		for _, imp := range edges(p) {
			if withinAny(imp, mods(targets)) {
				out = append(out, fmt.Sprintf("%s imports %s; %s", short(p.ImportPath), short(imp), why))
			}
		}
	}
	return out
}

// rule is one part of the package dependency rule.
type rule struct {
	// needs lists the packages the rule names, relative to the module root.
	// They must exist, or the rule could pass without checking anything.
	needs []string
	// check returns one message per violation.
	check func(listing) []string
}

// run checks a rule against the module.
func run(t *testing.T, r rule) {
	t.Helper()
	l := load(t)
	if missing := l.missing(r.needs); len(missing) > 0 {
		t.Fatalf("the rule names packages that do not exist, so it would check nothing: %s",
			strings.Join(missing, ", "))
	}
	for _, msg := range r.check(l) {
		t.Error(msg)
	}
}

// cliRule: nothing imports the command-line interface except the entry
// point.
var cliRule = rule{
	needs: []string{"cmd/irekey", "internal/cli"},
	check: func(l listing) []string {
		return l.importersOutside(allImports, "internal/cli", "cmd/irekey")
	},
}

// uiRule: only the entry point and the command-line interface import the web
// UI, to start its server.
var uiRule = rule{
	needs: []string{"cmd/irekey", "internal/cli", "internal/ui"},
	check: func(l listing) []string {
		return l.importersOutside(allImports, "internal/ui", "cmd/irekey", "internal/cli")
	},
}

// appRule: the service layer serves the command-line interface and the web
// UI, and depends on neither. The entry point may import it too, since its
// job is to wire the packages together.
var appRule = rule{
	needs: []string{"cmd/irekey", "internal/app", "internal/cli", "internal/ui"},
	check: func(l listing) []string {
		return slices.Concat(
			l.importersOutside(allImports, "internal/app", "cmd/irekey", "internal/cli", "internal/ui"),
			l.importsInto(allImports, "internal/app", "the service layer does not depend on its callers",
				"internal/cli", "internal/ui"),
		)
	},
}

// uiImportsRule: the web UI goes through the service layer. Only direct,
// non-test imports are checked: package app legitimately depends on the
// transport, so the UI's transitive dependencies always include it.
var uiImportsRule = rule{
	needs: []string{"internal/ui", "internal/transport", "internal/transfer", "internal/rollout"},
	check: func(l listing) []string {
		return l.importsInto(directImports, "internal/ui", "the web UI calls package app instead",
			"internal/transport", "internal/transfer", "internal/rollout")
	},
}

// layeringRule: delivery goes one way, rollout uses transfer, which uses the
// transport.
var layeringRule = rule{
	needs: []string{"internal/transport", "internal/transfer", "internal/rollout"},
	check: func(l listing) []string {
		return slices.Concat(
			l.importsInto(allImports, "internal/transport", "the transport is below transfer and rollout",
				"internal/transfer", "internal/rollout"),
			l.importsInto(allImports, "internal/transfer", "transfer is below rollout", "internal/rollout"),
		)
	},
}

// executorForbidden lists the packages, with their subpackages, that the
// executor must not depend on.
var executorForbidden = []string{
	// Operator-side packages.
	"internal/app", "internal/cli", "internal/ui", "internal/vault",
	"internal/forge", "internal/state", "internal/deployrepo",
	// The executor never listens on a port and opens no connection to other
	// hosts. The transport and transfer open and use connections to devices,
	// rollout drives them, and execbin exists only to upload the executor
	// over them, so none of them belongs in the executor. Whether local
	// health checks may connect to the device's own services on loopback is
	// an open question (see docs/open-questions.md), so the rule does not ban
	// the net packages.
	"internal/execbin", "internal/transport", "internal/transfer", "internal/rollout",
}

// executorAllowlist lists the packages, with their subpackages, outside the
// module and the standard library that the executor may depend on. Adding
// one is a decision: it grows the executor binary that every device runs.
var executorAllowlist = []string{}

// executorRule: the executor depends on no operator-side package and on
// nothing outside the module and the standard library except the allowlist.
var executorRule = rule{
	needs: slices.Concat(executorTrees, executorForbidden),
	check: func(l listing) []string {
		return l.checkExecutor(executorAllowlist)
	},
}

// checkExecutor checks the transitive dependencies of the executor's entry
// point and of every package under internal/exec, so that a package the
// executor will be built from is checked before the entry point imports it.
func (l listing) checkExecutor(allow []string) []string {
	if l[mod(executor)] == nil {
		return []string{executor + " is not in the listing"}
	}
	var out []string
	for _, p := range l.sorted() {
		if !withinAny(p.ImportPath, mods(executorTrees)) {
			continue
		}
		for _, dep := range p.Deps {
			switch {
			case withinAny(dep, mods(executorForbidden)):
				out = append(out, fmt.Sprintf("%s depends on %s%s, which the executor must not depend on",
					short(p.ImportPath), short(dep), l.via(p.ImportPath, dep)))
			case !standard(dep) && !within(dep, module) && !withinAny(dep, allow):
				out = append(out, fmt.Sprintf("%s depends on %s%s, which is outside the standard library and not in the executor allowlist",
					short(p.ImportPath), dep, l.via(p.ImportPath, dep)))
			}
		}
	}
	return out
}

// purePackages perform no I/O.
var purePackages = []string{
	"internal/model", "internal/layers", "internal/variant", "internal/graph",
	"internal/schema", "internal/errs", "internal/redact",
}

// ioPackages are the standard library packages a pure package must not
// import; ioTrees are forbidden together with their subpackages, except the
// packages in pureAllowed, which only parse and format values (net/netip
// handles IP addresses as values and opens nothing).
//
// Beyond the obvious ones: path/filepath walks and globs the file system
// (pure packages use path for slash-separated paths); log and log/syslog
// write to the process's output or to the system log; crypto/rand reads the
// operating system's randomness, which also makes results non-deterministic.
// log/slog is allowed: it only defines records and handlers, and a handler
// writes to the io.Writer it is given (the redaction package wraps one).
var (
	ioPackages = []string{
		"os", "os/exec", "os/signal", "os/user", "syscall", "unsafe",
		"plugin", "runtime/cgo", "C", "io/ioutil", "path/filepath", "crypto/rand",
		"log", "log/syslog",
	}
	ioTrees     = []string{"net"}
	pureAllowed = []string{"net/netip"}
)

// pureRule: pure packages import no I/O package and, inside the module, only
// other pure packages. Only their direct, non-test imports are checked: fmt
// and most of the standard library depend on os, and their tests may read
// files from testdata.
var pureRule = rule{
	needs: purePackages,
	check: func(l listing) []string {
		var out []string
		for _, rel := range purePackages {
			p := l[mod(rel)]
			if p == nil {
				continue
			}
			for _, imp := range p.Imports {
				switch {
				case slices.Contains(pureAllowed, imp):
				case slices.Contains(ioPackages, imp) || withinAny(imp, ioTrees):
					out = append(out, fmt.Sprintf("pure package %s imports %s; pure packages perform no I/O", rel, imp))
				case within(imp, module) && !slices.Contains(mods(purePackages), imp):
					out = append(out, fmt.Sprintf("pure package %s imports %s, which is not a pure package", rel, short(imp)))
				}
			}
		}
		return out
	},
}

// unsafeRule: no package imports unsafe or C, in its code or its tests.
var unsafeRule = rule{
	check: func(l listing) []string {
		var out []string
		for _, p := range l.sorted() {
			for _, imp := range allImports(p) {
				if imp == "unsafe" || imp == "C" {
					out = append(out, fmt.Sprintf("%s imports %s; irekey uses neither unsafe nor cgo", short(p.ImportPath), imp))
				}
			}
		}
		return out
	},
}

func TestOnlyTheEntryPointImportsCLI(t *testing.T) { run(t, cliRule) }

func TestOnlyTheEntryPointAndCLIImportUI(t *testing.T) { run(t, uiRule) }

func TestOnlyTheEntryPointCLIAndUIImportApp(t *testing.T) { run(t, appRule) }

func TestUIDoesNotImportTransportTransferOrRollout(t *testing.T) { run(t, uiImportsRule) }

func TestDeliveryLayersImportDownwardOnly(t *testing.T) { run(t, layeringRule) }

func TestExecutorHasNoOperatorSideDependencies(t *testing.T) { run(t, executorRule) }

func TestPurePackagesImportNoIOPackages(t *testing.T) { run(t, pureRule) }

func TestNoUnsafeOrCgo(t *testing.T) { run(t, unsafeRule) }
