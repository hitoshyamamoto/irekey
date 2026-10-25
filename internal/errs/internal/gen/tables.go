// SPDX-License-Identifier: MIT

package gen

import "slices"

// ExitDef is one exit code of irekey, as the specification's exit-code
// table defines it.
type ExitDef struct {
	Code int
	// Meaning and State are the columns of the exit-code table.
	Meaning string
	State   string
	// Allowed lists the device_state values a registered code with this
	// exit code may declare. It is empty for the exit codes that describe a
	// whole operation (success, mixed results) rather than one error, which
	// no registered code carries.
	Allowed []string
}

// Exits are the exit codes of irekey, in order. Package errs declares a
// constant for each; a test checks that the two agree.
var Exits = []ExitDef{
	{0, "success", "as requested", nil},
	{1, "usage or input error", "unchanged", []string{"unchanged"}},
	{2, "device unreachable", "unchanged", []string{"unchanged"}},
	{3, "identity mismatch", "unchanged", []string{"unchanged"}},
	{4, "verification failed (signature, format, schema, gate)", "unchanged", []string{"unchanged"}},
	{5, "pre-flight blocked (disk, drift, downtime, platform)", "unchanged", []string{"unchanged"}},
	{6, "aborted before the switch", "unchanged", []string{"unchanged"}},
	{7, "failed after the switch; rolled back successfully", "previous release set", []string{"previous-release"}},
	{8, "failed after the switch; rollback failed, or state ambiguous", "needs-intervention", []string{"needs-intervention"}},
	{9, "multi-device operation finished with mixed results", "see the JSON report", nil},
	{10, "halted: failure budget exceeded", "per device; see the report", []string{"per-device"}},
	{11, "authentication failed", "unchanged", []string{"unchanged"}},
	{12, "outcome unknown: contact lost after device-side work started", "unknown until reconciled", []string{"unknown"}},
	{13, "environment error (laptop, deployment repository, artifact sources)", "unchanged", []string{"unchanged"}},
	{14, "device busy (lock held)", "unchanged", []string{"unchanged"}},
	{15, "internal error (crash report written)", "see the report", []string{"unknown"}},
	{130, "interrupted by the operator; device-side work already started continues", "unchanged or unknown", []string{"unchanged", "unknown"}},
}

// Values of the exit and device_state fields that are rules rather than an
// exit code or a device state.
const (
	ByPhase     = "by-phase"
	None        = "none"
	PerDevice   = "per-device"
	AsRecovered = "as-recovered"
)

// NoneAllowed lists the device_state values of a code whose exit is none.
var NoneAllowed = []string{None, AsRecovered}

// Def is a named value with its meaning.
type Def struct {
	Name    string
	Meaning string
}

// DeviceStates are the states an error leaves a device in, in the order of
// the specification. Package errs declares the same values.
var DeviceStates = []Def{
	{"unchanged", "the device runs what it ran before the operation"},
	{"previous-release", "the operation failed after the switch and the device was rolled back to the previous release set"},
	{"new-release", "the device runs the new release set"},
	{"unknown", "the state cannot be proven; further changes to the device are blocked until `irekey status <device>` reconciles it"},
	{"needs-intervention", "the rollback failed or the state is ambiguous; an operator must intervene"},
}

// StateRules are the device_state values that are rules rather than device
// states. by-phase and none are exit values too.
var StateRules = []Def{
	{ByPhase, "exit code and device state: set by the phase of the operation; see \"Codes set by phase\""},
	{PerDevice, "device state: each device of the operation has its own state; see the report"},
	{AsRecovered, "device state: the state that recovery ended on"},
	{None, "exit code and device state: a warning or information code, which ends no command and describes no device state"},
}

// OutcomeDef maps a terminal per-device outcome to a device state.
type OutcomeDef struct {
	Name  string
	State string
}

// Outcomes are the terminal per-device states of a rollout and the device
// state each one maps to, in the order of the specification.
var Outcomes = []OutcomeDef{
	{"committed", "new-release"},
	{"rolled-back", "previous-release"},
	{"unchanged", "unchanged"},
	{"skipped", "unchanged"},
	{"needs-intervention", "needs-intervention"},
	{"unknown", "unknown"},
}

// PhaseDef is the exit code and device state that a phase gives a by-phase
// code.
type PhaseDef struct {
	Name    string
	Meaning string
	Exit    int
	State   string
}

// Phases are the phases of an operation, in the order of the Phase
// constants of package errs, which starts at zero.
var Phases = []PhaseDef{
	{"unknown", "nothing is known about device-side work", 15, "unknown"},
	{"local", "no device-side work was involved, as the caller states", 15, "unchanged"},
	{"before-switch", "before the switch", 6, "unchanged"},
	{"after-switch-rolled-back", "after the switch, and the rollback succeeded", 7, "previous-release"},
	{"rollback-failed", "after the switch, and the rollback failed", 8, "needs-intervention"},
	{"device-unknown", "device-side work started, and its outcome is unknown", 12, "unknown"},
}

// Pessimism lists the device states an error can leave a device in, from
// the most to the least favorable. The phase of an operation can move the
// result of a code only towards the end of the list.
var Pessimism = []string{"unchanged", "previous-release", "unknown", "needs-intervention"}

// ResolvedStates returns the device states that an error ending a command
// with the exit code can resolve to: the states a registered code with that
// exit code may declare, with per-device read as unknown for a single
// error, and the states the phases give a by-phase code with that exit
// code.
func ResolvedStates(exit int) []string {
	var out []string
	add := func(s string) {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if d, ok := exitDef(exit); ok {
		for _, s := range d.Allowed {
			if s == PerDevice {
				s = "unknown"
			}
			add(s)
		}
	}
	for _, p := range Phases {
		if p.Exit == exit {
			add(p.State)
		}
	}
	return out
}

// Kinds are the values of the kind field.
var Kinds = []string{"error", "warning", "info"}

// Statuses are the values of the status field.
var Statuses = []string{"reserved", "active"}

// Status values.
const (
	Reserved = "reserved"
	Active   = "active"
)

// KindError is the kind of a code that reports an error.
const KindError = "error"

// exitDef returns the definition of an exit code.
func exitDef(code int) (ExitDef, bool) {
	i := slices.IndexFunc(Exits, func(d ExitDef) bool { return d.Code == code })
	if i < 0 {
		return ExitDef{}, false
	}
	return Exits[i], true
}

// stateValues returns every valid device_state value: the device states,
// then the rules.
func stateValues() []string {
	var out []string
	for _, d := range DeviceStates {
		out = append(out, d.Name)
	}
	for _, d := range StateRules {
		out = append(out, d.Name)
	}
	return out
}
