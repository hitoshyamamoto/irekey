// SPDX-License-Identifier: MIT

package errs

import (
	"context"
	"errors"
	"strings"
)

// Error is an error classified with a registered code. It is created where
// the error is detected, from structured signals, and wraps the cause.
type Error struct {
	// Code is the registered code of the error.
	Code Code
	// Op names the operation that failed, such as "connect" or "activate".
	Op string
	// Device is the device the operation ran on, when there is one.
	Device string
	// Phase is what is known about the device-side work of the operation.
	// The zero value, PhaseUnknown, claims nothing; PhaseLocal states that
	// no device was touched.
	Phase Phase
	// Next is the command the operator should run next, when there is one.
	Next string
	// Err is the cause, or nil.
	Err error
}

// New returns an error with a code, the operation that failed and its
// cause, in PhaseUnknown. Set Phase when more is known.
func New(code Code, op string, err error) *Error {
	return &Error{Code: code, Op: op, Err: err}
}

// Error returns "<code>: <op>: <cause>", for example
// "LOC-001: patch: exec: git: not found". Without a cause, the title of the
// code takes its place.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	cause := ""
	if e.Err != nil {
		cause = e.Err.Error()
	} else if info, ok := Lookup(e.Code); ok {
		cause = info.Title
	}
	var parts []string
	for _, s := range []string{string(e.Code), e.Op, cause} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ": ")
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitCode returns the exit code of a command that fails with this error.
// See Info.Resolve.
func (e *Error) ExitCode() ExitCode {
	code, _ := e.resolve()
	return code
}

// DeviceState returns the state this error leaves the device in. See
// Info.Resolve.
func (e *Error) DeviceState() DeviceState {
	_, state := e.resolve()
	return state
}

// resolve resolves the code of the error in its phase. A code that is not
// registered is treated as BUG-003, like an unclassified error, and a nil
// *Error as an unclassified error.
func (e *Error) resolve() (ExitCode, DeviceState) {
	if e == nil {
		return lookup(BUG003).Resolve(PhaseUnknown)
	}
	info, ok := Lookup(e.Code)
	if !ok {
		info = lookup(BUG003)
	}
	return info.Resolve(e.Phase)
}

// lookup returns the entry of a code that the package itself relies on.
// If the registry ever lost it, the zero Info would resolve to an internal
// error in an unknown state, which still fails safe.
func lookup(c Code) Info {
	info, _ := Lookup(c)
	return info
}

// As returns the first *Error in the chain of err. A nil *Error does not
// count: an error chain that holds one is unclassified.
func As(err error) (*Error, bool) {
	e, ok := errors.AsType[*Error](err)
	if !ok || e == nil {
		return nil, false
	}
	return e, true
}

// CodeOf returns the code err is reported with, so that every failure
// states a code:
//
//   - "" for nil;
//   - the code of the first *Error in the chain, which takes precedence
//     over anything it wraps, an interruption included; BUG-003 when that
//     code is not registered;
//   - USR-006 for an interruption that carries no code: an error that wraps
//     context.Canceled;
//   - BUG-003 for any other error, which is unclassified.
func CodeOf(err error) Code {
	code, _, _ := classify(err)
	return code
}

// ExitCodeOf returns the exit code of a command that fails with err: ExitOK
// for nil, and otherwise the resolution (see Info.Resolve) of the code that
// CodeOf reports. The first *Error in the chain resolves in its own phase;
// an interruption or an unclassified error carries no phase and resolves in
// PhaseUnknown: exit 130 or 15, device state unknown.
func ExitCodeOf(err error) ExitCode {
	_, code, _ := classify(err)
	return code
}

// DeviceStateOf returns the state err leaves the device in, consistently
// with ExitCodeOf. For nil it returns "", which is no device state: the
// operation did what was asked.
func DeviceStateOf(err error) DeviceState {
	_, _, state := classify(err)
	return state
}

// classify returns the code err is reported with and its resolution.
func classify(err error) (Code, ExitCode, DeviceState) {
	if err == nil {
		return "", ExitOK, ""
	}
	if e, ok := As(err); ok {
		code, state := e.resolve()
		if _, registered := Lookup(e.Code); !registered {
			return BUG003, code, state
		}
		return e.Code, code, state
	}
	c := BUG003
	if errors.Is(err, context.Canceled) {
		c = USR006
	}
	code, state := lookup(c).Resolve(PhaseUnknown)
	return c, code, state
}
