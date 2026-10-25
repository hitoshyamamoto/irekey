// SPDX-License-Identifier: MIT

package errs

import (
	"fmt"
	"slices"
	"strconv"
)

// ExitCode is the exit status of an irekey command. The codes are
// documented in docs/errors.md and become a stable contract at 1.0: a
// released exit code never changes meaning.
type ExitCode int

// The exit codes of irekey.
const (
	ExitOK                    ExitCode = 0   // success
	ExitUsage                 ExitCode = 1   // usage or input error
	ExitUnreachable           ExitCode = 2   // device unreachable
	ExitIdentityMismatch      ExitCode = 3   // identity mismatch
	ExitVerificationFailed    ExitCode = 4   // verification failed (signature, format, schema, gate)
	ExitPreflightBlocked      ExitCode = 5   // pre-flight blocked (disk, drift, downtime, platform)
	ExitAbortedBeforeSwitch   ExitCode = 6   // aborted before the switch; device unchanged
	ExitRolledBack            ExitCode = 7   // failed after the switch; rolled back successfully
	ExitNeedsIntervention     ExitCode = 8   // failed after the switch; rollback failed, or state ambiguous
	ExitMixedResults          ExitCode = 9   // multi-device operation finished with mixed results
	ExitFailureBudgetExceeded ExitCode = 10  // halted: failure budget exceeded
	ExitAuthFailed            ExitCode = 11  // authentication failed
	ExitOutcomeUnknown        ExitCode = 12  // contact lost after device-side work started
	ExitEnvironment           ExitCode = 13  // environment error (operator machine, deployment repository, artifact sources)
	ExitDeviceBusy            ExitCode = 14  // device busy (lock held)
	ExitInternal              ExitCode = 15  // internal error (crash report written)
	ExitInterrupted           ExitCode = 130 // interrupted by the operator
)

// DeviceState is the state an operation leaves a device in. Every error
// states one. The values are the words irekey prints and writes in JSON.
type DeviceState string

// The device states.
const (
	// StateUnchanged: the device runs what it ran before the operation.
	StateUnchanged DeviceState = "unchanged"
	// StatePreviousRelease: the operation failed after the switch and the
	// device was rolled back to the previous release set.
	StatePreviousRelease DeviceState = "previous-release"
	// StateNewRelease: the device runs the new release set.
	StateNewRelease DeviceState = "new-release"
	// StateUnknown: the state cannot be proven. Unknown is not success:
	// further changes to the device are blocked until irekey status
	// reconciles it.
	StateUnknown DeviceState = "unknown"
	// StateNeedsIntervention: the rollback failed or the state is
	// ambiguous; an operator must intervene.
	StateNeedsIntervention DeviceState = "needs-intervention"
)

// deviceStates lists the device states in the order of the specification.
var deviceStates = []DeviceState{
	StateUnchanged, StatePreviousRelease, StateNewRelease, StateUnknown, StateNeedsIntervention,
}

// DeviceStates returns every device state. The slice belongs to the caller.
func DeviceStates() []DeviceState {
	return slices.Clone(deviceStates)
}

// ParseDeviceState returns the device state named s.
func ParseDeviceState(s string) (DeviceState, error) {
	if i := slices.Index(deviceStates, DeviceState(s)); i >= 0 {
		return deviceStates[i], nil
	}
	return "", fmt.Errorf("unknown device state %q", s)
}

// Outcome is the terminal state of one device in an operation.
type Outcome string

// The outcomes.
const (
	OutcomeCommitted         Outcome = "committed"
	OutcomeRolledBack        Outcome = "rolled-back"
	OutcomeUnchanged         Outcome = "unchanged"
	OutcomeSkipped           Outcome = "skipped"
	OutcomeNeedsIntervention Outcome = "needs-intervention"
	OutcomeUnknown           Outcome = "unknown"
)

// outcomes lists the outcomes in the order of the specification.
var outcomes = []Outcome{
	OutcomeCommitted, OutcomeRolledBack, OutcomeUnchanged, OutcomeSkipped,
	OutcomeNeedsIntervention, OutcomeUnknown,
}

// Outcomes returns every outcome. The slice belongs to the caller.
func Outcomes() []Outcome {
	return slices.Clone(outcomes)
}

// ParseOutcome returns the outcome named s.
func ParseOutcome(s string) (Outcome, error) {
	if i := slices.Index(outcomes, Outcome(s)); i >= 0 {
		return outcomes[i], nil
	}
	return "", fmt.Errorf("unknown outcome %q", s)
}

// DeviceState returns the state an outcome leaves the device in. An outcome
// that is not one of the defined values is not proven, so its state is
// StateUnknown.
func (o Outcome) DeviceState() DeviceState {
	switch o {
	case OutcomeCommitted:
		return StateNewRelease
	case OutcomeRolledBack:
		return StatePreviousRelease
	case OutcomeUnchanged, OutcomeSkipped:
		return StateUnchanged
	case OutcomeNeedsIntervention:
		return StateNeedsIntervention
	case OutcomeUnknown:
		return StateUnknown
	}
	return StateUnknown
}

// Phase is what is known about the device-side work of an operation when
// an error occurred. It sets the exit code and the device state of the codes
// whose exit is RuleByPhase, and can make the result of any other code more
// pessimistic, never more optimistic (see Info.Resolve).
//
// The zero value is PhaseUnknown: nothing is known, so the phase adds
// nothing to what the code itself states. A caller that knows the operation
// touched no device says so with PhaseLocal. A result is "unchanged" only
// for a code registered as unchanged in a phase that does not make it more
// pessimistic, for a by-phase code in PhaseLocal or PhaseBeforeSwitch, and
// for USR-006 in PhaseLocal.
type Phase int

// The phases, with the exit code and device state they give a by-phase
// code.
const (
	// PhaseUnknown: nothing is known about device-side work (exit 15,
	// unknown). It is the zero value.
	PhaseUnknown Phase = iota
	// PhaseLocal: the caller states that no device-side work was involved
	// (exit 15, unchanged).
	PhaseLocal
	// PhaseBeforeSwitch: the error occurred on the device before the
	// switch, the point of no return, and the operation aborted (exit 6,
	// unchanged).
	PhaseBeforeSwitch
	// PhaseAfterSwitchRolledBack: the error occurred after the switch and
	// the rollback succeeded (exit 7, previous-release).
	PhaseAfterSwitchRolledBack
	// PhaseRollbackFailed: the error occurred after the switch and the
	// rollback failed (exit 8, needs-intervention).
	PhaseRollbackFailed
	// PhaseDeviceUnknown: device-side work started and its outcome is
	// unknown (exit 12, unknown).
	PhaseDeviceUnknown
)

// String returns the name of the phase, such as "before-switch".
func (p Phase) String() string {
	switch p {
	case PhaseUnknown:
		return "unknown"
	case PhaseLocal:
		return "local"
	case PhaseBeforeSwitch:
		return "before-switch"
	case PhaseAfterSwitchRolledBack:
		return "after-switch-rolled-back"
	case PhaseRollbackFailed:
		return "rollback-failed"
	case PhaseDeviceUnknown:
		return "device-unknown"
	}
	return "Phase(" + strconv.Itoa(int(p)) + ")"
}

// byPhase returns the exit code and device state the phase gives a by-phase
// code. A phase that is not defined is treated as PhaseUnknown.
func (p Phase) byPhase() (ExitCode, DeviceState) {
	switch p {
	case PhaseUnknown:
		return ExitInternal, StateUnknown
	case PhaseLocal:
		return ExitInternal, StateUnchanged
	case PhaseBeforeSwitch:
		return ExitAbortedBeforeSwitch, StateUnchanged
	case PhaseAfterSwitchRolledBack:
		return ExitRolledBack, StatePreviousRelease
	case PhaseRollbackFailed:
		return ExitNeedsIntervention, StateNeedsIntervention
	case PhaseDeviceUnknown:
		return ExitOutcomeUnknown, StateUnknown
	}
	return ExitInternal, StateUnknown
}

// floor returns the most favorable result the phase allows for any code:
// after the switch the device cannot be unchanged, a failed rollback needs
// intervention, and device-side work with an unknown outcome leaves the
// device unknown. ok is false for the phases that put no floor on a result.
func (p Phase) floor() (code ExitCode, state DeviceState, ok bool) {
	switch p {
	case PhaseAfterSwitchRolledBack:
		return ExitRolledBack, StatePreviousRelease, true
	case PhaseRollbackFailed:
		return ExitNeedsIntervention, StateNeedsIntervention, true
	case PhaseDeviceUnknown:
		return ExitOutcomeUnknown, StateUnknown, true
	case PhaseUnknown, PhaseLocal, PhaseBeforeSwitch:
	}
	return 0, "", false
}

// pessimism ranks the device states an error can leave a device in, from
// the most to the least favorable: unchanged, previous-release, unknown,
// needs-intervention. A state that is not one of them ranks as unknown.
func pessimism(s DeviceState) int {
	switch s {
	case StateUnchanged:
		return 0
	case StatePreviousRelease:
		return 1
	case StateNeedsIntervention:
		return 3
	case StateUnknown, StateNewRelease:
	}
	return 2
}
