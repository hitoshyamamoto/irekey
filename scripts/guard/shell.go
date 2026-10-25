// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/token"
	"path"
	"regexp"
)

// The shell guard keeps remote commands out of string assembly. Its rule:
//
//  1. A call of a method named Run, Start, Output or CombinedOutput is
//     reported when any of its arguments is an assembled string. An
//     assembled string is a call of fmt.Sprintf, fmt.Sprint, fmt.Sprintln
//     or strings.Join; a "+" expression; a call of a String method with no
//     arguments (strings.Builder, bytes.Buffer); or a variable that the
//     enclosing function assigns one of these, or extends with "+=".
//     String literals, calls of the quoting functions, and values the
//     function does not build itself (parameters, results of other calls)
//     are not assembled strings.
//  2. A call of Command or CommandContext of os/exec, under whatever name
//     the file imports the package, is reported when it starts a shell
//     (sh, bash, dash, ash, zsh or ksh, matched on the base name, so
//     /usr/bin/bash counts) with a single-dash flag containing c (-c, -lc,
//     -ec), and the script after that flag is not a string literal, a call
//     of a quoting function, or a variable or constant assigned only
//     those.
//
// The quoting functions are the ones named quote, Quote or joinArgv; they
// live in the quoting file, which is not checked, and neither are tests.
// Packages are resolved per file, so aliased imports are covered.

// quotingFile holds the shared shell-quoting function, the one place
// where a command string may be assembled.
const quotingFile = "internal/transport/ssh/quote.go"

const shellAdvice = "build remote commands from an argument vector and " +
	"quote them with the shared quoting function"

// runMethods are the methods that run a command given as a string, as on
// an SSH session and on irekey's own transport wrappers, Run(ctx, cmd).
var runMethods = map[string]bool{"Run": true, "Start": true, "Output": true, "CombinedOutput": true}

// shellNames are the programs whose -c argument is a script.
var shellNames = map[string]bool{"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true, "ksh": true}

// quotingFuncs are the names of the shared quoting functions.
var quotingFuncs = map[string]bool{"quote": true, "Quote": true, "joinArgv": true}

// scriptFlag matches a single-dash shell option group that includes -c.
var scriptFlag = regexp.MustCompile(`^-[A-Za-z]*c[A-Za-z]*$`)

// checkShell fails when non-test Go code assembles a shell command from
// strings (see the rule above).
func checkShell(root string) ([]finding, error) {
	sources, out, err := loadGoSources(root, func(p string) bool { return p != quotingFile })
	if err != nil {
		return nil, err
	}
	for _, src := range sources {
		out = append(out, shellFindings(src)...)
	}
	return out, nil
}

// assignment is one value given to a variable or constant.
type assignment struct {
	value  ast.Expr // nil when unknown, as for a multi-value call
	extend bool     // x += value
}

// assignments maps a name to every value assigned to it in a scope. Names
// are not resolved: a shadowing variable counts as the same one, which
// errs on the side of reporting.
type assignments map[string][]assignment

func collectAssignments(nodes ...ast.Node) assignments {
	as := assignments{}
	for _, n := range nodes {
		collectInto(as, n)
	}
	return as
}

func collectInto(as assignments, n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				a := assignment{extend: n.Tok == token.ADD_ASSIGN}
				if len(n.Rhs) == len(n.Lhs) {
					a.value = n.Rhs[i]
				}
				as[id.Name] = append(as[id.Name], a)
			}
		case *ast.ValueSpec:
			for i, id := range n.Names {
				if len(n.Values) == len(n.Names) {
					as[id.Name] = append(as[id.Name], assignment{value: n.Values[i]})
				}
			}
		}
		return true
	})
}

// shellScanner checks the calls of one file.
type shellScanner struct {
	fmt, strings, exec pkgNames
	consts             assignments // package-level constants and variables
}

func shellFindings(src goSource) []finding {
	s := shellScanner{
		fmt:     importNames(src.file, "fmt"),
		strings: importNames(src.file, "strings"),
		exec:    importNames(src.file, "os/exec"),
		consts:  collectAssignments(packageValues(src.file)...),
	}
	var out []finding
	for _, decl := range src.file.Decls {
		// The variables of a function: its own, and those of the closures
		// inside it, which may share them.
		local := collectAssignments(decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if msg := s.check(call, local); msg != "" {
					out = append(out, finding{src.path, src.line(call), msg})
				}
			}
			return true
		})
	}
	return out
}

// packageValues returns the package-level const and var declarations.
func packageValues(f *ast.File) []ast.Node {
	var decls []ast.Node
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && (g.Tok == token.CONST || g.Tok == token.VAR) {
			decls = append(decls, g)
		}
	}
	return decls
}

func (s shellScanner) check(call *ast.CallExpr, local assignments) string {
	if s.exec.calls(call, "Command", "CommandContext") {
		if flag, computed := s.computedScript(call.Args, local); computed {
			return "shell started with " + flag + " and a computed script: " + shellAdvice
		}
		return ""
	}
	sel, ok := unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !runMethods[sel.Sel.Name] {
		return ""
	}
	for _, arg := range call.Args {
		if s.assembled(arg, local, map[string]bool{}) {
			return sel.Sel.Name + " called with a command assembled from strings: " + shellAdvice
		}
	}
	return ""
}

// assembled reports whether e builds a string (rule 1). seen stops the
// search through variables that are assigned each other.
func (s shellScanner) assembled(e ast.Expr, local assignments, seen map[string]bool) bool {
	switch e := unparen(e).(type) {
	case *ast.BinaryExpr:
		return e.Op == token.ADD
	case *ast.CallExpr:
		if s.fmt.calls(e, "Sprintf", "Sprint", "Sprintln") || s.strings.calls(e, "Join") {
			return true
		}
		sel, ok := unparen(e.Fun).(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "String" && len(e.Args) == 0
	case *ast.Ident:
		if seen[e.Name] {
			return false
		}
		seen[e.Name] = true
		for _, a := range local[e.Name] {
			if a.extend || (a.value != nil && s.assembled(a.value, local, seen)) {
				return true
			}
		}
	}
	return false
}

// computedScript finds a shell started with a -c style flag whose script
// is computed (rule 2), and returns that flag.
func (s shellScanner) computedScript(args []ast.Expr, local assignments) (string, bool) {
	for i, arg := range args {
		prog, ok := stringLit(arg)
		if !ok || !shellNames[path.Base(prog)] {
			continue
		}
		for j := i + 1; j < len(args); j++ {
			flag, ok := stringLit(args[j])
			if !ok || !scriptFlag.MatchString(flag) {
				continue
			}
			if j+1 < len(args) && !s.fixedScript(args[j+1], local, map[string]bool{}) {
				return flag, true
			}
			return "", false
		}
		return "", false
	}
	return "", false
}

// fixedScript reports whether a script is a string literal, a quoting
// call, or a name assigned only those.
func (s shellScanner) fixedScript(e ast.Expr, local assignments, seen map[string]bool) bool {
	switch e := unparen(e).(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.CallExpr:
		return quotingFuncs[funcName(e)]
	case *ast.Ident:
		if seen[e.Name] {
			return false
		}
		seen[e.Name] = true
		as := local[e.Name]
		if len(as) == 0 {
			as = s.consts[e.Name]
		}
		if len(as) == 0 {
			return false
		}
		for _, a := range as {
			if a.extend || a.value == nil || !s.fixedScript(a.value, local, seen) {
				return false
			}
		}
		return true
	}
	return false
}
