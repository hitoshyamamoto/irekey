// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strings"
	"testing"
)

// Made-up values with the shape of real ones, assembled at run time like
// the values in main_test.go: an SSH SHA-256 fingerprint (43 base64
// characters), an MD5 fingerprint (16 hex groups) and a fictional domain
// used as an allowlisted tool domain.
var (
	fakeSHA256Fingerprint = "SHA256:" + strings.Repeat("Zm9v", 10) + "YmE"
	fakeMD5Fingerprint    = globalMAC + ":2d:36:63:1b:56:4d:eb:df:a6:48"
	toolDomain            = "acme" + ".dev"
	dottedMAC             = "001a.2b3c" + ".4d5e"
)

const testDomainAllowlist = `# Tool domains for tests.

github.com # hosting
golang.org   # modules
`

func testAllowlist(t *testing.T) domainAllowlist {
	t.Helper()
	allow, findings := parseDomainAllowlist(allowedDomainsFile, []byte(testDomainAllowlist))
	if len(findings) > 0 {
		t.Fatalf("test allowlist: %v", findings)
	}
	return allow
}

type lineCase struct {
	name string
	line string
	want int // number of findings
}

func checkLines(t *testing.T, tests []lineCase, ctx lineContext) {
	t.Helper()
	allow := testAllowlist(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineRealData(tt.line, ctx, allow)
			if len(got) != tt.want {
				t.Errorf("lineRealData(%q) = %q, want %d finding(s)", tt.line, got, tt.want)
			}
		})
	}
}

func TestRealData_IPv4(t *testing.T) {
	checkLines(t, []lineCase{
		{"documentation range 1", "host 192.0.2.10", 0},
		{"documentation range 2 with port", "198.51.100.7:22", 0},
		{"documentation range 3 as prefix", "203.0.113.0/24", 0},
		{"loopback", "listen on 127.0.0.1 only", 0},
		{"loopback range", "127.10.20.30", 0},
		{"unspecified", "bind 0.0.0.0", 0},
		{"netmask", "netmask 255.255.255.0", 0},
		{"short netmask", "255.255.254.0 and 255.0.0.0", 0},
		{"broadcast mask", "255.255.255.255", 0},
		{"go version", "go 1.27.3", 0},
		{"toolchain", "toolchain go1.27.3", 0},
		{"two-part version", "version 1.2", 0},
		{"object identifier", "OID 1.3.6.1.4.1.32473", 0},
		{"five parts", "1.2.3" + ".4.5", 0},
		{"leading dot", "x." + privateIPv4, 0},
		{"octet out of range", "999.1.1.1", 0},
		{"leading zeros", "010.000" + ".000.001", 1},
		{"leading zeros, public", "008.008" + ".008.008", 1},
		{"version with a v", "v1.2.3", 0},
		{"public address", "dns " + publicIPv4, 1},
		{"private address", privateIPv4, 1},
		{"in a URL", "http://" + publicIPv4 + ":8080/x", 1},
		{"end of sentence", "Use " + privateIPv4 + ".", 1},
		{"after a letter", "v" + privateIPv4, 1},
		{"two on a line", publicIPv4 + " " + privateIPv4, 2},
		{"non-contiguous mask", "255.0." + "255.0", 1},
		{"mask of 9 bits", "255.128.0.0", 0},
		{"mask shorter than 8 bits", "128.0" + ".0.0 and 192.0" + ".0.0 and 254.0" + ".0.0", 3},
		{"four-part version", "FileVersion 1.0" + ".0.0", 1},
	}, lineContext{})
}

func TestRealData_IPv6(t *testing.T) {
	checkLines(t, []lineCase{
		{"documentation range", "2001:db8::1", 0},
		{"documentation range in brackets", "[2001:db8::10]:22", 0},
		{"documentation range, uppercase", "2001:DB8:0:0:0:0:0:FF", 0},
		{"loopback", "bind ::1", 0},
		{"unspecified", "listen [::]:0", 0},
		{"mapped documentation address", "::ffff:192.0.2.1", 0},
		{"mapped documentation address, hex", "::ffff:c000:201", 0},
		{"clock time", "ts 18:05:11.123Z", 0},
		{"clock time hh:mm", "at 10:30: done", 0},
		{"yaml key", "key: value", 0},
		{"go map literal", `m := map[string]int{"a": 1}`, 0},
		{"slice expression", "s[a:b:c]", 0},
		{"ssh fingerprint", fakeSHA256Fingerprint, 0},
		{"md5 fingerprint", fakeMD5Fingerprint, 0},
		{"scope operator", "std::string and Acquire::Retries=3", 0},
		{"admonition", ":::note", 0},
		{"link local", "via " + linkLocalV6, 1},
		{"after a label", "ip:" + linkLocalV6, 1},
		{"end of sentence", "Use " + linkLocalV6 + ".", 1},
		{"trailing colon", linkLocalV6 + ": up", 1},
		{"global unicast", "2002" + "::1", 1},
		{"mapped public address, hex", "::" + "ffff:808:404", 1},
		{"mapped public address, dotted", "::" + "ffff:" + publicIPv4, 1},
		{"next sentence after a dot", "via " + linkLocalV6 + ".Done", 1},
		{"next sentence after a dot, documentation", "via 2001:db8::1.Done", 0},
	}, lineContext{})
}

func TestRealData_MAC(t *testing.T) {
	checkLines(t, []lineCase{
		{"locally administered", "mac 02:00:00:00:00:15", 0},
		{"locally administered, dashes", "02-00-00-aa-bb-cc", 0},
		{"locally administered, uppercase", "0A:00:00:00:00:01", 0},
		{"broadcast", "ff:ff:ff:ff:ff:ff", 0},
		{"inside a longer fingerprint", globalMAC + ":63:1b:56:4d", 0},
		{"after a hex group", "ab:" + globalMAC, 0},
		{"before a hex group", globalMAC + "-ab", 0},
		{"after a hex digit", "a" + globalMAC, 0},
		{"mixed separators", strings.Replace(globalMAC, ":", "-", 2), 0},
		{"uuid", "123e4567-e89b-12d3-a456-426614174000", 0},
		{"date", "2026-10-25", 0},
		{"globally administered", "mac " + globalMAC, 1},
		{"all zero", zeroMAC, 1},
		{"dashes", strings.ReplaceAll(globalMAC, ":", "-"), 1},
		{"uppercase", strings.ToUpper(globalMAC), 1},
		{"end of sentence", globalMAC + ".", 1},
		{"dotted notation", "mac " + dottedMAC, 1},
		{"dotted notation, upper case", strings.ToUpper(dottedMAC), 1},
		{"dotted notation, locally administered", "0200.0000.0001", 0},
		{"dotted notation inside a longer group", dottedMAC + ".6f70", 0},
		{"dotted notation after a hex group", "6f70." + dottedMAC, 0},
	}, lineContext{})
}

func TestRealData_Domains(t *testing.T) {
	checkLines(t, []lineCase{
		{"example", "https://example.com/x", 0},
		{"example subdomain", "registry.example.com/irekey", 0},
		{"example net and org", "a.example.net b.example.org", 0},
		{"reserved suffixes", "dev.example host.test x.invalid ui.localhost localhost", 0},
		{"allowlisted", "https://github.com/hitoshyamamoto/irekey", 0},
		{"allowlisted subdomain", "docs.github.com and users.noreply.github.com", 0},
		{"allowlisted in an email address", "name@users.noreply.github.com", 0},
		{"module path", "golang.org/x/crypto/ssh", 0},
		{"markdown file names", "see README.md and docs/spec/masterprompt.md", 0},
		{"file extensions", "main.go a.sh b.py c.rs config.yaml d.json go.mod go.sum x.ts y.js", 0},
		{"go selectors", "errors.As(err, &target) and os.Getenv", 0},
		{"uppercase identifier", "Config.Net and X.Com", 0},
		{"underscore", "my_pkg.dev", 0},
		{"tld not in the root zone", "x.yml y.toml z.golden", 0},
		{"file extension tld", "Makefile.in key.pub multi-user.target go.work notes.md x.zip", 0},
		{"plain words", "e.g. i.e. etc.", 0},
		{"real domain", "https://www." + realDomain + "/", 1},
		{"email address", "someone@" + realDomain, 1},
		{"look-alike suffix", "not" + "github.com", 1},
		{"allowlisted name used as a subdomain", "github.com." + "globex" + ".co", 1},
		{"example as a subdomain", "example.com." + "globex" + ".de", 1},
		{"upper case", "WWW." + strings.ToUpper(realDomain), 1},
		{"mixed case in a URL", "https://Globex" + ".com/login", 1},
		{"capitalized last label", "Globex" + ".Com", 0},
		{"allowlisted, upper case", "GitHub.com/x", 0},
		{"country code", "www.globex" + ".es and globex" + ".it", 2},
		{"generic tlds", "globex" + ".biz globex" + ".xyz globex" + ".energy", 3},
		{"government and education", "energy" + ".gov and physics.university" + ".edu", 2},
		{"file extension tld after www", "www.globex" + ".in", 1},
		{"file extension tld in a URL", "https://globex" + ".sh/x", 1},
		{"file extension tld in an email address", "someone@globex" + ".so", 1},
		{"file extension tld before a path", "globex" + ".py/x", 1},
		{"file extension tld before a port", "globex" + ".md:8080", 1},
		{"call", "logging.info(msg) and asyncio.run(main())", 0},
		{"index", "x.data[0]", 0},
		{"field access in prose", "p" + ".app", 1},
		{"mdns name", "scada.globex" + ".local", 1},
		{"intranet suffixes", "a" + ".lan b" + ".corp c" + ".internal d" + ".intranet e.home" + ".arpa f" + ".localdomain", 6},
		{"intranet suffix whatever the tld list", "plc01.globex" + ".corp", 1},
		{"home network", "home" + ".arpa", 1},
	}, lineContext{domains: true})
}

func TestRealData_DomainsOnlyInScope(t *testing.T) {
	line := "see https://" + realDomain
	allow := testAllowlist(t)
	if got := lineRealData(line, lineContext{}, allow); len(got) != 0 {
		t.Errorf("domains out of scope: got %q", got)
	}
	if got := lineRealData(line, lineContext{domains: true}, allow); len(got) != 1 {
		t.Errorf("domains in scope: got %q, want 1 finding", got)
	}
}

func TestInDomainScope(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"README.md", true},
		{"CHANGELOG.md", true},
		{"docs/spec/masterprompt.md", true},
		{"docs/adr/0001-record-architecture-decisions.md", true},
		{"examples/deployment-repo/irekey.yaml", true},
		{"lab/images/device-systemd/Dockerfile", true},
		{"schemas/device.schema.json", true},
		{"testdata/compat/v0/bundle.json", true},
		{"internal/bundle/testdata/manifest.json", true},
		{".github/ISSUE_TEMPLATE/bug_report.yml", true},
		{".github/PULL_REQUEST_TEMPLATE.md", true},
		{".github/workflows/ci.yml", false},
		{".github/CODEOWNERS", false},
		{"internal/cli/root.go", false},
		{"internal/cli/doc.md", false},
		{"go.mod", false},
		{"LICENSE", false},
		{"scripts/guard/allowed-domains.txt", false},
	}
	for _, tt := range tests {
		if got := inDomainScope(tt.path); got != tt.want {
			t.Errorf("inDomainScope(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestParseDomainAllowlist(t *testing.T) {
	data := strings.Join([]string{
		"# header",
		"",
		"github.com # hosting",
		"  go.dev   # docs  ",
		"gitlab.com",
		"gitlab.com #",
		"GitHub.com # uppercase",
		"https://x.example.com # a URL",
	}, "\n")
	allow, got := parseDomainAllowlist("list.txt", []byte(data))
	if want := []string{"github.com", "go.dev"}; strings.Join(allow.domains, " ") != strings.Join(want, " ") {
		t.Errorf("domains = %q, want %q", allow.domains, want)
	}
	wantLines := []int{5, 6, 7, 8}
	if len(got) != len(wantLines) {
		t.Fatalf("findings = %v, want lines %v", got, wantLines)
	}
	for i, f := range got {
		if f.line != wantLines[i] || f.path != "list.txt" {
			t.Errorf("finding %d = %v, want list.txt line %d", i, f, wantLines[i])
		}
	}
}

func TestCheckRealData_Tree(t *testing.T) {
	root := writeTree(t, map[string]string{
		allowedDomainsFile:            toolDomain + " # fictional tool domain for this test\nbad entry\n",
		"docs/guide.md":               "Fetch from https://" + toolDomain + " and https://" + realDomain + "\n",
		"internal/cli/root.go":        goFile(`const url = "https://` + realDomain + `"`),
		"internal/x/testdata/dev.txt": "mac " + globalMAC + "\n",
		"config.yaml":                 "addr: " + publicIPv4 + "\n",
		"ok.md":                       "host 192.0.2.1, mac 02:00:00:00:00:01, https://example.com\n",
	})
	code, out := runGuard(t, "realdata", "-root", root)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, exitFindings, out)
	}
	for _, want := range []string{
		"config.yaml:1: IPv4 address",
		"docs/guide.md:1: domain " + realDomain,
		"internal/x/testdata/dev.txt:1: MAC address",
		allowedDomainsFile + ":2: ",
		"error: realdata guard: 4 finding(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestCheckRealData_MissingRoot(t *testing.T) {
	if _, err := checkRealData(t.TempDir() + "/missing"); err == nil {
		t.Error("want an error for a missing root")
	}
}

func TestRealData_CodeContext(t *testing.T) {
	allow := testAllowlist(t)
	tests := []struct {
		name string
		path string
		text string
		want []int // lines with findings
	}{
		{"field access in code spans", "docs/a.md",
			"`p.app`, `s.dev`, `c.info`, `paths.data`, `metadata.name`, `opts.io`, `h.me`, `x.id`, `msg.to`\n", nil},
		{"country code in a code span", "docs/a.md", "set `host` to `globex" + ".de`\n", []int{1}},
		{"classic tld in a code span", "docs/a.md", "``" + realDomain + "``\n", []int{1}},
		{"hyphen in a code span", "docs/a.md", "`globex-power" + ".app`\n", []int{1}},
		{"url in a code span", "docs/a.md", "`https://globex" + ".app/`\n", []int{1}},
		{"same name outside the code span", "docs/a.md", "`x` p" + ".app `y`\n", []int{1}},
		{"unclosed code span", "docs/a.md", "`p" + ".app\n", []int{1}},
		{"fenced block", "docs/a.md", "text\n```yaml\ndata: cfg.name\nhost: plc.globex" + ".de\n```\np" + ".app\n", []int{4, 6}},
		{"tilde fence", "docs/a.md", "~~~\ncfg.name\n```\ncfg.host\n~~~\n", nil},
		{"source file", "lab/sim/main.py",
			"self.args.host = cfg.name\nHOST = \"globex" + ".de\"\nURL = \"https://globex" + ".app/\"\nlogging.info(\"x\")\n", []int{2, 3}},
		{"dockerfile", "lab/images/x/Dockerfile", "RUN cfg.name\n", nil},
		{"field access in a text file", "docs/a.txt", "cfg" + ".name\n", []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := realDataFindings(tt.path, []byte(tt.text), allow)
			var lines []int
			for _, f := range got {
				lines = append(lines, f.line)
			}
			if !slices.Equal(lines, tt.want) {
				t.Errorf("findings %v, want lines %v", got, tt.want)
			}
		})
	}
}

func TestInlineCodeSpans(t *testing.T) {
	tests := []struct {
		line string
		want [][2]int
	}{
		{"no code", nil},
		{"a `b` c", [][2]int{{3, 4}}},
		{"`a` and `b`", [][2]int{{1, 2}, {9, 10}}},
		{"``a ` b`` c", [][2]int{{2, 7}}},
		{"`a`` b", nil},
		{"unclosed `a", nil},
		{"```", nil},
	}
	for _, tt := range tests {
		if got := inlineCodeSpans(tt.line); !slices.Equal(got, tt.want) {
			t.Errorf("inlineCodeSpans(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestIntranetNameInAllowlist(t *testing.T) {
	name := "build" + ".internal"
	allow, findings := parseDomainAllowlist("list.txt", []byte(name+" # a fixture host name\n"))
	if len(findings) > 0 {
		t.Fatal(findings)
	}
	ctx := lineContext{domains: true}
	if got := lineRealData("ssh "+name, ctx, allow); len(got) != 0 {
		t.Errorf("listed intranet name: got %q", got)
	}
	if got := lineRealData("ssh ci."+name, ctx, allow); len(got) != 1 {
		t.Errorf("subdomain of a listed intranet name: got %q, want 1 finding", got)
	}
}

func TestTLDList(t *testing.T) {
	lines := strings.Split(tldList, "\n")
	if !strings.HasPrefix(lines[0], "# Version ") {
		t.Errorf("first line %q is not the IANA header", lines[0])
	}
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, r := range line {
			if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				t.Errorf("tlds.txt:%d: %q is not an upper-case top-level domain", i+1, line)
				break
			}
		}
	}
	if len(rootZoneTLDs) < 1000 {
		t.Errorf("only %d top-level domains", len(rootZoneTLDs))
	}
	// Every special list names real top-level domains only.
	for _, set := range []map[string]bool{fileExtensionTLDs, classicTLDs, codeWordCountries} {
		for tld := range set {
			if !rootZoneTLDs[tld] {
				t.Errorf("%q is not in the IANA list", tld)
			}
		}
	}
	// Common file extensions that are not top-level domains need no entry.
	for _, ext := range []string{"go", "ts", "js", "yml", "yaml", "json", "txt", "mod", "sum"} {
		if rootZoneTLDs[ext] || fileExtensionTLDs[ext] {
			t.Errorf("%q: not expected in the IANA list or in the extension list", ext)
		}
	}
}
