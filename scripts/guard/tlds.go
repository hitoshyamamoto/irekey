// SPDX-License-Identifier: MIT

package main

import (
	_ "embed"
	"strings"
)

// tldList is the IANA list of top-level domains, as published (one
// upper-case name per line, "#" comment lines first). It is embedded so
// that every scanned tree uses the same list.
//
//go:embed tlds.txt
var tldList string

// rootZoneTLDs holds the lower-cased top-level domains of the root zone.
var rootZoneTLDs = parseTLDs(tldList)

func parseTLDs(list string) map[string]bool {
	tlds := map[string]bool{}
	for line := range strings.SplitSeq(list, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tlds[strings.ToLower(line)] = true
	}
	return tlds
}
