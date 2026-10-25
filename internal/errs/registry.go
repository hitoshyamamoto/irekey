// SPDX-License-Identifier: MIT

package errs

import (
	"slices"
	"strconv"
)

// Code is a registered error code of the form <CATEGORY>-<NNN>, for example
// NET-003. A released code never changes meaning. Every code has a generated
// constant; non-test code outside this package uses the constants, never a
// string literal.
type Code string

// Kind tells whether a code reports an error, a warning or information.
// Only an error ends a command.
type Kind string

// The kinds of code.
const (
	KindError   Kind = "error"
	KindWarning Kind = "warning"
	KindInfo    Kind = "info"
)

// Status tells whether any code path produces a code yet.
type Status string

// The statuses of a code.
const (
	// StatusReserved marks a code that is registered, but that no code path
	// produces yet.
	StatusReserved Status = "reserved"
	// StatusActive marks a code that non-test code produces.
	StatusActive Status = "active"
)

// Values of the Exit and State fields of Info that are rules rather than an
// exit code or a device state.
const (
	// RuleByPhase: the phase of the operation sets the exit code and the
	// device state (see Info.Resolve).
	RuleByPhase = "by-phase"
	// RuleNone: a warning or information code, which ends no command and
	// describes no device state.
	RuleNone = "none"
	// RulePerDevice: each device of the operation has its own state.
	RulePerDevice = "per-device"
	// RuleAsRecovered: the state that recovery ended on.
	RuleAsRecovered = "as-recovered"
)

// Info describes a registered code: the fields of its entry in codes.yaml.
type Info struct {
	Code Code
	// Category is the prefix of the code, such as "NET".
	Category string
	// Title is a short name of the code.
	Title string
	// Condition says when the code is raised.
	Condition string
	// Signal is the structured signal that identifies the condition, and
	// Handling what irekey does about it. Both are set for an active code;
	// for a reserved code they may be empty.
	Signal   string
	Handling string
	// Exit is the exit code of the command in decimal, or RuleByPhase or
	// RuleNone.
	Exit string
	// State is the device state the code leaves a device in (a DeviceState
	// value), or one of the rules RuleByPhase, RulePerDevice,
	// RuleAsRecovered or RuleNone.
	State  string
	Kind   Kind
	Status Status
	// Since is the first irekey version that has the code.
	Since string
}

// index maps every registered code to its position in the registry.
var index = func() map[Code]int {
	m := make(map[Code]int, len(registry))
	for i, info := range registry {
		m[info.Code] = i
	}
	return m
}()

// Lookup returns the registry entry of a code.
func Lookup(c Code) (Info, bool) {
	i, ok := index[c]
	if !ok {
		return Info{}, false
	}
	return registry[i], true
}

// All returns every registered code, by category and then by number. The
// slice belongs to the caller.
func All() []Info {
	return slices.Clone(registry)
}

// Resolve returns the exit code of a command that fails with this code in
// the given phase, and the state the failure leaves the device in. The
// result is always a pair the exit-code table allows.
//
// First, the code gives a result of its own:
//
//   - A code whose exit is RuleByPhase takes it from the phase: see Phase.
//     PhaseUnknown, like a phase that is not defined, gives ExitInternal and
//     StateUnknown.
//   - Any other error code has its registered exit code and device state.
//     A per-device state, which describes an operation on several devices,
//     is StateUnknown for a single error. An interruption (exit 130) is
//     registered as StateUnknown and becomes StateUnchanged only in
//     PhaseLocal, the caller's statement that no device was touched.
//   - A warning or information code never ends a command: an error that
//     carries one is a programming mistake, and gives ExitInternal and
//     StateUnknown. So does an entry whose fields are not valid.
//
// Then the phase can only make that result more pessimistic, in the order
// unchanged, previous-release, unknown, needs-intervention, never more
// optimistic: PhaseAfterSwitchRolledBack raises an unchanged result to exit
// 7 and previous-release, PhaseDeviceUnknown raises a result below unknown
// to exit 12 and unknown, and PhaseRollbackFailed always gives exit 8 and
// needs-intervention. A result at least as pessimistic as the phase keeps
// its own exit code.
func (i Info) Resolve(p Phase) (ExitCode, DeviceState) {
	code, state := i.own(p)
	if floorCode, floorState, ok := p.floor(); ok && pessimism(floorState) > pessimism(state) {
		return floorCode, floorState
	}
	return code, state
}

// own returns the result the code gives by itself in the phase.
func (i Info) own(p Phase) (ExitCode, DeviceState) {
	if i.Kind != KindError {
		return ExitInternal, StateUnknown
	}
	if i.Exit == RuleByPhase {
		return p.byPhase()
	}
	n, err := strconv.Atoi(i.Exit)
	if err != nil || n <= 0 {
		// An error never exits with 0, the success status.
		return ExitInternal, StateUnknown
	}
	code := ExitCode(n)
	if i.State == RulePerDevice {
		return code, StateUnknown
	}
	state, err := ParseDeviceState(i.State)
	if err != nil || state == StateNewRelease {
		// No failure leaves a device on the new release.
		return ExitInternal, StateUnknown
	}
	if code == ExitInterrupted && p == PhaseLocal {
		return code, StateUnchanged
	}
	return code, state
}
