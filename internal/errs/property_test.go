// SPDX-License-Identifier: MIT

package errs_test

// The specification asks for property tests and fuzzing of pure packages.
// The property checked here holds for every code in every phase: the
// resolution is an exit code and device state that the exit-code table
// allows, and it is never more optimistic than the code's registered state
// or than the phase.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/quick"

	"github.com/hitoshyamamoto/irekey/internal/errs"
	"github.com/hitoshyamamoto/irekey/internal/errs/internal/gen"
)

// rank returns the position of a device state in the generator's
// pessimism order, from the most to the least favorable, or -1.
func rank(s string) int {
	return slices.Index(gen.Pessimism, s)
}

// floorOf returns the least pessimistic device state a resolution of the
// code in the phase may have.
func floorOf(info errs.Info, p errs.Phase) string {
	known := int(p) >= 0 && int(p) < len(gen.Phases)
	var floor string
	switch {
	case info.Kind != errs.KindError:
		floor = string(errs.StateUnknown)
	case info.Exit == errs.RuleByPhase:
		floor = string(errs.StateUnknown)
		if known {
			floor = gen.Phases[p].State
		}
	case info.State == errs.RulePerDevice:
		floor = string(errs.StateUnknown)
	case info.Exit == "130" && p == errs.PhaseLocal:
		// The one exception: the caller states that no device was touched.
		floor = string(errs.StateUnchanged)
	default:
		floor = info.State
	}
	// The phase raises the floor after the switch and when the outcome of
	// device-side work is unknown.
	var phaseFloor string
	switch p {
	case errs.PhaseAfterSwitchRolledBack:
		phaseFloor = string(errs.StatePreviousRelease)
	case errs.PhaseRollbackFailed:
		phaseFloor = string(errs.StateNeedsIntervention)
	case errs.PhaseDeviceUnknown:
		phaseFloor = string(errs.StateUnknown)
	case errs.PhaseUnknown, errs.PhaseLocal, errs.PhaseBeforeSwitch:
	}
	if phaseFloor != "" && rank(phaseFloor) > rank(floor) {
		floor = phaseFloor
	}
	return floor
}

// checkResolution returns why a resolution breaks the property, or "".
func checkResolution(info errs.Info, p errs.Phase, exit errs.ExitCode, state errs.DeviceState) string {
	if exit == errs.ExitOK {
		return "an error exits with 0"
	}
	if !slices.Contains(gen.ResolvedStates(int(exit)), string(state)) {
		return fmt.Sprintf("(%d, %s) is not a pair the exit-code table allows", exit, state)
	}
	if floor := floorOf(info, p); rank(string(state)) < rank(floor) {
		return fmt.Sprintf("(%d, %s) is more optimistic than %s", exit, state, floor)
	}
	return ""
}

// testPhases are the defined phases and two that are not.
var testPhases = append(slices.Clone(phases), errs.Phase(-1), errs.Phase(len(phases)))

// TestResolveProperty checks the property exhaustively: every registered
// code in every phase.
func TestResolveProperty(t *testing.T) {
	for _, info := range errs.All() {
		for _, p := range testPhases {
			exit, state := info.Resolve(p)
			if msg := checkResolution(info, p, exit, state); msg != "" {
				t.Errorf("%s (exit %s, state %s) in %s: %s", info.Code, info.Exit, info.State, p, msg)
			}
		}
	}
}

// pick returns the registered code at an arbitrary index.
func pick(i uint) errs.Info {
	all := errs.All()
	return all[int(i%uint(len(all)))]
}

// wrapped returns err as a command might see it, depending on how.
func wrapped(err error, how uint8) error {
	switch how % 4 {
	case 1:
		return fmt.Errorf("command: %w", err)
	case 2:
		return errors.Join(err, context.Canceled)
	case 3:
		return fmt.Errorf("%w: %w", context.Canceled, err)
	}
	return err
}

// resolutionHolds is the property for one code, phase and way of wrapping:
// the resolution is allowed, and an *Error resolves the same, with its own
// code, however it is wrapped and whatever interruption it is joined with.
func resolutionHolds(i uint, phase int, how uint8) error {
	info := pick(i)
	p := errs.Phase(phase)
	exit, state := info.Resolve(p)
	if msg := checkResolution(info, p, exit, state); msg != "" {
		return fmt.Errorf("%s in %s: %s", info.Code, p, msg)
	}
	err := wrapped(&errs.Error{Code: info.Code, Phase: p}, how)
	if errs.ExitCodeOf(err) != exit || errs.DeviceStateOf(err) != state || errs.CodeOf(err) != info.Code {
		return fmt.Errorf("%s in %s, wrapped %d: (%d, %s, %s), want (%d, %s, %s)", info.Code, p, how%4,
			errs.ExitCodeOf(err), errs.DeviceStateOf(err), errs.CodeOf(err), exit, state, info.Code)
	}
	return nil
}

func TestResolveQuick(t *testing.T) {
	property := func(i uint, phase int8, how uint8) bool {
		if err := resolutionHolds(i, int(phase), how); err != nil {
			t.Log(err)
			return false
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 5000}); err != nil {
		t.Error(err)
	}
}

// TestUnclassifiedQuick checks that an error without an *Error never claims
// a known device state.
func TestUnclassifiedQuick(t *testing.T) {
	property := func(msg string, how uint8) bool {
		err := wrapped(errors.New(msg), how)
		want := errs.BUG003
		if errors.Is(err, context.Canceled) {
			want = errs.USR006
		}
		return errs.CodeOf(err) == want && errs.DeviceStateOf(err) == errs.StateUnknown &&
			errs.ExitCodeOf(err) != errs.ExitOK
	}
	if err := quick.Check(property, nil); err != nil {
		t.Error(err)
	}
}

func FuzzResolve(f *testing.F) {
	for i := range errs.All() {
		for _, p := range testPhases {
			f.Add(uint(i), int(p), uint8(i))
		}
	}
	f.Fuzz(func(t *testing.T, i uint, phase int, how uint8) {
		if err := resolutionHolds(i, phase, how); err != nil {
			t.Error(err)
		}
	})
}
