// SPDX-License-Identifier: MIT

package errs_test

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hitoshyamamoto/irekey/internal/errs"
	"github.com/hitoshyamamoto/irekey/internal/errs/internal/gen"
)

const (
	registryFile  = "codes.yaml"
	generatedFile = "zz_generated_codes.go"
	docsFile      = "../../docs/errors.md"
	specFile      = "../../docs/spec/masterprompt.md"
)

// emDash is how the specification's tables write "none".
const emDash = "\u2014"

// additionADR is the decision record that added the codes that the
// specification's seed table did not have.
const additionADR = "0019"

// additions are the codes added by additionADR.
var additions = []errs.Code{errs.USR005, errs.USR006, errs.LOC004, errs.TIME002}

func loadRegistry(t *testing.T) *gen.Registry {
	t.Helper()
	reg, err := gen.Load(registryFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Validate(reg); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestGeneratedFilesAreFresh(t *testing.T) {
	reg := loadRegistry(t)
	src, err := gen.RenderGo(reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		path string
		want []byte
	}{{generatedFile, src}, {docsFile, gen.RenderMarkdown(reg)}} {
		got, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("%v (run go generate ./internal/errs)", err)
		}
		if !bytes.Equal(got, f.want) {
			t.Errorf("%s is out of date with %s: run go generate ./internal/errs", f.path, registryFile)
		}
	}
}

// specRow is one row of the specification's table of seed error codes.
type specRow struct {
	code      errs.Code
	condition string // with invariants named, and without the addition marker
	exit      string // as in codes.yaml
	state     string // as in codes.yaml
	addedBy   string // the ADR that added the code, or ""
}

var (
	invariantRow = regexp.MustCompile("^\\| \\*\\*I(\\d+) [^*|]+\\*\\* \\| `([A-Za-z]+)` \\|")
	invariantRef = regexp.MustCompile(`\bI(\d+)\b`)
	categoryRow  = regexp.MustCompile(`^\| ([A-Z]{2,5}) \| ([^|]+) \| ([^|]+) \|$`)
	codeRow      = regexp.MustCompile(`^\| ([A-Z]{2,5}-\d{3}) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|$`)
	additionMark = regexp.MustCompile(`^(.+) \(addition, ADR (\d{4})\)$`)
)

// specSection returns the lines of the section of the specification whose
// heading, at the given level ("##" or "###"), contains title.
func specSection(t *testing.T, spec, level, title string) []string {
	t.Helper()
	var out []string
	in, fenced := false, false
	for line := range strings.Lines(spec) {
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "#") {
			marks, _, _ := strings.Cut(line, " ")
			if in && len(marks) <= len(level) {
				break
			}
			if marks == level && strings.Contains(line, title) {
				in = true
				continue
			}
		}
		if in {
			out = append(out, line)
		}
	}
	if !in {
		t.Fatalf("%s: no %s heading containing %q", specFile, level, title)
	}
	return out
}

func readSpec(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// nameInvariants replaces the invariant numbers of a specification text by
// the invariant names, as everything outside the specification refers to
// them.
func nameInvariants(t *testing.T, text string, names map[string]string) string {
	t.Helper()
	return invariantRef.ReplaceAllStringFunc(text, func(ref string) string {
		name, ok := names[ref[1:]]
		if !ok {
			t.Fatalf("%q refers to an invariant the specification does not define", text)
		}
		return name
	})
}

func invariantNames(t *testing.T, spec string) map[string]string {
	t.Helper()
	names := map[string]string{}
	for _, line := range specSection(t, spec, "##", "Invariants") {
		if m := invariantRow.FindStringSubmatch(line); m != nil {
			names[m[1]] = m[2]
		}
	}
	if len(names) == 0 {
		t.Fatal("no invariants found in the specification")
	}
	return names
}

func seedRows(t *testing.T, spec string, names map[string]string) []specRow {
	t.Helper()
	var rows []specRow
	for _, line := range specSection(t, spec, "##", "Seed error codes") {
		m := codeRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		r := specRow{
			code:      errs.Code(m[1]),
			condition: strings.TrimSpace(m[2]),
			exit:      specValue(m[3]),
			state:     specValue(m[4]),
		}
		if a := additionMark.FindStringSubmatch(r.condition); a != nil {
			r.condition, r.addedBy = a[1], a[2]
		}
		r.condition = nameInvariants(t, r.condition, names)
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		t.Fatal("no seed error codes found in the specification")
	}
	return rows
}

// specValue converts an exit or device-state cell of the specification to
// its codes.yaml form.
func specValue(cell string) string {
	switch v := strings.TrimSpace(cell); v {
	case emDash:
		return "none"
	default:
		return strings.ReplaceAll(v, " ", "-")
	}
}

// specKind derives the kind of a seed code from its condition.
func specKind(condition string) errs.Kind {
	switch {
	case strings.Contains(condition, "(warning"):
		return errs.KindWarning
	case strings.Contains(condition, "(informational)"):
		return errs.KindInfo
	}
	return errs.KindError
}

// TestSeedCodesMatchTheSpecification is the transcription check: every row
// of the specification's table of seed error codes is registered with the
// same condition, exit code and device state, and nothing else is
// registered.
func TestSeedCodesMatchTheSpecification(t *testing.T) {
	spec := readSpec(t)
	names := invariantNames(t, spec)
	rows := seedRows(t, spec, names)

	var seed int
	var added []errs.Code
	listed := map[errs.Code]bool{}
	for _, r := range rows {
		if listed[r.code] {
			t.Errorf("%s is listed twice in the specification", r.code)
		}
		listed[r.code] = true
		switch r.addedBy {
		case "":
			seed++
		case additionADR:
			added = append(added, r.code)
		}
		info, ok := errs.Lookup(r.code)
		if !ok {
			t.Errorf("%s is in the specification but not registered", r.code)
			continue
		}
		if info.Condition != r.condition {
			t.Errorf("%s: condition\n  registered:    %q\n  specification: %q", r.code, info.Condition, r.condition)
		}
		if info.Exit != r.exit || info.State != r.state {
			t.Errorf("%s: registered exit %s and state %s, the specification says %s and %s",
				r.code, info.Exit, info.State, r.exit, r.state)
		}
		if want := specKind(r.condition); info.Kind != want {
			t.Errorf("%s: registered kind %s, the specification's condition reads as %s", r.code, info.Kind, want)
		}
	}
	slices.Sort(added)
	want := slices.Sorted(slices.Values(additions))
	if !slices.Equal(added, want) {
		t.Errorf("the specification marks %v as added by ADR %s, want %v", added, additionADR, want)
	}
	if got := len(errs.All()); got != seed+len(additions) {
		t.Errorf("%d codes registered, want the specification's %d seed codes plus %d additions", got, seed, len(additions))
	}
	for _, info := range errs.All() {
		if !listed[info.Code] {
			t.Errorf("%s is registered but not in the specification's table of seed error codes", info.Code)
		}
	}
}

// TestCategoriesMatchTheSpecification checks the categories, their order
// and their descriptions against the specification's category table.
func TestCategoriesMatchTheSpecification(t *testing.T) {
	spec := readSpec(t)
	names := invariantNames(t, spec)
	var want []string
	for _, line := range specSection(t, spec, "###", "Categories") {
		if m := categoryRow.FindStringSubmatch(line); m != nil {
			want = append(want, m[1]+" | "+strings.TrimSpace(m[2])+" | "+nameInvariants(t, strings.TrimSpace(m[3]), names))
		}
	}
	var got []string
	for _, c := range loadRegistry(t).Categories {
		got = append(got, c.Prefix+" | "+c.Title+" | "+c.Covers)
	}
	if !slices.Equal(got, want) {
		t.Errorf("categories differ from the specification\n  registered:    %q\n  specification: %q", got, want)
	}
}
