// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"
)

// goSource is one parsed Go file.
type goSource struct {
	path string
	fset *token.FileSet
	file *ast.File
}

func (s goSource) line(n ast.Node) int {
	return s.fset.Position(n.Pos()).Line
}

// loadGoSources parses the non-test Go files under root that satisfy
// keep. Files in testdata directories are skipped: the go tool never
// builds them. A file that does not parse becomes a finding, because a
// guard that cannot read a file cannot vouch for it.
func loadGoSources(root string, keep func(path string) bool) ([]goSource, []finding, error) {
	files, err := loadTextFiles(root, func(p string) bool {
		return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") &&
			!inTestdata(p) && keep(p)
	})
	if err != nil {
		return nil, nil, err
	}
	var (
		sources  []goSource
		findings []finding
	)
	for _, f := range files {
		src, err := parseGo(f.path, f.data)
		if err != nil {
			findings = append(findings, parseFailure(f.path, err))
			continue
		}
		sources = append(sources, src)
	}
	return sources, findings, nil
}

func parseGo(p string, data []byte) (goSource, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, p, data, parser.SkipObjectResolution)
	if err != nil {
		return goSource{}, err
	}
	return goSource{path: p, fset: fset, file: file}, nil
}

func parseFailure(p string, err error) finding {
	line := 1
	var list scanner.ErrorList
	if errors.As(err, &list) && len(list) > 0 && list[0].Pos.Line > 0 {
		line = list[0].Pos.Line
		err = errors.New(list[0].Msg)
	}
	return finding{p, line, "cannot parse Go file: " + err.Error()}
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// stringLit returns the value of a string literal expression.
func stringLit(e ast.Expr) (string, bool) {
	lit, ok := unparen(e).(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// pkgNames holds the names under which a file imports one package.
type pkgNames struct {
	names map[string]bool // exec, or an alias such as osexec
	dot   bool            // imported with "."
}

// importNames returns how file f refers to the package importPath.
func importNames(f *ast.File, importPath string) pkgNames {
	r := pkgNames{names: map[string]bool{}}
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		switch {
		case spec.Name == nil:
			r.names[path.Base(p)] = true
		case spec.Name.Name == ".":
			r.dot = true
		case spec.Name.Name != "_":
			r.names[spec.Name.Name] = true
		}
	}
	return r
}

// calls reports whether call calls one of funcs of the package.
func (r pkgNames) calls(call *ast.CallExpr, funcs ...string) bool {
	switch fn := unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		x, ok := fn.X.(*ast.Ident)
		return ok && r.names[x.Name] && slices.Contains(funcs, fn.Sel.Name)
	case *ast.Ident:
		return r.dot && slices.Contains(funcs, fn.Name)
	}
	return false
}

// funcName returns the name of the called function or method: f, pkg.f
// and x.f all give "f".
func funcName(call *ast.CallExpr) string {
	switch fn := unparen(call.Fun).(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// typeName returns the name of a type expression: T, *T, pkg.T and *pkg.T
// all give "T".
func typeName(e ast.Expr) string {
	switch t := unparen(e).(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return typeName(t.X)
	}
	return ""
}
