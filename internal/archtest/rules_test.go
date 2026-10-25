// SPDX-License-Identifier: MIT

package archtest

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// spec describes one package of a fake listing. Paths that start with cmd/
// or internal/ are relative to the module root; other paths are used as is.
type spec struct {
	path                         string
	imports, tests, xtests, deps []string
}

func resolve(path string) string {
	if strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/") {
		return mod(path)
	}
	return path
}

func resolveAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = resolve(p)
	}
	return out
}

// fake builds a listing. Several specs for the same path are merged, as the
// listings of several build configurations are.
func fake(specs ...spec) listing {
	l := listing{}
	for _, s := range specs {
		l.add(&pkg{
			ImportPath:   resolve(s.path),
			Imports:      resolveAll(s.imports),
			TestImports:  resolveAll(s.tests),
			XTestImports: resolveAll(s.xtests),
			Deps:         resolveAll(s.deps),
		})
	}
	return l
}

// cleanSpecs describe a module that follows every rule, including the cases
// the rules deliberately allow.
func cleanSpecs() []spec {
	return []spec{
		// The entry point wires the packages together.
		{path: "cmd/irekey", imports: []string{"internal/app", "internal/cli", "internal/ui", "os"}, tests: []string{"testing"}},
		{path: "cmd/irekey-exec", imports: []string{"internal/exec", "os"}, deps: []string{
			"internal/bundle", "internal/exec", "internal/exec/journal", "internal/model", "os",
			"vendor/golang.org/x/net/dns/dnsmessage",
		}},
		// An external test package imports the package it tests.
		{path: "internal/cli", imports: []string{"internal/app", "internal/ui"}, xtests: []string{"internal/cli", "testing"}},
		// Only the UI's non-test imports are restricted.
		{path: "internal/ui", imports: []string{"internal/app", "internal/ui/static", "net/http"}, tests: []string{"internal/transport", "testing"}},
		{path: "internal/ui/static", imports: []string{"embed"}},
		{path: "internal/app", imports: []string{
			"internal/deployrepo", "internal/execbin", "internal/forge", "internal/layers",
			"internal/rollout", "internal/state", "internal/vault",
		}},
		{path: "internal/rollout", imports: []string{"internal/transfer"}},
		{path: "internal/transfer", imports: []string{"internal/transport"}},
		{path: "internal/transport"},
		{path: "internal/transport/ssh", imports: []string{"internal/transport", "net"}},
		{path: "internal/deployrepo", imports: []string{"internal/model", "internal/schema", "os"}},
		{path: "internal/vault"},
		{path: "internal/forge"},
		{path: "internal/state"},
		{path: "internal/execbin", imports: []string{"embed"}},
		{path: "internal/exec", imports: []string{"internal/bundle", "internal/exec/journal"}, deps: []string{
			"internal/bundle", "internal/exec/journal", "internal/model", "os",
		}},
		{path: "internal/exec/journal", imports: []string{"os"}, deps: []string{"os"}},
		{path: "internal/bundle", imports: []string{"internal/model"}},
		{path: "internal/model", imports: []string{"fmt", "strings"}},
		// Pure packages handle slash-separated paths with package path.
		{path: "internal/layers", imports: []string{"internal/model", "maps", "path"}},
		{path: "internal/variant", imports: []string{"crypto/sha256", "encoding/json"}},
		{path: "internal/graph", imports: []string{"internal/model"}},
		// Tests of a pure package may read files.
		{path: "internal/schema", imports: []string{"slices"}, tests: []string{"flag", "os", "testing"}},
		{path: "internal/errs", imports: []string{"errors", "fmt"}},
		{path: "internal/redact", imports: []string{"bytes", "io"}},
	}
}

// badSpecs add one or more violations of every rule to cleanSpecs.
func badSpecs() []spec {
	return []spec{
		{path: "internal/vault", imports: []string{"internal/cli"}},
		{path: "internal/state", tests: []string{"internal/app"}},
		{path: "internal/app", imports: []string{"internal/ui/static"}},
		{path: "internal/ui/static", imports: []string{"internal/transport/ssh"}},
		{path: "internal/transport/ssh", xtests: []string{"internal/transfer"}},
		{path: "internal/transfer", imports: []string{"internal/rollout"}},
		{path: "internal/exec", imports: []string{"internal/vault"}, deps: []string{"internal/vault"}},
		{path: "internal/exec/journal", imports: []string{"example.org/other"}, deps: []string{"example.org/other"}},
		{path: "cmd/irekey-exec", deps: []string{"internal/vault", "internal/vault/local", "example.org/other"}},
		// A package under internal/exec that the entry point does not import
		// yet is checked too.
		{path: "internal/exec/health", imports: []string{"internal/transport"}, deps: []string{"internal/transport", "net"}},
		{path: "internal/model", imports: []string{"os", "net/netip", "net/http"}},
		{path: "internal/layers", imports: []string{"path/filepath"}},
		{path: "internal/variant", imports: []string{"internal/deployrepo", "crypto/rand"}},
		{path: "internal/graph", imports: []string{"log", "log/slog"}},
		{path: "internal/redact", imports: []string{"C"}},
		{path: "internal/errs", xtests: []string{"unsafe"}},
	}
}

// allRules lists every rule with the name of its test.
var allRules = []struct {
	name string
	rule rule
}{
	{"TestOnlyTheEntryPointImportsCLI", cliRule},
	{"TestOnlyTheEntryPointAndCLIImportUI", uiRule},
	{"TestOnlyTheEntryPointCLIAndUIImportApp", appRule},
	{"TestUIDoesNotImportTransportTransferOrRollout", uiImportsRule},
	{"TestDeliveryLayersImportDownwardOnly", layeringRule},
	{"TestExecutorHasNoOperatorSideDependencies", executorRule},
	{"TestPurePackagesImportNoIOPackages", pureRule},
	{"TestNoUnsafeOrCgo", unsafeRule},
}

func TestRulesAcceptACleanModule(t *testing.T) {
	l := fake(cleanSpecs()...)
	for _, r := range allRules {
		if missing := l.missing(r.rule.needs); len(missing) > 0 {
			t.Errorf("%s: the clean listing lacks %q", r.name, missing)
		}
		if got := r.rule.check(l); len(got) > 0 {
			t.Errorf("%s: the clean listing has violations:\n%s", r.name, strings.Join(got, "\n"))
		}
	}
}

func TestExecutorAllowlist(t *testing.T) {
	l := fake(append(cleanSpecs(), spec{path: executor, deps: []string{"example.org/allowed/sub"}})...)
	if got := l.checkExecutor([]string{"example.org/allowed"}); len(got) > 0 {
		t.Errorf("an allowlisted dependency of the executor is reported:\n%s", strings.Join(got, "\n"))
	}
	want := []string{"cmd/irekey-exec depends on example.org/allowed/sub, which is outside the standard library and not in the executor allowlist"}
	if got := l.checkExecutor(nil); !slices.Equal(got, want) {
		t.Errorf("checkExecutor(nil) = %q, want %q", got, want)
	}
}

func TestRulesReportEveryViolation(t *testing.T) {
	l := fake(append(cleanSpecs(), badSpecs()...)...)
	want := map[string][]string{
		"TestOnlyTheEntryPointImportsCLI": {
			"internal/vault imports internal/cli; only cmd/irekey may import internal/cli",
		},
		"TestOnlyTheEntryPointAndCLIImportUI": {
			"internal/app imports internal/ui/static; only cmd/irekey and internal/cli may import internal/ui",
		},
		"TestOnlyTheEntryPointCLIAndUIImportApp": {
			"internal/state imports internal/app; only cmd/irekey, internal/cli and internal/ui may import internal/app",
			"internal/app imports internal/ui/static; the service layer does not depend on its callers",
		},
		"TestUIDoesNotImportTransportTransferOrRollout": {
			"internal/ui/static imports internal/transport/ssh; the web UI calls package app instead",
		},
		"TestDeliveryLayersImportDownwardOnly": {
			"internal/transport/ssh imports internal/transfer; the transport is below transfer and rollout",
			"internal/transfer imports internal/rollout; transfer is below rollout",
		},
		"TestExecutorHasNoOperatorSideDependencies": {
			"cmd/irekey-exec depends on example.org/other (via cmd/irekey-exec -> internal/exec -> internal/exec/journal -> example.org/other), which is outside the standard library and not in the executor allowlist",
			"cmd/irekey-exec depends on internal/vault (via cmd/irekey-exec -> internal/exec -> internal/vault), which the executor must not depend on",
			"cmd/irekey-exec depends on internal/vault/local, which the executor must not depend on",
			"internal/exec depends on internal/vault (via internal/exec -> internal/vault), which the executor must not depend on",
			"internal/exec/health depends on internal/transport (via internal/exec/health -> internal/transport), which the executor must not depend on",
			"internal/exec/journal depends on example.org/other (via internal/exec/journal -> example.org/other), which is outside the standard library and not in the executor allowlist",
		},
		"TestPurePackagesImportNoIOPackages": {
			"pure package internal/model imports net/http; pure packages perform no I/O",
			"pure package internal/model imports os; pure packages perform no I/O",
			"pure package internal/layers imports path/filepath; pure packages perform no I/O",
			"pure package internal/variant imports crypto/rand; pure packages perform no I/O",
			"pure package internal/variant imports internal/deployrepo, which is not a pure package",
			"pure package internal/graph imports log; pure packages perform no I/O",
			"pure package internal/redact imports C; pure packages perform no I/O",
		},
		"TestNoUnsafeOrCgo": {
			"internal/errs imports unsafe; irekey uses neither unsafe nor cgo",
			"internal/redact imports C; irekey uses neither unsafe nor cgo",
		},
	}
	for _, r := range allRules {
		got := r.rule.check(l)
		if !slices.Equal(got, want[r.name]) {
			t.Errorf("%s reports\n%s\nwant\n%s", r.name, strings.Join(got, "\n"), strings.Join(want[r.name], "\n"))
		}
	}
}

func TestMissingPackagesAreReported(t *testing.T) {
	var specs []spec
	for _, s := range cleanSpecs() {
		if s.path != "internal/cli" && s.path != executor {
			specs = append(specs, s)
		}
	}
	l := fake(specs...)
	if got, want := l.missing(uiRule.needs), []string{"internal/cli"}; !slices.Equal(got, want) {
		t.Errorf("missing(%q) = %q, want %q", uiRule.needs, got, want)
	}
	// The executor rule also names the packages it forbids.
	if got, want := l.missing(executorRule.needs), []string{executor, "internal/cli"}; !slices.Equal(got, want) {
		t.Errorf("missing(executor rule) = %q, want %q", got, want)
	}
	if got, want := l.checkExecutor(nil), []string{executor + " is not in the listing"}; !slices.Equal(got, want) {
		t.Errorf("checkExecutor without the executor = %q, want %q", got, want)
	}
}

func TestEveryRuleNamesItsPackages(t *testing.T) {
	// Every rule that names packages lists them, so that it cannot pass
	// vacuously; only the rule about unsafe and cgo applies to every
	// package and names none.
	for _, r := range allRules {
		if r.rule.check == nil {
			t.Errorf("%s has no check", r.name)
		}
		if len(r.rule.needs) == 0 && r.name != "TestNoUnsafeOrCgo" {
			t.Errorf("%s names no packages", r.name)
		}
	}
}

func TestDecodeList(t *testing.T) {
	pkgs, err := decodeList([]byte(`{"ImportPath":"a"}
{"ImportPath":"b","Imports":["a"],"Deps":["a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 || pkgs[0].ImportPath != "a" || pkgs[1].ImportPath != "b" || !slices.Equal(pkgs[1].Imports, []string{"a"}) {
		t.Errorf("decodeList = %+v, %+v", pkgs[0], pkgs[1])
	}

	if pkgs, err := decodeList(nil); err != nil || len(pkgs) != 0 {
		t.Errorf("decodeList(nil) = %v, %v; want no packages and no error", pkgs, err)
	}
	if _, err := decodeList([]byte(`{"ImportPath":`)); err == nil || !strings.Contains(err.Error(), "decoding") {
		t.Errorf("decodeList of malformed input: error = %v, want a decoding error", err)
	}
}

func TestLoadErrors(t *testing.T) {
	if err := loadErrors([]*pkg{{ImportPath: "a"}}); err != nil {
		t.Errorf("loadErrors of a clean package = %v", err)
	}
	for _, tt := range []struct {
		name string
		p    *pkg
		want string
	}{
		{"package error", &pkg{ImportPath: "a", Error: &pkgError{Err: "boom"}}, "a: boom"},
		{"dependency error", &pkg{ImportPath: "a", DepsErrors: []*pkgError{{Err: "gone"}}}, "a: dependency: gone"},
	} {
		if err := loadErrors([]*pkg{tt.p}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: loadErrors = %v, want an error containing %q", tt.name, err, tt.want)
		}
	}
}

func TestBuildConfigsCoverEveryPlatform(t *testing.T) {
	configs := buildConfigs()
	if len(configs) != len(platforms)*len(variants) {
		t.Fatalf("%d configurations, want %d", len(configs), len(platforms)*len(variants))
	}
	seen := map[string]bool{}
	for _, c := range configs {
		key := strings.Join(c.env, " ")
		seen[key] = true
		// Only the executor-only configurations drop the operator packages.
		if got, want := c.keep(mod("internal/ui")), c.only == nil; got != want {
			t.Errorf("%s: keep(internal/ui) = %v, want %v", key, got, want)
		}
		if !c.keep(mod(executor)) || !c.keep(mod("internal/exec/journal")) {
			t.Errorf("%s does not keep the executor packages", key)
		}
	}
	for _, want := range []string{
		"GOOS=linux GOARCH=amd64 CGO_ENABLED=0",
		"GOOS=linux GOARCH=amd64 CGO_ENABLED=1",
		"GOOS=windows GOARCH=amd64 CGO_ENABLED=0",
		"GOOS=windows GOARCH=amd64 CGO_ENABLED=1",
		"GOOS=darwin GOARCH=amd64 CGO_ENABLED=0",
		"GOOS=darwin GOARCH=amd64 CGO_ENABLED=1",
		"GOOS=linux GOARCH=arm64 CGO_ENABLED=0",
		"GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1",
	} {
		if !seen[want] {
			t.Errorf("no configuration %s", want)
		}
	}
}

func TestNames(t *testing.T) {
	for _, tt := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
	} {
		if got := names(tt.in); got != tt.want {
			t.Errorf("names(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestListingMergesBuildConfigurations(t *testing.T) {
	l := listing{}
	l.add(&pkg{ImportPath: "a", Imports: []string{"os", "fmt"}, Deps: []string{"os"}})
	l.add(&pkg{ImportPath: "a", Imports: []string{"C", "fmt"}, TestImports: []string{"testing"}, XTestImports: []string{"a"}})
	p := l["a"]
	if want := []string{"C", "fmt", "os"}; !slices.Equal(p.Imports, want) {
		t.Errorf("Imports = %q, want %q", p.Imports, want)
	}
	if want := []string{"C", "fmt", "os", "testing"}; !slices.Equal(allImports(p), want) {
		t.Errorf("allImports = %q, want %q", allImports(p), want)
	}
	if want := []string{"os"}; !slices.Equal(p.Deps, want) {
		t.Errorf("Deps = %q, want %q", p.Deps, want)
	}
}

func TestPathHelpers(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"os", true},
		{"net/http", true},
		{"vendor/golang.org/x/net/dns/dnsmessage", true},
		{"C", true},
		{"golang.org/x/sys/unix", false},
		{"example.org/a", false},
		{module + "/internal/model", false},
	} {
		if got := standard(tt.path); got != tt.want {
			t.Errorf("standard(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
	for _, tt := range []struct {
		path, root string
		want       bool
	}{
		{"internal/ui", "internal/ui", true},
		{"internal/ui/static", "internal/ui", true},
		{"internal/uikit", "internal/ui", false},
		{"internal", "internal/ui", false},
	} {
		if got := within(tt.path, tt.root); got != tt.want {
			t.Errorf("within(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.want)
		}
	}
	if got := short("os"); got != "os" {
		t.Errorf(`short("os") = %q`, got)
	}
	if got := short(mod("internal/ui")); got != "internal/ui" {
		t.Errorf("short(mod(internal/ui)) = %q", got)
	}
}

func TestRunGoReportsTheErrorOutput(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not found")
	}
	_, err = runGo(context.Background(), goBin, "", nil, "no-such-command")
	if err == nil || !strings.Contains(err.Error(), "no-such-command") {
		t.Errorf("runGo error = %v, want one naming the unknown command", err)
	}
}
