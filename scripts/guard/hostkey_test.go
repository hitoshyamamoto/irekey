// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestHostKeyFindings(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []int // lines with findings, counted in body
	}{
		{"pinned callback", `
var cb = func(host string, remote net.Addr, key ssh.PublicKey) error {
	return compare(key)
}`, nil},
		{"callback that checks first", `
func check(host string, remote net.Addr, key ssh.PublicKey) error {
	if key == nil {
		return errNoKey
	}
	return nil
}`, nil},
		{"no-op method with three parameters", `
func (none) Publish(ctx context.Context, repo, ref string) error { return nil }`, nil},
		{"two parameters", `
var cb = func(remote net.Addr, key ssh.PublicKey) error { return nil }`, nil},
		{"returns a value other than nil", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error { return errDenied }`, nil},
		{"no error result", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) bool { return true }`, nil},
		{"other identifiers", `
var cb = ssh.FixedHostKey(key)`, nil},
		{"insecure helper", `
var cfg = ssh.ClientConfig{HostKeyCallback: ssh.` + insecureHostKey + `()}`, []int{2}},
		{"insecure helper as a value", `
var cb = ` + insecureHostKey, []int{2}},
		{"accept-all literal", `
var cb = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return nil
}`, []int{2}},
		{"accept-all literal with unnamed parameters", `
var cb = ssh.HostKeyCallback(func(string, net.Addr, ssh.PublicKey) error { return nil })`, []int{2}},
		{"address parameter of another type", `
var cb = func(h string, r *net.TCPAddr, k ssh.PublicKey) error { return (nil) }`, nil},
		{"named result and parenthesized nil", `
var cb = func(h string, r net.Addr, k PublicKey) (err error) { return (nil) }`, []int{2}},
		{"accept-all method", `

func (p *pinner) check(host string, remote net.Addr, key ssh.PublicKey) error {
	return nil
}`, []int{3}},
		{"accept-all method used as a value", `
func (pinner) Check(h string, r net.Addr, k ssh.PublicKey) error { return nil }

var cb = pinner{}.Check`, []int{2}},
		{"insecure helper through an aliased import", `
var cfg = xssh.ClientConfig{HostKeyCallback: xssh.` + insecureHostKey + `()}`, []int{2}},
		{"logs, then accepts", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error {
	log.Printf("accepting %s", h)
	return nil
}`, []int{2}},
		{"checks, but drops the result", `
func ignore(h string, r net.Addr, k ssh.PublicKey) error {
	_ = verify(k)
	return nil
}`, []int{2}},
		{"naked return of an unassigned named result", `
func naked(h string, r net.Addr, k ssh.PublicKey) (err error) {
	return
}`, []int{2}},
		{"unassigned named result returned by name", `
func named(h string, r net.Addr, k ssh.PublicKey) (err error) {
	return err
}`, []int{2}},
		{"blank named result", `
func blank(h string, r net.Addr, k ssh.PublicKey) (_ error) {
	return
}`, []int{2}},
		{"conversion of nil", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error { return error(nil) }`, []int{2}},
		{"nil on every path", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error {
	if k == nil {
		return nil
	}
	return nil
}`, []int{2}},
		{"error only inside a nested literal", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error {
	check := func() error { return errDenied }
	_ = check
	return nil
}`, []int{2}},
		{"naked return of an assigned named result", `
func set(h string, r net.Addr, k ssh.PublicKey) (err error) {
	err = verify(k)
	return
}`, nil},
		{"named result assigned in a closure", `
func deferred(h string, r net.Addr, k ssh.PublicKey) (err error) {
	defer func() { err = verify(k) }()
	return
}`, nil},
		{"named result passed by address", `
func addr(h string, r net.Addr, k ssh.PublicKey) (err error) {
	load(&err)
	return err
}`, nil},
		{"named result set by a range loop", `
func loop(h string, r net.Addr, k ssh.PublicKey) (err error) {
	for _, err = range errs {
	}
	return
}`, nil},
		{"conversion of a non-nil value", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error { return error(errDenied) }`, nil},
		{"two results returned", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error { return nil, nil }`, nil},
		{"no return statement", `
var cb = func(h string, r net.Addr, k ssh.PublicKey) error { panic("unreachable") }`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := parseGo("x.go", []byte(goFile(tt.body)))
			if err != nil {
				t.Fatal(err)
			}
			got := hostKeyFindings(src)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want findings on body lines %v", got, tt.want)
			}
			for i, f := range got {
				// goFile puts four lines before the body.
				if f.line-4 != tt.want[i] {
					t.Errorf("finding %d on body line %d, want %d", i, f.line-4, tt.want[i])
				}
			}
		})
	}
}

func TestCheckHostKey_Tree(t *testing.T) {
	bad := goFile("var cb = ssh." + insecureHostKey + "()")
	root := writeTree(t, map[string]string{
		"internal/transport/ssh/client.go":      bad,
		"internal/transport/ssh/client_test.go": bad,
		"internal/x/testdata/fixture.go":        bad,
		"internal/broken/broken.go":             goFile("func {"),
		"internal/ok/ok.go":                     goFile("var x = 1"),
	})
	code, out := runGuard(t, "hostkey", "-root", root)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, exitFindings, out)
	}
	for _, want := range []string{
		"internal/transport/ssh/client.go:5: " + insecureHostKey,
		"internal/broken/broken.go:5: cannot parse Go file",
		"error: hostkey guard: 2 finding(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "_test.go") || strings.Contains(out, "testdata") {
		t.Errorf("test files and testdata must be skipped:\n%s", out)
	}
}

func TestParseFailure_WithoutPosition(t *testing.T) {
	f := parseFailure("a.go", testError("boom"))
	if f.line != 1 || !strings.Contains(f.msg, "boom") {
		t.Errorf("parseFailure = %v", f)
	}
}

type testError string

func (e testError) Error() string { return string(e) }
