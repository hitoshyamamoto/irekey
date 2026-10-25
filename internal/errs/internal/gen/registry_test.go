// SPDX-License-Identifier: MIT

package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureYAML is a small valid registry. It has one active code, NET-004,
// and covers every kind of exit and device state.
const fixtureYAML = `# A registry for tests.
categories:
  - prefix: USR
    title: operator input
    covers: unknown names, interruption
  - prefix: NET
    title: network and transport
    covers: unreachable, connection lost
  - prefix: ENG
    title: executor
    covers: lock held | journal damage
  - prefix: HLT
    title: health
    covers: failed check
  - prefix: RO
    title: rollouts
    covers: failure budget
  - prefix: BUG
    title: internal
    covers: unclassified error

codes:
  - code: USR-001
    category: USR
    title: unknown name
    condition: unknown device
    exit: 1
    device_state: unchanged
    kind: error
    status: reserved
    since: "0.1.0"
  - code: USR-006
    category: USR
    title: interrupted
    condition: interrupted by the operator
    exit: 130
    device_state: unchanged
    kind: error
    status: reserved
    since: "0.1.0"
  - code: NET-004
    category: NET
    title: connection lost
    condition: connection lost after device-side work started
    signal: The SSH session ends while the executor runs.
    handling: The device state is reported as unknown.
    exit: 12
    device_state: unknown
    kind: error
    status: active
    since: "0.1.0"
  - code: ENG-004
    category: ENG
    title: truncated journal line
    condition: truncated last journal line | recovered (warning)
    exit: none
    device_state: as-recovered
    kind: warning
    status: reserved
    since: "0.1.0"
  - code: HLT-003
    category: HLT
    title: check failing before the update
    condition: check already failing (informational)
    exit: none
    device_state: none
    kind: info
    status: reserved
    since: "0.2.0"
  - code: RO-002
    category: RO
    title: failure budget exceeded
    condition: failure budget exceeded
    exit: 10
    device_state: per-device
    kind: error
    status: reserved
    since: "0.1.0"
  - code: BUG-003
    category: BUG
    title: unclassified error
    condition: unclassified error (fail safe)
    exit: by-phase
    device_state: by-phase
    kind: error
    status: reserved
    since: "0.1.0"
`

func parseFixture(t *testing.T) *Registry {
	t.Helper()
	r, err := Parse([]byte(fixtureYAML))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParse(t *testing.T) {
	r := parseFixture(t)
	if len(r.Categories) != 6 || len(r.Codes) != 7 {
		t.Fatalf("parsed %d categories and %d codes", len(r.Categories), len(r.Codes))
	}
	c := r.Codes[2]
	want := Code{
		Code: "NET-004", Category: "NET", Title: "connection lost",
		Condition: "connection lost after device-side work started",
		Signal:    "The SSH session ends while the executor runs.",
		Handling:  "The device state is reported as unknown.",
		Exit:      "12", DeviceState: "unknown", Kind: "error", Status: "active", Since: "0.1.0",
		Line: 41,
	}
	if c != want {
		t.Errorf("NET-004 = %+v\nwant      %+v", c, want)
	}
	if r.Categories[0].Line != 3 || r.Codes[0].Line != 23 {
		t.Errorf("lines: first category %d, first code %d; want 3 and 23", r.Categories[0].Line, r.Codes[0].Line)
	}
	if r.Codes[len(r.Codes)-1].Exit != ByPhase {
		t.Errorf("BUG-003 exit = %q", r.Codes[len(r.Codes)-1].Exit)
	}
	if err := Validate(r); err != nil {
		t.Errorf("the fixture is invalid: %v", err)
	}
}

func TestParseIsStrict(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"empty", "", "the registry is empty"},
		{"only comments", "# nothing\n", "the registry is empty"},
		{"unknown top-level key", "categories: []\nextra: 1\n", "field extra not found"},
		{"unknown code key", "codes:\n  - code: USR-001\n    exits: 1\n", "field exits not found"},
		{"duplicate key", "codes:\n  - code: USR-001\n    code: USR-002\n", "already defined"},
		{"wrong type", "codes: 3\n", "cannot unmarshal"},
		{"syntax error", "codes: [\n", "did not find expected"},
		{"two documents", "codes: []\n---\ncodes: []\n", "a single YAML document"},
		{"broken second document", "codes: []\n---\n[\n", "a single YAML document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestParseWithoutLines(t *testing.T) {
	// A null document and null lists decode to an empty registry, with no
	// lines to record.
	for _, doc := range []string{"null\n", "categories: null\ncodes: null\n"} {
		r, err := Parse([]byte(doc))
		if err != nil || len(r.Categories) != 0 || len(r.Codes) != 0 {
			t.Errorf("Parse(%q) = %+v, %v", doc, r, err)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "codes.yaml")
	if err := os.WriteFile(good, []byte(fixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err := Load(good); err != nil || len(r.Codes) != 7 {
		t.Errorf("Load = %v, %v", r, err)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("extra: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.HasPrefix(err.Error(), bad+": ") {
		t.Errorf("Load(bad) = %v, want an error naming the file", err)
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("Load(missing) succeeded")
	}
}

func TestConstName(t *testing.T) {
	for code, want := range map[string]string{"LOC-001": "LOC001", "TIME-002": "TIME002", "ID-006": "ID006"} {
		if got := ConstName(code); got != want {
			t.Errorf("ConstName(%q) = %q, want %q", code, got, want)
		}
	}
}
