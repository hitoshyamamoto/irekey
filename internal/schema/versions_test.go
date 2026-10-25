// SPDX-License-Identifier: MIT

package schema

import (
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// versionsFile lists every schema identifier this build reads.
const versionsFile = "../../docs/schema-versions.txt"

// idPattern is the form of the schema: identifier of a deployment
// repository or component file, irekey/<kind>/v<N>, where N counts from 1.
var idPattern = regexp.MustCompile(`^irekey/[a-z][a-z0-9-]*/v[1-9][0-9]*$`)

// splitVersions separates the header of the versions file, the leading
// comment and blank lines kept verbatim, from the identifiers it lists.
// Comment and blank lines after the first identifier are dropped.
func splitVersions(text string) (header string, ids []string) {
	inHeader := true
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		comment := trimmed == "" || strings.HasPrefix(trimmed, "#")
		if inHeader && comment {
			header += line
			continue
		}
		inHeader = false
		if !comment {
			ids = append(ids, trimmed)
		}
	}
	return header, ids
}

// formatVersions renders the versions file from its header and identifiers,
// with a blank line between them.
func formatVersions(header string, ids []string) string {
	var b strings.Builder
	b.WriteString(header)
	if header != "" && !strings.HasSuffix(header, "\n") {
		b.WriteByte('\n')
	}
	if header != "" && len(ids) > 0 && !lastLineBlank(header) {
		b.WriteByte('\n')
	}
	for _, id := range ids {
		b.WriteString(id)
		b.WriteByte('\n')
	}
	return b.String()
}

// lastLineBlank reports whether the last line of header is blank.
func lastLineBlank(header string) bool {
	body := strings.TrimSuffix(header, "\n")
	return strings.TrimSpace(body[strings.LastIndexByte(body, '\n')+1:]) == ""
}

func TestVersionsFileListsSupportedSchemas(t *testing.T) {
	data, err := os.ReadFile(versionsFile)
	if err != nil {
		t.Fatal(err)
	}
	header, listed := splitVersions(string(data))
	if header == "" {
		t.Errorf("%s has lost its header comment; restore it from git", versionsFile)
	}
	want := Supported()
	if *update {
		data = []byte(formatVersions(header, want))
		if err := os.WriteFile(versionsFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, listed = splitVersions(string(data))
	}
	if !slices.Equal(listed, want) {
		t.Errorf("%s lists %q, but this build reads %q (run go test ./internal/schema -update)",
			versionsFile, listed, want)
	}
}

func TestSupportedIsSortedUniqueAndWellFormed(t *testing.T) {
	ids := Supported()
	if ids == nil {
		t.Fatal("Supported returned nil; want a non-nil slice")
	}
	if !slices.IsSorted(ids) {
		t.Errorf("Supported returned %q, which is not sorted", ids)
	}
	for i, id := range ids {
		if !idPattern.MatchString(id) {
			t.Errorf("schema identifier %q does not have the form irekey/<kind>/v<N>", id)
		}
		if i > 0 && ids[i-1] == id {
			t.Errorf("schema identifier %q is listed twice", id)
		}
	}
}

func TestSupportedReturnsASortedCopy(t *testing.T) {
	saved := supported
	t.Cleanup(func() { supported = saved })
	supported = []string{"irekey/repo/v1", "irekey/device/v1"}

	got := Supported()
	if want := []string{"irekey/device/v1", "irekey/repo/v1"}; !slices.Equal(got, want) {
		t.Fatalf("Supported() = %q, want %q", got, want)
	}
	got[0] = "changed"
	if again := Supported(); again[0] != "irekey/device/v1" {
		t.Errorf("changing the result of Supported changed the next result: %q", again)
	}
	if supported[0] != "irekey/repo/v1" {
		t.Errorf("Supported reordered its source list: %q", supported)
	}
}

func TestIDPattern(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"irekey/repo/v1", true},
		{"irekey/release-set/v1", true},
		{"irekey/component/v1", true},
		{"irekey/device/v12", true},
		{"irekey/device/1", false},
		{"irekey/device/v01", false},
		{"irekey/device/v0", false},
		{"irekey/Device/v1", false},
		{"irekey//v1", false},
		{"irekey/-device/v1", false},
		{"device/v1", false},
		{"irekey/device/v1/extra", false},
		{"irekey/device/v1 ", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := idPattern.MatchString(tt.id); got != tt.want {
			t.Errorf("idPattern.MatchString(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestVersionsFileFormat(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantHeader string
		wantIDs    []string
		wantOutput string // formatVersions(header, ids); empty means text
	}{
		{
			name: "empty file",
		},
		{
			name:       "header only",
			text:       "# a\n#\n# b\n",
			wantHeader: "# a\n#\n# b\n",
		},
		{
			name:       "header without final newline",
			text:       "# a",
			wantHeader: "# a",
			wantOutput: "# a\n",
		},
		{
			name:       "identifiers after the header",
			text:       "# a\n\nirekey/device/v1\nirekey/repo/v1\n",
			wantHeader: "# a\n\n",
			wantIDs:    []string{"irekey/device/v1", "irekey/repo/v1"},
		},
		{
			name:       "identifiers without a header",
			text:       "irekey/repo/v1\n",
			wantIDs:    []string{"irekey/repo/v1"},
			wantOutput: "irekey/repo/v1\n",
		},
		{
			name:       "comments after the first identifier are dropped",
			text:       "# a\nirekey/device/v1\n\n# b\nirekey/repo/v1\n",
			wantHeader: "# a\n",
			wantIDs:    []string{"irekey/device/v1", "irekey/repo/v1"},
			wantOutput: "# a\n\nirekey/device/v1\nirekey/repo/v1\n",
		},
		{
			name:       "carriage returns and spaces are trimmed",
			text:       "# a\r\n  irekey/repo/v1 \r\n",
			wantHeader: "# a\r\n",
			wantIDs:    []string{"irekey/repo/v1"},
			wantOutput: "# a\r\n\nirekey/repo/v1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, ids := splitVersions(tt.text)
			if header != tt.wantHeader {
				t.Errorf("header = %q, want %q", header, tt.wantHeader)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ids = %q, want %q", ids, tt.wantIDs)
			}
			want := tt.wantOutput
			if want == "" {
				want = tt.text
			}
			got := formatVersions(header, ids)
			if got != want {
				t.Errorf("formatVersions = %q, want %q", got, want)
			}
			// Rewriting the rendered file changes nothing.
			if again := formatVersions(splitVersions(got)); again != got {
				t.Errorf("formatVersions is not idempotent: %q, then %q", got, again)
			}
		})
	}
}
