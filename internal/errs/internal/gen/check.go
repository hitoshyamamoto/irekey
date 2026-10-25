// SPDX-License-Identifier: MIT

package gen

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Paths, relative to the root of the repository, that Check reads.
const (
	RegistryFile = "internal/errs/codes.yaml"
	GoFile       = "internal/errs/zz_generated_codes.go"
	DocsFile     = "docs/errors.md"
)

// errsImport is the import path of package errs.
const errsImport = "github.com/hitoshyamamoto/irekey/internal/errs"

// errsDir holds package errs and the generator, which define the codes
// rather than use them.
const errsDir = "internal/errs/"

// generate is the command that brings the generated files up to date.
const generate = "go generate ./internal/errs"

var (
	// constPattern matches the name of a generated code constant.
	constPattern = regexp.MustCompile(`^[A-Z]{2,5}\d{3}$`)
	// yamlLinePattern finds the line in an error of the YAML decoder.
	yamlLinePattern = regexp.MustCompile(`line (\d+)`)
)

// Finding is one problem Check found, at a line of a file.
type Finding struct {
	Path string // slash-separated, relative to the root
	Line int
	Msg  string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Msg)
}

// Check checks the repository rooted at root:
//
//   - the registry is valid, and the generated Go file and docs/errors.md
//     are up to date with it;
//   - non-test Go code outside internal/errs never writes a code as a
//     string literal or a constant concatenation of literals, never imports
//     package errs with a dot, and uses only registered codes, through
//     their generated constants (errs.LOC001);
//   - a code that non-test code uses is active, and an active code is used
//     by non-test code outside internal/errs, and a test outside
//     internal/errs refers to its constant;
//   - the Markdown files, except the specification, docs/errors.md and
//     test data, mention only registered codes.
//
// The files checked are those git lists, tracked or untracked but not
// ignored, when root is inside a git work tree, and every file under root
// otherwise.
// Findings are sorted by path and line. An error means the check could not
// run.
func Check(root string) ([]Finding, error) {
	rt, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rt.Close() }()

	data, err := rt.ReadFile(filepath.FromSlash(RegistryFile))
	if err != nil {
		return nil, err
	}
	reg, err := Parse(data)
	if err != nil {
		return []Finding{{RegistryFile, yamlLine(err), err.Error()}}, nil
	}
	if problems := validate(reg); len(problems) > 0 {
		out := make([]Finding, len(problems))
		for i, p := range problems {
			out[i] = Finding{RegistryFile, max(p.Line, 1), p.Msg}
		}
		return out, nil
	}

	out, err := staleFindings(rt, reg)
	if err != nil {
		return nil, err
	}
	paths, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	s := newUsageScan(reg)
	for _, p := range paths {
		isGo := strings.HasSuffix(p, ".go") && !strings.HasPrefix(p, errsDir) && !inTestdata(p)
		if !isGo && !isCheckedDoc(p) {
			continue
		}
		data, ok, err := readRegular(rt, p)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if isGo {
			s.goFile(p, data)
		} else {
			s.docFile(p, data)
		}
	}
	out = append(out, s.findings...)
	out = append(out, s.statusFindings()...)
	slices.SortStableFunc(out, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line))
	})
	return out, nil
}

// yamlLine extracts the line number from a YAML decoding error, or returns
// 1.
func yamlLine(err error) int {
	m := yamlLinePattern.FindStringSubmatch(err.Error())
	if m == nil {
		return 1
	}
	n, _ := strconv.Atoi(m[1]) // digits, by the pattern
	return n
}

// isCheckedDoc reports whether the code mentions of a file are checked:
// every Markdown file of the repository, at the root (README.md and the
// others), under docs/ or anywhere else, except the specification, which
// defines the codes, docs/errors.md, which is generated, and test data.
func isCheckedDoc(p string) bool {
	return strings.HasSuffix(p, ".md") && !strings.HasPrefix(p, "docs/spec/") &&
		p != DocsFile && !inTestdata(p)
}

// staleFindings compares the generated files with what the registry renders
// to.
func staleFindings(rt *os.Root, reg *Registry) ([]Finding, error) {
	goSrc, err := RenderGo(reg)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, f := range []struct {
		path string
		want []byte
	}{{GoFile, goSrc}, {DocsFile, RenderMarkdown(reg)}} {
		got, err := rt.ReadFile(filepath.FromSlash(f.path))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			out = append(out, Finding{f.path, 1, "missing: run " + generate})
		case err != nil:
			return nil, err
		case !bytes.Equal(got, f.want):
			out = append(out, Finding{f.path, 1, "out of date with " + RegistryFile + ": run " + generate})
		}
	}
	return out, nil
}

// use is where a code constant is referred to.
type use struct {
	path string
	line int
}

// usageScan collects how the scanned files use the codes.
type usageScan struct {
	reg      *Registry
	byConst  map[string]Code // registered codes by constant name
	literal  *regexp.Regexp  // a code with a registered prefix
	used     map[string]use  // first use in non-test code, by constant name
	tested   map[string]bool // constant names used by tests
	findings []Finding
}

func newUsageScan(reg *Registry) *usageScan {
	prefixes := make([]string, len(reg.Categories))
	for i, c := range reg.Categories {
		prefixes[i] = regexp.QuoteMeta(c.Prefix)
	}
	s := &usageScan{
		reg:     reg,
		byConst: map[string]Code{},
		literal: regexp.MustCompile(`\b(?:` + strings.Join(prefixes, "|") + `)-\d{3}\b`),
		used:    map[string]use{},
		tested:  map[string]bool{},
	}
	for _, c := range reg.Codes {
		s.byConst[ConstName(c.Code)] = c
	}
	return s
}

func (s *usageScan) add(path string, line int, format string, args ...any) {
	s.findings = append(s.findings, Finding{path, line, fmt.Sprintf(format, args...)})
}

// goFile records the code constants a Go file uses and reports string
// literals that spell out a code.
func (s *usageScan) goFile(p string, data []byte) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, data, parser.SkipObjectResolution)
	if err != nil {
		line := 1
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 && list[0].Pos.Line > 0 {
			line = list[0].Pos.Line
		}
		s.add(p, line, "cannot parse Go file, so its use of error codes cannot be checked: %v", err)
		return
	}
	test := strings.HasSuffix(p, "_test.go")
	// A file may import package errs more than once, under several names.
	names := map[string]bool{}
	for _, imp := range f.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path != errsImport {
			continue
		}
		name := "errs"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch name {
		case ".":
			s.add(p, fset.Position(imp.Pos()).Line,
				"imports %s with a dot: import it by name, so that the codes it uses can be checked", errsImport)
		case "_":
		default:
			names[name] = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			// A constant concatenation, such as "LOC" + "-001", is checked
			// as a whole. When it names a code, it is reported once and its
			// literals are not looked at again.
			if v, ok := constString(x); ok && !test && s.stringValue(p, fset.Position(x.Pos()).Line, v, "constant string expression") {
				return false
			}
		case *ast.BasicLit:
			if !test && x.Kind == token.STRING {
				v, _ := constString(x)
				s.stringValue(p, fset.Position(x.Pos()).Line, v, "string literal")
			}
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok || !names[id.Name] || !constPattern.MatchString(x.Sel.Name) {
				return true
			}
			switch {
			case test:
				s.tested[x.Sel.Name] = true
			default:
				if _, seen := s.used[x.Sel.Name]; !seen {
					s.used[x.Sel.Name] = use{p, fset.Position(x.Pos()).Line}
				}
			}
		}
		return true
	})
}

// stringValue reports a constant string of non-test code that contains a
// code, and tells whether it did.
func (s *usageScan) stringValue(p string, line int, value, what string) bool {
	code := s.literal.FindString(value)
	if code == "" {
		return false
	}
	if _, ok := s.byConst[ConstName(code)]; !ok {
		s.add(p, line, "%s contains %s, which is not a registered error code: register it in %s and use its generated constant",
			what, code, RegistryFile)
		return true
	}
	s.add(p, line, "%s contains the error code %s: use the generated constant errs.%s", what, code, ConstName(code))
	return true
}

// constString returns the value of a string literal, or of a concatenation
// of string literals, possibly in parentheses.
func constString(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(x.Value)
		if err != nil {
			// The parser accepted the literal, so this does not happen;
			// the raw text is still worth searching.
			return x.Value, true
		}
		return v, true
	case *ast.ParenExpr:
		return constString(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, ok := constString(x.X)
		if !ok {
			return "", false
		}
		r, ok := constString(x.Y)
		return l + r, ok
	}
	return "", false
}

// docFile reports mentions of codes that are not registered. Only codes
// with a registered prefix are recognized: a name such as FOO-001, whose
// prefix is no category, is not an irekey code and is out of scope.
func (s *usageScan) docFile(p string, data []byte) {
	for i, line := range strings.Split(string(data), "\n") {
		for _, code := range s.literal.FindAllString(line, -1) {
			if _, ok := s.byConst[ConstName(code)]; !ok {
				s.add(p, i+1, "mentions %s, which is not a registered error code (%s)", code, RegistryFile)
			}
		}
	}
}

// statusFindings compares the use of every code with its status.
func (s *usageScan) statusFindings() []Finding {
	var out []Finding
	for _, name := range slices.Sorted(maps.Keys(s.used)) {
		u := s.used[name]
		c, ok := s.byConst[name]
		switch {
		case !ok:
			out = append(out, Finding{u.path, u.line, fmt.Sprintf(
				"errs.%s is not a registered error code: register it in %s", name, RegistryFile)})
		case c.Status == Reserved:
			out = append(out, Finding{u.path, u.line, fmt.Sprintf(
				"%s is used here but reserved: mark it active in %s, with its signal and handling",
				c.Code, RegistryFile)})
		}
	}
	for _, c := range s.reg.Codes {
		if c.Status != Active {
			continue
		}
		name := ConstName(c.Code)
		line := max(c.Line, 1)
		_, used := s.used[name]
		switch {
		case !used:
			out = append(out, Finding{RegistryFile, line, fmt.Sprintf(
				"%s is active, but no non-test code outside %s uses errs.%s: set it back to reserved",
				c.Code, errsDir, name)})
		case !s.tested[name]:
			out = append(out, Finding{RegistryFile, line, fmt.Sprintf(
				"%s is active, but no test outside %s refers to errs.%s: add a test that produces the code and refers to the constant",
				c.Code, errsDir, name)})
		}
	}
	return out
}
