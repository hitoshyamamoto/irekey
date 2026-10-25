// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// FuzzRealData feeds arbitrary text to the real-data scanner: it must
// never panic, and every finding must point at an existing line.
func FuzzRealData(f *testing.F) {
	for _, seed := range []string{
		"",
		"host 192.0.2.1 and " + publicIPv4 + ".\n",
		"[2001:db8::1]:22 " + linkLocalV6 + " ::ffff:192.0.2.1 :::\n",
		"mac " + globalMAC + " 02:00:00:00:00:01 " + zeroMAC,
		"https://example.com https://" + realDomain + " a@b.github.com\r\n",
		"1.27.3 18:05:11 SHA256:abc std::string",
		"`opts.io` and ``p.app`` and WWW.EXAMPLE.COM and host" + ".corp\n```\ncfg.name\n```\n`x",
		"\xff\xfe" + eAcute + "\n\n\n",
	} {
		f.Add(seed)
	}
	allow, _ := parseDomainAllowlist(allowedDomainsFile, []byte("github.com # hosting\n"))
	f.Fuzz(func(t *testing.T, text string) {
		lines := strings.Count(text, "\n") + 1
		for _, fd := range realDataFindings("docs/fuzz.md", []byte(text), allow) {
			if fd.line < 1 || fd.line > lines {
				t.Fatalf("finding on line %d of a %d-line text: %v", fd.line, lines, fd)
			}
			if fd.path != "docs/fuzz.md" || fd.msg == "" {
				t.Fatalf("bad finding %v", fd)
			}
		}
	})
}

// FuzzModuleAllowlist feeds arbitrary text to the module allowlist parser
// and to the go.mod line finder.
func FuzzModuleAllowlist(f *testing.F) {
	f.Add(testGoMod, "golang.org/x/crypto BSD-3-Clause # SSH\n", "golang.org/x/crypto")
	f.Add("require (\n)\nreplace x => ./lab\n", "x MIT OR # y\n", "./lab")
	f.Add("replace (\n\t=>\n", "#\n# only comments\n", "")
	f.Fuzz(func(t *testing.T, gomod, list, modPath string) {
		lines := strings.Count(gomod, "\n") + 1
		for _, replace := range []bool{false, true} {
			if n := goModLine([]byte(gomod), modPath, replace); n < 1 || n > lines {
				t.Fatalf("goModLine = %d for a %d-line go.mod", n, lines)
			}
		}
		listLines := strings.Count(list, "\n") + 1
		_, findings := parseModuleAllowlist(allowedModulesFile, []byte(list))
		for _, fd := range findings {
			if fd.line < 1 || fd.line > listLines || fd.msg == "" {
				t.Fatalf("bad finding %v for a %d-line allowlist", fd, listLines)
			}
		}
	})
}
