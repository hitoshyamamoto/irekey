// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net/netip"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// allowedDomainsFile lists tool, module and hosting domains that docs may
// name, relative to the scanned root.
const allowedDomainsFile = "scripts/guard/allowed-domains.txt"

var (
	// Maximal runs of digits and dots: a candidate IPv4 address is a whole
	// run, so versions (1.27.3) and OIDs (1.3.6.1.4.1) never match.
	ipv4Run   = regexp.MustCompile(`[0-9.]+`)
	ipv4Shape = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)
	// Maximal runs of hex digits, colons and dots (for embedded IPv4).
	ipv6Run = regexp.MustCompile(`[0-9A-Fa-f:.]+`)
	// Six hex pairs, colon or dash separated, and the dotted notation of
	// some network vendors (three groups of four hex digits).
	macColon = regexp.MustCompile(`[0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5}`)
	macDash  = regexp.MustCompile(`[0-9A-Fa-f]{2}(?:-[0-9A-Fa-f]{2}){5}`)
	macDot   = regexp.MustCompile(`[0-9A-Fa-f]{4}(?:\.[0-9A-Fa-f]{4}){2}`)
	// Dotted words, the domain candidates.
	dottedRun   = regexp.MustCompile(`[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)+`)
	domainShape = regexp.MustCompile(`^[a-z0-9-]+(?:\.[a-z0-9-]+)+$`)
	// A label made only of an identifier, as in a field access.
	plainLabel = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
)

var allowedIPv4 = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation (TEST-NET-1)
	netip.MustParsePrefix("198.51.100.0/24"), // documentation (TEST-NET-2)
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation (TEST-NET-3)
	netip.MustParsePrefix("127.0.0.0/8"),     // loopback
}

var ipv6Documentation = netip.MustParsePrefix("2001:db8::/32")

// reservedDomains are always allowed, with their subdomains: the example
// domains and the reserved top-level names of RFC 2606 and RFC 6761.
var reservedDomains = []string{
	"example.com", "example.net", "example.org",
	"example", "test", "invalid", "localhost",
}

// intranetSuffixes name private networks. They are not in the root zone,
// so they are checked on their own, and only a whole name listed in the
// allowlist is accepted.
var intranetSuffixes = []string{
	".local",       // multicast DNS (RFC 6762)
	".localdomain", // default domain of many Linux installations
	".lan", ".corp", ".internal", ".intranet",
	".home.arpa", // residential networks (RFC 8375)
}

// fileExtensionTLDs are top-level domains that are also common file
// extensions. Outside an obvious host context (a URL, an email address,
// a www. name, a path or a port after the name), a name ending in one of
// them is read as a file name. Each one is in the IANA list; a test checks
// that.
var fileExtensionTLDs = map[string]bool{
	"ac":      true, // autoconf input (configure.ac)
	"am":      true, // automake input (Makefile.am)
	"cc":      true, // C++ source
	"fish":    true, // fish shell script
	"in":      true, // template input (Makefile.in, requirements.in)
	"java":    true, // Java source
	"link":    true, // systemd network link file
	"map":     true, // source map (app.js.map)
	"md":      true, // Markdown
	"mk":      true, // make include file
	"ml":      true, // OCaml source
	"mov":     true, // QuickTime video
	"network": true, // systemd-networkd file
	"pl":      true, // Perl script
	"pm":      true, // Perl module
	"ps":      true, // PostScript
	"pub":     true, // SSH public key (id_ed25519.pub)
	"py":      true, // Python source
	"rs":      true, // Rust source
	"sh":      true, // shell script
	"so":      true, // shared library
	"target":  true, // systemd target unit (multi-user.target)
	"tf":      true, // Terraform configuration
	"work":    true, // Go workspace file (go.work)
	"zip":     true, // zip archive
}

// classicTLDs are the original generic top-level domains; in code a name
// ending in one of them is always read as a domain.
var classicTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "edu": true, "gov": true, "mil": true, "int": true,
}

// codeWordCountries are country-code top-level domains that are also
// common field names in code (x.id, opts.io, h.me, msg.to); in code they
// are treated like the generic top-level domains.
var codeWordCountries = map[string]bool{"id": true, "io": true, "me": true, "to": true}

// sourceExtensions mark source code files, where dotted lowercase words
// are mostly field accesses and method calls.
var sourceExtensions = map[string]bool{
	".go": true, ".py": true, ".sh": true, ".bash": true, ".js": true, ".mjs": true,
	".ts": true, ".c": true, ".h": true, ".rs": true, ".java": true, ".rb": true, ".lua": true,
}

// checkRealData fails on IPv4 and IPv6 addresses outside the
// documentation and loopback ranges and on globally administered MAC
// addresses in every text file, and on host names that are neither
// reserved nor allowlisted in documentation, examples, lab files, schemas
// and test data.
func checkRealData(root string) ([]finding, error) {
	data, err := readOptional(root, allowedDomainsFile)
	if err != nil {
		return nil, err
	}
	allow, out := parseDomainAllowlist(allowedDomainsFile, data)
	files, err := loadTextFiles(root, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		out = append(out, realDataFindings(f.path, f.data, allow)...)
	}
	return out, nil
}

// lineContext says how a line is scanned.
type lineContext struct {
	domains bool     // check host names
	code    bool     // the whole line is code: a source file or a fenced block
	spans   [][2]int // Markdown inline code spans, as byte offsets
}

// codeAt reports whether the byte at offset i of the line is code.
func (c lineContext) codeAt(i int) bool {
	if c.code {
		return true
	}
	for _, s := range c.spans {
		if s[0] <= i && i < s[1] {
			return true
		}
	}
	return false
}

func realDataFindings(p string, data []byte, allow domainAllowlist) []finding {
	domains, source, markdown := inDomainScope(p), isSourceFile(p), path.Ext(p) == ".md"
	var (
		out   []finding
		fence markdownFence
	)
	for i, line := range splitLines(data) {
		ctx := lineContext{domains: domains, code: source}
		if markdown {
			ctx.code = fence.line(line)
			if !ctx.code {
				ctx.spans = inlineCodeSpans(line)
			}
		}
		for _, msg := range lineRealData(line, ctx, allow) {
			out = append(out, finding{p, i + 1, msg})
		}
	}
	return out
}

// markdownFence follows fenced code blocks (``` or ~~~) through a file.
type markdownFence struct {
	open   bool
	marker byte
}

// line reports whether a line is part of a fenced block, its fence lines
// included.
func (f *markdownFence) line(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	isFence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
	switch {
	case !f.open && isFence:
		f.open, f.marker = true, trimmed[0]
		return true
	case f.open && isFence && trimmed[0] == f.marker:
		f.open = false
		return true
	}
	return f.open
}

// inlineCodeSpans returns the offsets of the Markdown code spans of a
// line: text between a run of backticks and the next run of the same
// length.
func inlineCodeSpans(line string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := backtickRun(line, i)
		closed := false
		for j := i + n; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			m := backtickRun(line, j)
			if m == n {
				spans = append(spans, [2]int{i + n, j})
				i, closed = j+m, true
				break
			}
			j += m
		}
		if !closed {
			i += n
		}
	}
	return spans
}

func backtickRun(line string, i int) int {
	n := 0
	for i+n < len(line) && line[i+n] == '`' {
		n++
	}
	return n
}

func lineRealData(line string, ctx lineContext, allow domainAllowlist) []string {
	out := ipv4Findings(line)
	out = append(out, ipv6Findings(line)...)
	out = append(out, macFindings(line)...)
	if ctx.domains {
		out = append(out, domainFindings(line, ctx, allow)...)
	}
	return out
}

// inDomainScope reports whether host names are checked in a file: the
// places where a real host name would be data rather than code.
func inDomainScope(p string) bool {
	for _, prefix := range []string{"examples/", "lab/", "docs/", "schemas/", ".github/ISSUE_TEMPLATE/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	switch {
	case p == ".github/PULL_REQUEST_TEMPLATE.md":
		return true
	case !strings.Contains(p, "/") && path.Ext(p) == ".md":
		return true
	}
	return inTestdata(p)
}

func isSourceFile(p string) bool {
	base := path.Base(p)
	return sourceExtensions[path.Ext(p)] || strings.HasPrefix(base, "Dockerfile") || base == "Makefile"
}

func ipv4Findings(line string) []string {
	var out []string
	for _, s := range ipv4Run.FindAllString(line, -1) {
		// A trailing dot ends a sentence; a leading one means the run
		// continues something else, so it is not trimmed.
		s = strings.TrimRight(s, ".")
		addr, ok := parseIPv4(s)
		if !ok || ipv4Allowed(addr) {
			continue
		}
		out = append(out, ipv4Message(s))
	}
	return out
}

// parseIPv4 parses a dotted quad. Unlike netip.ParseAddr it accepts
// leading zeros (192.000.002.001), which many tools print and accept.
func parseIPv4(s string) (netip.Addr, bool) {
	if !ipv4Shape.MatchString(s) {
		return netip.Addr{}, false
	}
	parts := strings.Split(s, ".")
	for i, part := range parts {
		if trimmed := strings.TrimLeft(part, "0"); trimmed != "" {
			parts[i] = trimmed
		} else {
			parts[i] = "0"
		}
	}
	addr, err := netip.ParseAddr(strings.Join(parts, "."))
	return addr, err == nil
}

func ipv4Message(s string) string {
	return fmt.Sprintf("IPv4 address %s is outside the documentation and loopback ranges "+
		"(use 192.0.2.0/24, 198.51.100.0/24 or 203.0.113.0/24)", s)
}

func ipv4Allowed(addr netip.Addr) bool {
	for _, p := range allowedIPv4 {
		if p.Contains(addr) {
			return true
		}
	}
	return isNetmask(addr)
}

// isNetmask reports whether addr is 0.0.0.0 or a contiguous netmask of
// at least 8 bits (255.0.0.0 to 255.255.255.255). Shorter masks are also
// ordinary public addresses, and are reported.
func isNetmask(addr netip.Addr) bool {
	b := addr.As4()
	if b == [4]byte{} {
		return true
	}
	if b[0] != 255 {
		return false
	}
	host := ^(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
	return host&(host+1) == 0
}

func ipv6Findings(line string) []string {
	var out []string
	for _, loc := range ipv6Run.FindAllStringIndex(line, -1) {
		s, addr, ok := ipv6Address(line, loc[0], loc[1])
		if !ok {
			continue
		}
		if addr.Is4In6() {
			// The dotted form is checked as IPv4 on its own.
			if !strings.Contains(s, ".") && !ipv4Allowed(addr.Unmap()) {
				out = append(out, ipv4Message(s))
			}
			continue
		}
		if !ipv6Allowed(addr) {
			out = append(out, fmt.Sprintf("IPv6 address %s is outside the documentation "+
				"range and loopback (use 2001:db8::/32)", s))
		}
	}
	return out
}

// ipv6Address finds an IPv6 address in the hex, colon and dot run
// line[start:end]. When the whole run is not one (it may go on into the
// next sentence, as in "2001:db8::1.Done"), the part before its first dot
// is tried.
func ipv6Address(line string, start, end int) (string, netip.Addr, bool) {
	ends := []int{end}
	if i := strings.IndexByte(line[start:end], '.'); i > 0 {
		ends = append(ends, start+i)
	}
	for _, e := range ends {
		s, ok := ipv6Candidate(line, start, e)
		if !ok {
			continue
		}
		if addr, err := netip.ParseAddr(s); err == nil {
			return s, addr, true
		}
	}
	return "", netip.Addr{}, false
}

// ipv6Candidate trims a hex, colon and dot run down to a possible IPv6
// address, or reports that it cannot be one. A single colon at either end
// is punctuation ("ip:2001:db8::1", "at 10:30:"), a double colon is part
// of the address, and a trailing dot ends a sentence. The candidate must
// not touch a word character, so that "std::string" and "Acquire::Retries"
// are not read as addresses.
func ipv6Candidate(line string, start, end int) (string, bool) {
	s := strings.TrimRight(line[start:end], ".")
	end = start + len(s)
	if strings.HasPrefix(s, ":") && !strings.HasPrefix(s, "::") {
		s, start = s[1:], start+1
	}
	if strings.HasSuffix(s, ":") && !strings.HasSuffix(s, "::") {
		s, end = s[:len(s)-1], end-1
	}
	if strings.Count(s, ":") < 2 {
		return "", false
	}
	if start > 0 && isWordByte(line[start-1]) {
		return "", false
	}
	if end < len(line) && isWordByte(line[end]) {
		return "", false
	}
	return s, true
}

func ipv6Allowed(addr netip.Addr) bool {
	return ipv6Documentation.Contains(addr) || addr == netip.IPv6Loopback() || addr == netip.IPv6Unspecified()
}

// isWordByte reports whether b can be part of an identifier. Bytes of
// multi-byte UTF-8 sequences count, as they are usually letters.
func isWordByte(b byte) bool {
	return b == '_' || b >= 0x80 ||
		('0' <= b && b <= '9') || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

func isHexByte(b byte) bool {
	return ('0' <= b && b <= '9') || ('a' <= b && b <= 'f') || ('A' <= b && b <= 'F')
}

func macFindings(line string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{macColon, macDash, macDot} {
		for _, loc := range re.FindAllStringIndex(line, -1) {
			start, end := loc[0], loc[1]
			if partOfLongerHexGroup(line, start, end) {
				continue
			}
			first, _ := strconv.ParseUint(line[start:start+2], 16, 8)
			if first&0x02 != 0 {
				continue // locally administered
			}
			out = append(out, fmt.Sprintf("MAC address %s is globally administered "+
				"(use a locally administered address such as 02:00:00:00:00:01)", line[start:end]))
		}
	}
	return out
}

// partOfLongerHexGroup reports whether line[start:end] touches another hex
// digit or another ":hex", "-hex" or ".hex" group, as in a key fingerprint
// or a longer hex dump.
func partOfLongerHexGroup(line string, start, end int) bool {
	separator := func(b byte) bool { return b == ':' || b == '-' || b == '.' }
	if start > 0 {
		b := line[start-1]
		if isHexByte(b) || (separator(b) && start > 1 && isHexByte(line[start-2])) {
			return true
		}
	}
	if end < len(line) {
		b := line[end]
		if isHexByte(b) || (separator(b) && end+1 < len(line) && isHexByte(line[end+1])) {
			return true
		}
	}
	return false
}

// domainFindings reports the host names of a line that are neither
// reserved nor allowlisted. A dotted word is read as a host name when:
//
//   - it contains no underscore (host names have none), and its last label
//     does not read as an exported Go identifier ("slog.Info",
//     "errors.As"); all-capital names such as WWW.EXAMPLE.COM are checked,
//     case-insensitively;
//   - it ends in an intranet suffix (always reported), or in a top-level
//     domain of the IANA root zone;
//   - it is not followed by "(" or "[", which make it a call or an index;
//   - for top-level domains that are also file extensions (md, sh, py,
//     ...), it is in a host context: after "://" or "@", starting with
//     "www.", or followed by "/" or ":port";
//   - in code (a source file, a Markdown code span or fenced block), a
//     chain of plain identifiers ("cfg.name", "paths.data", "opts.io") is
//     a field access when it is not in a host context and ends in a
//     generic top-level domain other than com, net, org, edu, gov, mil
//     and int, or in id, io, me or to. Other country codes (globex.de)
//     are still checked in code.
func domainFindings(line string, ctx lineContext, allow domainAllowlist) []string {
	var out []string
	for _, loc := range dottedRun.FindAllStringIndex(line, -1) {
		if msg := domainFinding(line, loc[0], loc[1], ctx, allow); msg != "" {
			out = append(out, msg)
		}
	}
	return out
}

func domainFinding(line string, start, end int, ctx lineContext, allow domainAllowlist) string {
	raw := line[start:end]
	if strings.Contains(raw, "_") {
		return ""
	}
	labels := strings.Split(raw, ".")
	if exportedIdentifier(labels[len(labels)-1]) {
		return ""
	}
	name := strings.ToLower(raw)
	if !domainShape.MatchString(name) {
		return ""
	}
	if suffix := intranetSuffix(name); suffix != "" {
		if allow.lists(name) {
			return ""
		}
		return fmt.Sprintf("intranet host name %s (%s) is not allowed (use a name under example.com)", name, suffix)
	}
	tld := name[strings.LastIndexByte(name, '.')+1:]
	if !rootZoneTLDs[tld] {
		return ""
	}
	if end < len(line) && (line[end] == '(' || line[end] == '[') {
		return "" // a call or an index
	}
	host := hostContext(line, start, end, name)
	if fileExtensionTLDs[tld] && !host {
		return ""
	}
	if !host && ctx.codeAt(start) && fieldAccess(name, tld) {
		return ""
	}
	if allow.allows(name) {
		return ""
	}
	return fmt.Sprintf("domain %s is not allowed (use example.com, or add a tool "+
		"or hosting domain with a comment to %s)", name, allowedDomainsFile)
}

// exportedIdentifier reports whether a label reads as an exported Go
// identifier: an upper-case letter followed by a lower-case one.
func exportedIdentifier(label string) bool {
	return len(label) >= 2 && 'A' <= label[0] && label[0] <= 'Z' && 'a' <= label[1] && label[1] <= 'z'
}

func intranetSuffix(name string) string {
	for _, s := range intranetSuffixes {
		if strings.HasSuffix(name, s) || name == s[1:] {
			return s
		}
	}
	return ""
}

// hostContext reports whether the text around a name makes it a host
// name: a URL, an email address, a www. name, or a path or port after it.
func hostContext(line string, start, end int, name string) bool {
	switch {
	case strings.HasSuffix(line[:start], "://"), start > 0 && line[start-1] == '@',
		strings.HasPrefix(name, "www."):
		return true
	case end < len(line) && line[end] == '/':
		return true
	case end+1 < len(line) && line[end] == ':' && '0' <= line[end+1] && line[end+1] <= '9':
		return true
	}
	return false
}

// fieldAccess reports whether a name, seen in code, reads as a field
// access rather than as a host name.
func fieldAccess(name, tld string) bool {
	for l := range strings.SplitSeq(name, ".") {
		if !plainLabel.MatchString(l) {
			return false
		}
	}
	return (len(tld) > 2 && !classicTLDs[tld]) || codeWordCountries[tld]
}

// domainAllowlist holds the allowlisted domains; each one also allows its
// subdomains.
type domainAllowlist struct {
	domains []string
}

func (a domainAllowlist) allows(name string) bool {
	for _, lists := range [][]string{reservedDomains, a.domains} {
		for _, d := range lists {
			if name == d || strings.HasSuffix(name, "."+d) {
				return true
			}
		}
	}
	return false
}

// lists reports whether name itself is an allowlist entry.
func (a domainAllowlist) lists(name string) bool {
	for _, d := range a.domains {
		if name == d {
			return true
		}
	}
	return false
}

// parseDomainAllowlist reads lines of the form "domain # why". Blank lines
// and full-line comments are allowed; an entry without a comment, or that
// is not a lowercase domain name, is a finding.
func parseDomainAllowlist(p string, data []byte) (domainAllowlist, []finding) {
	var (
		allow domainAllowlist
		out   []finding
	)
	for i, raw := range splitLines(data) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, comment, found := strings.Cut(line, "#")
		entry = strings.TrimSpace(entry)
		switch {
		case !domainShape.MatchString(entry):
			out = append(out, finding{p, i + 1, fmt.Sprintf("%q is not a lowercase domain name", entry)})
		case !found || strings.TrimSpace(comment) == "":
			out = append(out, finding{p, i + 1, "entry " + entry + " needs a comment saying why it is allowed"})
		default:
			allow.domains = append(allow.domains, entry)
		}
	}
	return allow, out
}
