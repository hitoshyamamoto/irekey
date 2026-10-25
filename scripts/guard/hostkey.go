// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/token"
)

// insecureHostKey is the x/crypto/ssh helper that accepts any host key.
// It is assembled so that this file never contains the identifier itself.
const insecureHostKey = "Insecure" + "IgnoreHostKey"

const (
	msgInsecureHostKey = insecureHostKey + " disables host-key verification: " +
		"pin the device host key with a callback that compares it"
	msgAnyHostKey = "host-key callback accepts every key (no return statement can return " +
		"an error): compare the presented key with the pinned one"
)

// checkHostKey fails when non-test Go code can connect to a device
// without verifying its host key: a reference to the insecure helper
// (under any import name, called or used as a value), or a function with the
// host-key callback signature that can only return nil.
func checkHostKey(root string) ([]finding, error) {
	sources, out, err := loadGoSources(root, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	for _, src := range sources {
		out = append(out, hostKeyFindings(src)...)
	}
	return out, nil
}

func hostKeyFindings(src goSource) []finding {
	var out []finding
	ast.Inspect(src.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			// Covers plain identifiers and the Sel part of pkg.Name.
			if n.Name == insecureHostKey {
				out = append(out, finding{src.path, src.line(n), msgInsecureHostKey})
			}
		case *ast.FuncLit:
			if acceptsAnyHostKey(n.Type, n.Body) {
				out = append(out, finding{src.path, src.line(n), msgAnyHostKey})
			}
		case *ast.FuncDecl:
			if acceptsAnyHostKey(n.Type, n.Body) {
				out = append(out, finding{src.path, src.line(n), msgAnyHostKey})
			}
		}
		return true
	})
	return out
}

// acceptsAnyHostKey reports whether a function has the signature of an
// SSH host-key callback, func(hostname string, remote net.Addr, key
// ssh.PublicKey) error, and no return statement of its body (nested
// function literals aside) can return a non-nil error. A nil error is the
// identifier nil, a conversion such as error(nil), or the named result
// when the function never assigns it, returned by name or by a naked
// return. Logging, or calling a check and dropping its result, does not
// make such a callback verify anything.
//
// The parameter types are matched by name (string, Addr, PublicKey) so
// that ordinary three-argument no-op methods returning error, such as a
// do-nothing implementation of an interface, are not reported.
func acceptsAnyHostKey(ft *ast.FuncType, body *ast.BlockStmt) bool {
	if body == nil || !hostKeySignature(ft) {
		return false
	}
	result := ""
	if names := ft.Results.List[0].Names; len(names) == 1 {
		result = names[0].Name
	}
	resultNil := result != "" && (result == "_" || !assigns(body, result))

	returns, nonNil := 0, false
	inspectShallow(body, func(n ast.Node) {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return
		}
		returns++
		if !returnsNil(ret, result, resultNil) {
			nonNil = true
		}
	})
	return returns > 0 && !nonNil
}

func hostKeySignature(ft *ast.FuncType) bool {
	if ft.Results.NumFields() != 1 || typeName(ft.Results.List[0].Type) != "error" {
		return false
	}
	params := paramTypes(ft.Params)
	return len(params) == 3 &&
		typeName(params[0]) == "string" &&
		typeName(params[1]) == "Addr" &&
		typeName(params[2]) == "PublicKey"
}

// returnsNil reports whether a return statement returns a nil error.
func returnsNil(ret *ast.ReturnStmt, result string, resultNil bool) bool {
	switch len(ret.Results) {
	case 0:
		return resultNil // a naked return of the named result
	case 1:
		return isNilError(ret.Results[0], result, resultNil)
	}
	return false
}

func isNilError(e ast.Expr, result string, resultNil bool) bool {
	switch e := unparen(e).(type) {
	case *ast.Ident:
		return e.Name == "nil" || (e.Name == result && resultNil)
	case *ast.CallExpr:
		// error(nil), a conversion.
		fn, ok := unparen(e.Fun).(*ast.Ident)
		return ok && fn.Name == "error" && len(e.Args) == 1 && isNilError(e.Args[0], result, resultNil)
	}
	return false
}

// assigns reports whether body may assign name: an assignment to it, or
// its address taken, anywhere in body, closures included.
func assigns(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.UnaryExpr:
			if id, ok := n.X.(*ast.Ident); ok && n.Op == token.AND && id.Name == name {
				found = true
			}
		case *ast.RangeStmt:
			for _, x := range []ast.Expr{n.Key, n.Value} {
				if id, ok := x.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// inspectShallow calls f for every node of body except those inside
// nested function literals, whose return statements are their own.
func inspectShallow(body *ast.BlockStmt, f func(ast.Node)) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if n != nil {
			f(n)
		}
		return true
	})
}

// paramTypes lists the type of every parameter, once per name:
// (a, b string) gives two entries.
func paramTypes(fl *ast.FieldList) []ast.Expr {
	if fl == nil {
		return nil
	}
	var types []ast.Expr
	for _, field := range fl.List {
		for range max(1, len(field.Names)) {
			types = append(types, field.Type)
		}
	}
	return types
}
