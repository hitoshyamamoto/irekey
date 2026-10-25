// SPDX-License-Identifier: MIT

package errs_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/hitoshyamamoto/irekey/internal/errs"
	"github.com/hitoshyamamoto/irekey/internal/errs/internal/gen"
)

// exitCodes lists every exit code constant, in order.
var exitCodes = []errs.ExitCode{
	errs.ExitOK, errs.ExitUsage, errs.ExitUnreachable, errs.ExitIdentityMismatch,
	errs.ExitVerificationFailed, errs.ExitPreflightBlocked, errs.ExitAbortedBeforeSwitch,
	errs.ExitRolledBack, errs.ExitNeedsIntervention, errs.ExitMixedResults,
	errs.ExitFailureBudgetExceeded, errs.ExitAuthFailed, errs.ExitOutcomeUnknown,
	errs.ExitEnvironment, errs.ExitDeviceBusy, errs.ExitInternal, errs.ExitInterrupted,
}

// phases lists every phase, in order.
var phases = []errs.Phase{
	errs.PhaseUnknown, errs.PhaseLocal, errs.PhaseBeforeSwitch, errs.PhaseAfterSwitchRolledBack,
	errs.PhaseRollbackFailed, errs.PhaseDeviceUnknown,
}

func TestExitCodesMatchTheGenerator(t *testing.T) {
	var want []errs.ExitCode
	for _, d := range gen.Exits {
		want = append(want, errs.ExitCode(d.Code))
	}
	if !slices.Equal(exitCodes, want) {
		t.Errorf("exit code constants = %v, the generator documents %v", exitCodes, want)
	}
}

func TestEnumsMatchTheGenerator(t *testing.T) {
	var states []string
	for _, s := range errs.DeviceStates() {
		states = append(states, string(s))
	}
	var genStates []string
	for _, d := range gen.DeviceStates {
		genStates = append(genStates, d.Name)
	}
	if !slices.Equal(states, genStates) {
		t.Errorf("DeviceStates() = %v, the generator lists %v", states, genStates)
	}

	rules := []string{errs.RuleByPhase, errs.RulePerDevice, errs.RuleAsRecovered, errs.RuleNone}
	var genRules []string
	for _, d := range gen.StateRules {
		genRules = append(genRules, d.Name)
	}
	if !slices.Equal(rules, genRules) {
		t.Errorf("rule constants = %v, the generator lists %v", rules, genRules)
	}

	var outcomes, genOutcomes []string
	for _, o := range errs.Outcomes() {
		outcomes = append(outcomes, string(o)+" -> "+string(o.DeviceState()))
	}
	for _, d := range gen.Outcomes {
		genOutcomes = append(genOutcomes, d.Name+" -> "+d.State)
	}
	if !slices.Equal(outcomes, genOutcomes) {
		t.Errorf("Outcomes() maps %v, the generator documents %v", outcomes, genOutcomes)
	}

	kinds := []string{string(errs.KindError), string(errs.KindWarning), string(errs.KindInfo)}
	if !slices.Equal(kinds, gen.Kinds) {
		t.Errorf("kinds = %v, the generator lists %v", kinds, gen.Kinds)
	}
	statuses := []string{string(errs.StatusReserved), string(errs.StatusActive)}
	if !slices.Equal(statuses, gen.Statuses) {
		t.Errorf("statuses = %v, the generator lists %v", statuses, gen.Statuses)
	}

	if len(phases) != len(gen.Phases) {
		t.Fatalf("%d phases, the generator documents %d", len(phases), len(gen.Phases))
	}
	byPhase := errs.Info{Kind: errs.KindError, Exit: errs.RuleByPhase, State: errs.RuleByPhase}
	for i, p := range phases {
		d := gen.Phases[i]
		code, state := byPhase.Resolve(p)
		if p.String() != d.Name || int(code) != d.Exit || string(state) != d.State {
			t.Errorf("phase %d = %s (%d, %s), the generator documents %s (%d, %s)",
				i, p, code, state, d.Name, d.Exit, d.State)
		}
	}
	if errs.Phase(0) != errs.PhaseUnknown {
		t.Error("the zero Phase must be PhaseUnknown, which claims nothing")
	}
}

func TestOutcomeDeviceState(t *testing.T) {
	tests := []struct {
		outcome errs.Outcome
		want    errs.DeviceState
	}{
		{errs.OutcomeCommitted, errs.StateNewRelease},
		{errs.OutcomeRolledBack, errs.StatePreviousRelease},
		{errs.OutcomeUnchanged, errs.StateUnchanged},
		{errs.OutcomeSkipped, errs.StateUnchanged},
		{errs.OutcomeNeedsIntervention, errs.StateNeedsIntervention},
		{errs.OutcomeUnknown, errs.StateUnknown},
		{errs.Outcome(""), errs.StateUnknown},
		{errs.Outcome("succeeded"), errs.StateUnknown},
	}
	for _, tt := range tests {
		if got := tt.outcome.DeviceState(); got != tt.want {
			t.Errorf("Outcome(%q).DeviceState() = %q, want %q", tt.outcome, got, tt.want)
		}
	}
}

func TestParseDeviceState(t *testing.T) {
	for _, s := range errs.DeviceStates() {
		got, err := errs.ParseDeviceState(string(s))
		if err != nil || got != s {
			t.Errorf("ParseDeviceState(%q) = %q, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "Unchanged", "by-phase", "per-device", "none", "previous release"} {
		if got, err := errs.ParseDeviceState(s); err == nil {
			t.Errorf("ParseDeviceState(%q) = %q, want an error", s, got)
		}
	}
}

func TestParseOutcome(t *testing.T) {
	for _, o := range errs.Outcomes() {
		got, err := errs.ParseOutcome(string(o))
		if err != nil || got != o {
			t.Errorf("ParseOutcome(%q) = %q, %v", o, got, err)
		}
	}
	for _, s := range []string{"", "Committed", "rolled back", "new-release"} {
		if got, err := errs.ParseOutcome(s); err == nil {
			t.Errorf("ParseOutcome(%q) = %q, want an error", s, got)
		}
	}
}

func TestEnumListsBelongToTheCaller(t *testing.T) {
	states := errs.DeviceStates()
	states[0] = "changed"
	if errs.DeviceStates()[0] != errs.StateUnchanged {
		t.Error("changing the result of DeviceStates changed the package")
	}
	outcomes := errs.Outcomes()
	outcomes[0] = "changed"
	if errs.Outcomes()[0] != errs.OutcomeCommitted {
		t.Error("changing the result of Outcomes changed the package")
	}
}

func TestPhaseString(t *testing.T) {
	for p, want := range map[errs.Phase]string{errs.Phase(99): "Phase(99)", errs.Phase(-1): "Phase(-1)"} {
		if got := p.String(); got != want {
			t.Errorf("Phase.String() = %q, want %q", got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	byPhase := errs.Info{Kind: errs.KindError, Exit: errs.RuleByPhase, State: errs.RuleByPhase}
	fixed := func(exit errs.ExitCode, state errs.DeviceState) errs.Info {
		return errs.Info{Kind: errs.KindError, Exit: strconv.Itoa(int(exit)), State: string(state)}
	}
	unchanged := fixed(errs.ExitEnvironment, errs.StateUnchanged)
	rolledBack := fixed(errs.ExitRolledBack, errs.StatePreviousRelease)
	unknown := fixed(errs.ExitInternal, errs.StateUnknown)
	intervention := fixed(errs.ExitNeedsIntervention, errs.StateNeedsIntervention)
	interrupted := fixed(errs.ExitInterrupted, errs.StateUnknown)
	perDevice := errs.Info{Kind: errs.KindError, Exit: "10", State: errs.RulePerDevice}
	warning := errs.Info{Kind: errs.KindWarning, Exit: errs.RuleNone, State: errs.RuleNone}
	tests := []struct {
		name      string
		info      errs.Info
		phase     errs.Phase
		wantExit  errs.ExitCode
		wantState errs.DeviceState
	}{
		{"by phase, unknown", byPhase, errs.PhaseUnknown, errs.ExitInternal, errs.StateUnknown},
		{"by phase, local", byPhase, errs.PhaseLocal, errs.ExitInternal, errs.StateUnchanged},
		{"by phase, before the switch", byPhase, errs.PhaseBeforeSwitch, errs.ExitAbortedBeforeSwitch, errs.StateUnchanged},
		{"by phase, rolled back", byPhase, errs.PhaseAfterSwitchRolledBack, errs.ExitRolledBack, errs.StatePreviousRelease},
		{"by phase, rollback failed", byPhase, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},
		{"by phase, device unknown", byPhase, errs.PhaseDeviceUnknown, errs.ExitOutcomeUnknown, errs.StateUnknown},
		{"by phase, undefined phase", byPhase, errs.Phase(99), errs.ExitInternal, errs.StateUnknown},
		{"by phase, negative phase", byPhase, errs.Phase(-1), errs.ExitInternal, errs.StateUnknown},

		{"unchanged code, unknown phase", unchanged, errs.PhaseUnknown, errs.ExitEnvironment, errs.StateUnchanged},
		{"unchanged code, local", unchanged, errs.PhaseLocal, errs.ExitEnvironment, errs.StateUnchanged},
		{"unchanged code, before the switch", unchanged, errs.PhaseBeforeSwitch, errs.ExitEnvironment, errs.StateUnchanged},
		{"unchanged code, rolled back", unchanged, errs.PhaseAfterSwitchRolledBack, errs.ExitRolledBack, errs.StatePreviousRelease},
		{"unchanged code, rollback failed", unchanged, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},
		{"unchanged code, device unknown", unchanged, errs.PhaseDeviceUnknown, errs.ExitOutcomeUnknown, errs.StateUnknown},

		{"rolled-back code, local", rolledBack, errs.PhaseLocal, errs.ExitRolledBack, errs.StatePreviousRelease},
		{"rolled-back code, before the switch", rolledBack, errs.PhaseBeforeSwitch, errs.ExitRolledBack, errs.StatePreviousRelease},
		{"rolled-back code, device unknown", rolledBack, errs.PhaseDeviceUnknown, errs.ExitOutcomeUnknown, errs.StateUnknown},
		{"rolled-back code, rollback failed", rolledBack, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},

		{"unknown code keeps its exit, local", unknown, errs.PhaseLocal, errs.ExitInternal, errs.StateUnknown},
		{"unknown code keeps its exit, rolled back", unknown, errs.PhaseAfterSwitchRolledBack, errs.ExitInternal, errs.StateUnknown},
		{"unknown code keeps its exit, device unknown", unknown, errs.PhaseDeviceUnknown, errs.ExitInternal, errs.StateUnknown},
		{"unknown code, rollback failed", unknown, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},

		{"intervention stays, local", intervention, errs.PhaseLocal, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},
		{"intervention stays, device unknown", intervention, errs.PhaseDeviceUnknown, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},
		{"intervention stays, rolled back", intervention, errs.PhaseAfterSwitchRolledBack, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},

		{"per device is unknown", perDevice, errs.PhaseUnknown, errs.ExitFailureBudgetExceeded, errs.StateUnknown},
		{"per device keeps its exit, device unknown", perDevice, errs.PhaseDeviceUnknown, errs.ExitFailureBudgetExceeded, errs.StateUnknown},
		{"per device, rollback failed", perDevice, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},

		{"interruption, unknown phase", interrupted, errs.PhaseUnknown, errs.ExitInterrupted, errs.StateUnknown},
		{"interruption, local", interrupted, errs.PhaseLocal, errs.ExitInterrupted, errs.StateUnchanged},
		{"interruption, before the switch", interrupted, errs.PhaseBeforeSwitch, errs.ExitInterrupted, errs.StateUnknown},
		{"interruption, rolled back", interrupted, errs.PhaseAfterSwitchRolledBack, errs.ExitInterrupted, errs.StateUnknown},
		{"interruption, device unknown", interrupted, errs.PhaseDeviceUnknown, errs.ExitInterrupted, errs.StateUnknown},
		{"interruption, rollback failed", interrupted, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},

		{"warning used as an error", warning, errs.PhaseLocal, errs.ExitInternal, errs.StateUnknown},
		{"warning used as an error, rollback failed", warning, errs.PhaseRollbackFailed, errs.ExitNeedsIntervention, errs.StateNeedsIntervention},
		{"information used as an error", errs.Info{Kind: errs.KindInfo, Exit: errs.RuleNone, State: errs.RuleNone}, errs.PhaseBeforeSwitch,
			errs.ExitInternal, errs.StateUnknown},
		{"error with exit none", errs.Info{Kind: errs.KindError, Exit: errs.RuleNone, State: errs.RuleNone}, errs.PhaseLocal,
			errs.ExitInternal, errs.StateUnknown},
		{"error with exit 0", fixed(errs.ExitOK, errs.StateUnchanged), errs.PhaseLocal, errs.ExitInternal, errs.StateUnknown},
		{"error with an invalid state", errs.Info{Kind: errs.KindError, Exit: "4", State: "broken"}, errs.PhaseLocal,
			errs.ExitInternal, errs.StateUnknown},
		{"error that claims the new release", fixed(errs.ExitVerificationFailed, errs.StateNewRelease), errs.PhaseLocal,
			errs.ExitInternal, errs.StateUnknown},
		{"zero Info", errs.Info{}, errs.PhaseLocal, errs.ExitInternal, errs.StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, state := tt.info.Resolve(tt.phase)
			if exit != tt.wantExit || state != tt.wantState {
				t.Errorf("Resolve(%s) = (%d, %q), want (%d, %q)", tt.phase, exit, state, tt.wantExit, tt.wantState)
			}
		})
	}
}

func TestLookupAndAll(t *testing.T) {
	info, ok := errs.Lookup(errs.LOC001)
	if !ok || info.Code != errs.LOC001 || info.Category != "LOC" || info.Exit != "13" ||
		info.State != string(errs.StateUnchanged) || info.Kind != errs.KindError || info.Since != "0.1.0" {
		t.Errorf("Lookup(LOC001) = %+v, %v", info, ok)
	}
	if info, ok := errs.Lookup("LOC-999"); ok {
		t.Errorf("Lookup(LOC-999) = %+v, want not found", info)
	}
	all := errs.All()
	if len(all) == 0 || all[0].Code != errs.USR001 || all[len(all)-1].Code != errs.BUG003 {
		t.Fatalf("All() does not run from USR-001 to BUG-003: %d codes", len(all))
	}
	all[0].Title = "changed"
	if errs.All()[0].Title == "changed" {
		t.Error("changing the result of All changed the registry")
	}
	for _, info := range all {
		if got, ok := errs.Lookup(info.Code); !ok || got.Code != info.Code {
			t.Errorf("Lookup(%s) = %+v, %v", info.Code, got, ok)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	cause := errors.New("exec: git: not found")
	tests := []struct {
		name string
		err  *errs.Error
		want string
	}{
		{"code, operation and cause", errs.New(errs.LOC001, "patch", cause), "LOC-001: patch: exec: git: not found"},
		{"no operation", errs.New(errs.LOC001, "", cause), "LOC-001: exec: git: not found"},
		{"no cause uses the title", errs.New(errs.ENG002, "activate", nil), "ENG-002: activate: device lock held"},
		{"unregistered code without cause", errs.New("XYZ-001", "op", nil), "XYZ-001: op"},
		{"empty", &errs.Error{}, ""},
		{"nil", nil, "<nil>"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("%s: Error() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestErrorWrapsItsCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := fmt.Errorf("adopt: %w", errs.New(errs.NET002, "connect", cause))
	if !errors.Is(err, cause) {
		t.Error("errors.Is does not find the cause through *Error")
	}
	e, ok := errs.As(err)
	if !ok || e.Code != errs.NET002 || e.Op != "connect" || !errors.Is(e.Unwrap(), cause) {
		t.Errorf("As = %+v, %v", e, ok)
	}
	if e.Phase != errs.PhaseUnknown {
		t.Errorf("New set phase %s; it must claim nothing", e.Phase)
	}
	if _, ok := errs.As(cause); ok {
		t.Error("As found an *Error in a plain error")
	}
	if e.ExitCode() != errs.ExitUnreachable || e.DeviceState() != errs.StateUnchanged {
		t.Errorf("NET-002 = (%d, %q), want (2, unchanged)", e.ExitCode(), e.DeviceState())
	}
}

func TestNilError(t *testing.T) {
	var e *errs.Error
	if e.Unwrap() != nil {
		t.Error("a nil *Error unwraps to a cause")
	}
	if e.ExitCode() != errs.ExitInternal || e.DeviceState() != errs.StateUnknown {
		t.Errorf("nil *Error = (%d, %q), want (15, unknown)", e.ExitCode(), e.DeviceState())
	}
	for _, err := range []error{e, fmt.Errorf("wrapped: %w", e)} {
		if got, ok := errs.As(err); ok || got != nil {
			t.Errorf("As(%v) = %v, %v; a nil *Error does not count", err, got, ok)
		}
		if errs.ExitCodeOf(err) != errs.ExitInternal || errs.DeviceStateOf(err) != errs.StateUnknown ||
			errs.CodeOf(err) != errs.BUG003 {
			t.Errorf("a chain holding a nil *Error = (%d, %q, %q), want (15, unknown, BUG-003)",
				errs.ExitCodeOf(err), errs.DeviceStateOf(err), errs.CodeOf(err))
		}
	}
}

func TestExitCodeOfAndDeviceStateOf(t *testing.T) {
	inPhase := func(code errs.Code, p errs.Phase, cause error) error {
		return &errs.Error{Code: code, Op: "op", Phase: p, Err: cause}
	}
	plain := errors.New("something broke")
	canceled := context.Canceled
	tests := []struct {
		name      string
		err       error
		wantExit  errs.ExitCode
		wantState errs.DeviceState
		wantCode  errs.Code
	}{
		{"nil", nil, errs.ExitOK, "", ""},

		// Unclassified errors claim nothing.
		{"unclassified", plain, errs.ExitInternal, errs.StateUnknown, errs.BUG003},
		{"unclassified, wrapped", fmt.Errorf("status: %w", plain), errs.ExitInternal, errs.StateUnknown, errs.BUG003},
		{"deadline is not an interruption", context.DeadlineExceeded, errs.ExitInternal, errs.StateUnknown, errs.BUG003},
		{"unregistered code, unknown phase", errs.New("XYZ-001", "op", nil), errs.ExitInternal, errs.StateUnknown, errs.BUG003},
		{"unregistered code, before the switch", inPhase("XYZ-001", errs.PhaseBeforeSwitch, nil),
			errs.ExitAbortedBeforeSwitch, errs.StateUnchanged, errs.BUG003},
		{"empty *Error", &errs.Error{}, errs.ExitInternal, errs.StateUnknown, errs.BUG003},

		// Interruptions.
		{"interrupted", canceled, errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interrupted, wrapped", fmt.Errorf("plan: %w", canceled), errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interrupted, joined with another error", errors.Join(plain, canceled), errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interruption stated local", inPhase(errs.USR006, errs.PhaseLocal, canceled), errs.ExitInterrupted, errs.StateUnchanged, errs.USR006},
		{"interruption, nothing known", errs.New(errs.USR006, "op", canceled), errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interruption before the switch", inPhase(errs.USR006, errs.PhaseBeforeSwitch, nil), errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interruption, device unknown", inPhase(errs.USR006, errs.PhaseDeviceUnknown, nil), errs.ExitInterrupted, errs.StateUnknown, errs.USR006},
		{"interruption, rollback failed", inPhase(errs.USR006, errs.PhaseRollbackFailed, nil),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.USR006},

		// An *Error takes precedence over an interruption it wraps.
		{"classified error wraps an interruption", errs.New(errs.LOC001, "patch", canceled), errs.ExitEnvironment, errs.StateUnchanged, errs.LOC001},
		{"needs intervention wraps an interruption", errs.New(errs.RCV002, "rollback", canceled),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.RCV002},
		{"rolled back wraps an interruption", inPhase(errs.DB002, errs.PhaseLocal, canceled), errs.ExitRolledBack, errs.StatePreviousRelease, errs.DB002},
		{"interruption joined before an *Error", fmt.Errorf("%w: %w", canceled, errs.New(errs.LOC001, "x", nil)),
			errs.ExitEnvironment, errs.StateUnchanged, errs.LOC001},
		{"the first *Error wins", errs.New(errs.HLT001, "health", errs.New(errs.NET004, "poll", nil)),
			errs.ExitRolledBack, errs.StatePreviousRelease, errs.HLT001},

		// Classified errors.
		{"classified", errs.New(errs.LOC001, "patch", plain), errs.ExitEnvironment, errs.StateUnchanged, errs.LOC001},
		{"classified, wrapped", fmt.Errorf("save: %w", errs.New(errs.ENG002, "lock", nil)), errs.ExitDeviceBusy, errs.StateUnchanged, errs.ENG002},
		{"after the switch", errs.New(errs.HLT001, "health", nil), errs.ExitRolledBack, errs.StatePreviousRelease, errs.HLT001},
		{"after the switch, outcome unknown", inPhase(errs.HLT001, errs.PhaseDeviceUnknown, nil), errs.ExitOutcomeUnknown, errs.StateUnknown, errs.HLT001},
		{"connect failed during the switch window", inPhase(errs.NET001, errs.PhaseAfterSwitchRolledBack, nil),
			errs.ExitRolledBack, errs.StatePreviousRelease, errs.NET001},
		{"connect failed before the switch", inPhase(errs.NET001, errs.PhaseBeforeSwitch, nil), errs.ExitUnreachable, errs.StateUnchanged, errs.NET001},
		{"contact lost", errs.New(errs.NET004, "activate", nil), errs.ExitOutcomeUnknown, errs.StateUnknown, errs.NET004},
		{"panic keeps its exit", inPhase(errs.BUG001, errs.PhaseDeviceUnknown, nil), errs.ExitInternal, errs.StateUnknown, errs.BUG001},
		{"intervention is never downgraded", inPhase(errs.RCV002, errs.PhaseDeviceUnknown, nil),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.RCV002},
		{"failure budget", errs.New(errs.RO002, "rollout", nil), errs.ExitFailureBudgetExceeded, errs.StateUnknown, errs.RO002},
		{"failure budget, rollback failed", inPhase(errs.RO002, errs.PhaseRollbackFailed, nil),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.RO002},

		// By-phase codes.
		{"by phase, unknown", errs.New(errs.BUG003, "op", plain), errs.ExitInternal, errs.StateUnknown, errs.BUG003},
		{"by phase, local", inPhase(errs.BUG003, errs.PhaseLocal, plain), errs.ExitInternal, errs.StateUnchanged, errs.BUG003},
		{"by phase, before the switch", inPhase(errs.STO003, errs.PhaseBeforeSwitch, nil),
			errs.ExitAbortedBeforeSwitch, errs.StateUnchanged, errs.STO003},
		{"by phase, rolled back", inPhase(errs.PRV003, errs.PhaseAfterSwitchRolledBack, nil),
			errs.ExitRolledBack, errs.StatePreviousRelease, errs.PRV003},
		{"by phase, rollback failed", inPhase(errs.BUG003, errs.PhaseRollbackFailed, plain),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.BUG003},
		{"by phase, device unknown", inPhase(errs.BUG003, errs.PhaseDeviceUnknown, plain),
			errs.ExitOutcomeUnknown, errs.StateUnknown, errs.BUG003},

		// Warnings and information are not errors.
		{"warning used as an error", errs.New(errs.TIME001, "status", nil), errs.ExitInternal, errs.StateUnknown, errs.TIME001},
		{"warning used as an error, rollback failed", inPhase(errs.TIME001, errs.PhaseRollbackFailed, nil),
			errs.ExitNeedsIntervention, errs.StateNeedsIntervention, errs.TIME001},
		{"information used as an error", errs.New(errs.HLT003, "health", nil), errs.ExitInternal, errs.StateUnknown, errs.HLT003},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errs.ExitCodeOf(tt.err); got != tt.wantExit {
				t.Errorf("ExitCodeOf = %d, want %d", got, tt.wantExit)
			}
			if got := errs.DeviceStateOf(tt.err); got != tt.wantState {
				t.Errorf("DeviceStateOf = %q, want %q", got, tt.wantState)
			}
			if got := errs.CodeOf(tt.err); got != tt.wantCode {
				t.Errorf("CodeOf = %q, want %q", got, tt.wantCode)
			}
		})
	}
}
