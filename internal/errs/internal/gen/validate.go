// SPDX-License-Identifier: MIT

package gen

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	codePattern    = regexp.MustCompile(`^[A-Z]{2,5}-\d{3}$`)
	prefixPattern  = regexp.MustCompile(`^[A-Z]{2,5}$`)
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Problem is one reason a registry is invalid.
type Problem struct {
	// Line is the line in codes.yaml of the entry concerned, or 0 when the
	// problem concerns the whole registry.
	Line int
	Msg  string
}

// ValidationError lists every problem of an invalid registry.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		if p.Line == 0 {
			lines[i] = p.Msg
		} else {
			lines[i] = fmt.Sprintf("line %d: %s", p.Line, p.Msg)
		}
	}
	return "invalid registry:\n" + strings.Join(lines, "\n")
}

// Validate checks a registry and returns a *ValidationError listing every
// problem, or nil when the registry is valid.
func Validate(r *Registry) error {
	if problems := validate(r); len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// validate returns every problem of a registry.
func validate(r *Registry) []Problem {
	v := validator{}
	v.categories(r.Categories)
	v.codes(r)
	return v.problems
}

type validator struct {
	problems []Problem
}

func (v *validator) add(line int, format string, args ...any) {
	v.problems = append(v.problems, Problem{Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (v *validator) categories(cats []Category) {
	if len(cats) == 0 {
		v.add(0, "the registry lists no categories")
	}
	seen := map[string]bool{}
	for _, c := range cats {
		switch {
		case !prefixPattern.MatchString(c.Prefix):
			v.add(c.Line, "category prefix %q must be two to five capital letters", c.Prefix)
		case seen[c.Prefix]:
			v.add(c.Line, "category %s is listed twice", c.Prefix)
		}
		seen[c.Prefix] = true
		v.text(c.Line, "category "+c.Prefix, "title", c.Title, true)
		v.text(c.Line, "category "+c.Prefix, "covers", c.Covers, true)
	}
}

// text checks a free-text field: one line, and present when required.
func (v *validator) text(line int, who, field, value string, required bool) {
	switch {
	case required && strings.TrimSpace(value) == "":
		v.add(line, "%s: %s is required", who, field)
	case strings.ContainsAny(value, "\r\n"):
		v.add(line, "%s: %s must be a single line", who, field)
	}
}

func (v *validator) codes(r *Registry) {
	order := map[string]int{}
	for i, c := range r.Categories {
		if _, ok := order[c.Prefix]; !ok {
			order[c.Prefix] = i
		}
	}
	seen := map[string]bool{}
	// prev is the sort key of the previous well-formed code.
	prev, havePrev := [2]int{}, false
	for _, c := range r.Codes {
		key, ok := v.identity(c, order)
		if ok {
			if seen[c.Code] {
				v.add(c.Line, "%s is registered twice", c.Code)
			} else if havePrev && compareKeys(key, prev) < 0 {
				v.add(c.Line, "%s is out of order: codes are sorted by category, in the order of the categories, then by number", c.Code)
			}
			seen[c.Code] = true
			prev, havePrev = key, true
		}
		v.fields(c)
		v.exitAndState(c)
	}
}

// identity checks the code and its category, and returns the sort key of a
// well-formed code: the index of its category and its number.
func (v *validator) identity(c Code, order map[string]int) ([2]int, bool) {
	if !codePattern.MatchString(c.Code) {
		v.add(c.Line, "code %q does not have the form <CATEGORY>-<NNN> (two to five capital letters, a dash, three digits)", c.Code)
		return [2]int{}, false
	}
	prefix, num, _ := strings.Cut(c.Code, "-")
	idx, known := order[c.Category]
	switch {
	case !known:
		v.add(c.Line, "%s: unknown category %q", c.Code, c.Category)
		return [2]int{}, false
	case prefix != c.Category:
		v.add(c.Line, "%s: the code does not start with its category %s", c.Code, c.Category)
		return [2]int{}, false
	}
	n, _ := strconv.Atoi(num) // three digits, by the pattern
	return [2]int{idx, n}, true
}

func compareKeys(a, b [2]int) int {
	if a[0] != b[0] {
		return a[0] - b[0]
	}
	return a[1] - b[1]
}

func (v *validator) fields(c Code) {
	v.text(c.Line, c.Code, "title", c.Title, true)
	v.text(c.Line, c.Code, "condition", c.Condition, true)
	active := c.Status == Active
	v.text(c.Line, c.Code, "signal", c.Signal, active)
	v.text(c.Line, c.Code, "handling", c.Handling, active)
	if !slices.Contains(Kinds, c.Kind) {
		v.add(c.Line, "%s: kind %q must be one of %s", c.Code, c.Kind, strings.Join(Kinds, ", "))
	}
	if !slices.Contains(Statuses, c.Status) {
		v.add(c.Line, "%s: status %q must be one of %s", c.Code, c.Status, strings.Join(Statuses, ", "))
	}
	if !versionPattern.MatchString(c.Since) {
		v.add(c.Line, "%s: since %q must be a version such as 0.1.0", c.Code, c.Since)
	}
}

// exitAndState checks the exit and device_state fields and that they agree
// with each other and with the kind.
func (v *validator) exitAndState(c Code) {
	stateOK := slices.Contains(stateValues(), c.DeviceState)
	if !stateOK {
		v.add(c.Line, "%s: device_state %q must be one of %s", c.Code, c.DeviceState, strings.Join(stateValues(), ", "))
	}
	var def ExitDef
	if c.Exit != ByPhase && c.Exit != None {
		n, err := strconv.Atoi(c.Exit)
		d, known := exitDef(n)
		if err != nil || strconv.Itoa(n) != c.Exit || !known || len(d.Allowed) == 0 {
			v.add(c.Line, "%s: exit %q must be by-phase, none or one of the exit codes %s", c.Code, c.Exit, codeExits())
			return
		}
		def = d
	}
	switch {
	case c.Exit == None && c.Kind == KindError:
		v.add(c.Line, "%s: an error must end the command with an exit code; exit none is for warnings and information", c.Code)
	case c.Exit != None && c.Kind != KindError && slices.Contains(Kinds, c.Kind):
		v.add(c.Line, "%s: a code of kind %s ends no command, so its exit must be none, not %s", c.Code, c.Kind, c.Exit)
	}
	if !stateOK {
		return
	}
	switch {
	case c.Exit == ByPhase && c.DeviceState != ByPhase:
		v.add(c.Line, "%s: exit by-phase needs device_state by-phase, not %s", c.Code, c.DeviceState)
	case c.Exit != ByPhase && c.DeviceState == ByPhase:
		v.add(c.Line, "%s: device_state by-phase needs exit by-phase, not %s", c.Code, c.Exit)
	case c.Exit == None && !slices.Contains(NoneAllowed, c.DeviceState):
		v.add(c.Line, "%s: exit none needs device_state %s, not %s", c.Code, strings.Join(NoneAllowed, " or "), c.DeviceState)
	case c.Exit != ByPhase && c.Exit != None && !slices.Contains(def.Allowed, c.DeviceState):
		v.add(c.Line, "%s: exit %d (%s) needs device_state %s, not %s",
			c.Code, def.Code, def.Meaning, strings.Join(def.Allowed, " or "), c.DeviceState)
	}
}

// codeExits lists the exit codes a registered code may carry.
func codeExits() string {
	var out []string
	for _, d := range Exits {
		if len(d.Allowed) > 0 {
			out = append(out, strconv.Itoa(d.Code))
		}
	}
	return strings.Join(out, ", ")
}
