// SPDX-License-Identifier: MIT

package gen

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// code returns a pointer to the fixture code named c.
func code(t *testing.T, r *Registry, c string) *Code {
	t.Helper()
	i := slices.IndexFunc(r.Codes, func(x Code) bool { return x.Code == c })
	if i < 0 {
		t.Fatalf("the fixture has no code %s", c)
	}
	return &r.Codes[i]
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, r *Registry)
		want   string // a substring of the only problem
	}{
		// Categories.
		{"no categories", func(_ *testing.T, r *Registry) { r.Categories, r.Codes = nil, nil },
			"the registry lists no categories"},
		{"bad prefix", func(_ *testing.T, r *Registry) {
			r.Categories = append(r.Categories, Category{Prefix: "Abc", Title: "t", Covers: "c"})
		},
			`category prefix "Abc" must be two to five capital letters`},
		{"long prefix", func(_ *testing.T, r *Registry) {
			r.Categories = append(r.Categories, Category{Prefix: "ABCDEF", Title: "t", Covers: "c"})
		}, `category prefix "ABCDEF"`},
		{"duplicate category", func(_ *testing.T, r *Registry) { r.Categories = append(r.Categories, r.Categories[0]) },
			"category USR is listed twice"},
		{"category without title", func(_ *testing.T, r *Registry) { r.Categories[1].Title = " " },
			"category NET: title is required"},
		{"category covers on two lines", func(_ *testing.T, r *Registry) { r.Categories[1].Covers = "a\nb" },
			"category NET: covers must be a single line"},

		// Identity and order.
		{"malformed code", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Code = "USR-1" },
			`code "USR-1" does not have the form <CATEGORY>-<NNN>`},
		{"lower-case code", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Code = "usr-001" },
			`code "usr-001" does not have the form`},
		{"four digits", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Code = "USR-0001" },
			`code "USR-0001" does not have the form`},
		{"unknown category", func(t *testing.T, r *Registry) {
			c := code(t, r, "USR-001")
			c.Code, c.Category = "XYZ-001", "XYZ"
		}, `XYZ-001: unknown category "XYZ"`},
		{"code outside its category", func(t *testing.T, r *Registry) { code(t, r, "USR-006").Category = "NET" },
			"USR-006: the code does not start with its category NET"},
		{"duplicate code", func(t *testing.T, r *Registry) { code(t, r, "USR-006").Code = "USR-001" },
			"USR-001 is registered twice"},
		{"numbers out of order", func(_ *testing.T, r *Registry) { r.Codes[0], r.Codes[1] = r.Codes[1], r.Codes[0] },
			"USR-001 is out of order"},
		{"categories out of order", func(_ *testing.T, r *Registry) {
			r.Categories[0], r.Categories[1] = r.Categories[1], r.Categories[0]
		}, "NET-004 is out of order"},

		// Fields.
		{"no title", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Title = "" }, "USR-001: title is required"},
		{"no condition", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Condition = "" },
			"USR-001: condition is required"},
		{"condition on two lines", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Condition = "a\r\nb" },
			"USR-001: condition must be a single line"},
		{"active without signal", func(t *testing.T, r *Registry) { code(t, r, "NET-004").Signal = "" },
			"NET-004: signal is required"},
		{"active without handling", func(t *testing.T, r *Registry) { code(t, r, "NET-004").Handling = "" },
			"NET-004: handling is required"},
		{"reserved handling on two lines", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Handling = "a\nb" },
			"USR-001: handling must be a single line"},
		{"unknown kind", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Kind = "fatal" },
			`USR-001: kind "fatal" must be one of error, warning, info`},
		{"unknown status", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Status = "deprecated" },
			`USR-001: status "deprecated" must be one of reserved, active`},
		{"no since", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Since = "" }, `USR-001: since ""`},
		{"bad since", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Since = "v0.1" }, `USR-001: since "v0.1"`},

		// Exit codes.
		{"exit not a number", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "one" },
			`USR-001: exit "one" must be by-phase, none or one of the exit codes 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15, 130`},
		{"exit empty", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "" }, `USR-001: exit ""`},
		{"exit with a leading zero", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "01" }, `USR-001: exit "01"`},
		{"exit 0 is success", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "0" }, `USR-001: exit "0"`},
		{"exit 9 is an operation result", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "9" }, `USR-001: exit "9"`},
		{"exit that does not exist", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "16" }, `USR-001: exit "16"`},
		{"warning with an exit code", func(t *testing.T, r *Registry) {
			c := code(t, r, "ENG-004")
			c.Exit, c.DeviceState = "5", "unchanged"
		}, "ENG-004: a code of kind warning ends no command, so its exit must be none, not 5"},
		{"information set by phase", func(t *testing.T, r *Registry) {
			c := code(t, r, "HLT-003")
			c.Exit, c.DeviceState = ByPhase, ByPhase
		}, "HLT-003: a code of kind info ends no command, so its exit must be none, not by-phase"},
		{"exit none for an error", func(t *testing.T, r *Registry) {
			c := code(t, r, "USR-001")
			c.Exit, c.DeviceState = None, None
		}, "USR-001: an error must end the command with an exit code"},

		// Device states.
		{"unknown device state", func(t *testing.T, r *Registry) { code(t, r, "USR-001").DeviceState = "changed" },
			`USR-001: device_state "changed" must be one of unchanged, previous-release, new-release, unknown, needs-intervention, by-phase, per-device, as-recovered, none`},
		{"by-phase exit without by-phase state", func(t *testing.T, r *Registry) { code(t, r, "BUG-003").DeviceState = "unknown" },
			"BUG-003: exit by-phase needs device_state by-phase, not unknown"},
		{"by-phase state without by-phase exit", func(t *testing.T, r *Registry) { code(t, r, "BUG-003").Exit = "15" },
			"BUG-003: device_state by-phase needs exit by-phase, not 15"},
		{"by-phase state with exit none", func(t *testing.T, r *Registry) {
			c := code(t, r, "ENG-004")
			c.DeviceState = ByPhase
		}, "ENG-004: device_state by-phase needs exit by-phase, not none"},
		{"exit none with a device state", func(t *testing.T, r *Registry) { code(t, r, "HLT-003").DeviceState = "unchanged" },
			"HLT-003: exit none needs device_state none or as-recovered, not unchanged"},
		{"exit 7 needs previous-release", func(t *testing.T, r *Registry) { code(t, r, "USR-001").Exit = "7" },
			"USR-001: exit 7 (failed after the switch; rolled back successfully) needs device_state previous-release, not unchanged"},
		{"exit 12 needs unknown", func(t *testing.T, r *Registry) { code(t, r, "NET-004").DeviceState = "unchanged" },
			"NET-004: exit 12 (outcome unknown: contact lost after device-side work started) needs device_state unknown, not unchanged"},
		{"exit 10 needs per-device", func(t *testing.T, r *Registry) { code(t, r, "RO-002").DeviceState = "unknown" },
			"RO-002: exit 10 (halted: failure budget exceeded) needs device_state per-device, not unknown"},
		{"exit 130 allows unchanged or unknown", func(t *testing.T, r *Registry) { code(t, r, "USR-006").DeviceState = "new-release" },
			"USR-006: exit 130 (interrupted by the operator; device-side work already started continues) needs device_state unchanged or unknown, not new-release"},
		{"exit 1 with a rule state", func(t *testing.T, r *Registry) { code(t, r, "USR-001").DeviceState = AsRecovered },
			"USR-001: exit 1 (usage or input error) needs device_state unchanged, not as-recovered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := parseFixture(t)
			tt.change(t, r)
			err := Validate(r)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate = %v, want a *ValidationError", err)
			}
			if len(ve.Problems) != 1 || !strings.Contains(ve.Problems[0].Msg, tt.want) {
				t.Errorf("problems = %+v\nwant one containing %q", ve.Problems, tt.want)
			}
		})
	}
}

func TestValidateAcceptsTheAllowedPairs(t *testing.T) {
	r := parseFixture(t)
	c := code(t, r, "USR-006")
	c.DeviceState = "unknown" // exit 130 allows unchanged or unknown
	h := code(t, r, "HLT-003")
	h.DeviceState = AsRecovered // exit none allows none or as-recovered
	if err := Validate(r); err != nil {
		t.Error(err)
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	r := parseFixture(t)
	r.Codes[0].Kind = "fatal"
	r.Codes[0].Exit = "99"
	r.Codes[0].DeviceState = "changed"
	r.Codes[1].Since = ""
	err := Validate(r)
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 4 {
		t.Fatalf("Validate = %v, want 4 problems", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"invalid registry:\n",
		"line 23: USR-001: kind",
		"line 23: USR-001: device_state",
		"line 23: USR-001: exit",
		"line 32: USR-006: since",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	empty := &ValidationError{Problems: []Problem{{Msg: "the registry lists no categories"}}}
	if got := empty.Error(); got != "invalid registry:\nthe registry lists no categories" {
		t.Errorf("Error() = %q", got)
	}
}
