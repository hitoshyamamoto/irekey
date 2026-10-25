// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
)

const (
	spdxGo    = "// SPDX-License-Identifier: MIT"
	spdxShell = "# SPDX-License-Identifier: MIT"
)

// checkSPDX requires the license line as the first line of every Go file
// and as the second line, after the shebang, of every shell script.
// Generated Go files put it before their "Code generated" line, so they
// need no exception.
func checkSPDX(root string) ([]finding, error) {
	files, err := loadTextFiles(root, func(p string) bool {
		return strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".sh")
	})
	if err != nil {
		return nil, err
	}
	var out []finding
	for _, f := range files {
		out = append(out, spdxFindings(f.path, f.data)...)
	}
	return out, nil
}

func spdxFindings(p string, data []byte) []finding {
	lines := splitLines(data)
	line := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}
	if strings.HasSuffix(p, ".go") {
		if line(0) != spdxGo {
			return []finding{{p, 1, fmt.Sprintf("first line must be %q", spdxGo)}}
		}
		return nil
	}
	var out []finding
	if !strings.HasPrefix(line(0), "#!") {
		out = append(out, finding{p, 1, `first line must be a shebang ("#!/bin/sh")`})
	}
	if line(1) != spdxShell {
		out = append(out, finding{p, 2, fmt.Sprintf("second line must be %q", spdxShell)})
	}
	return out
}
