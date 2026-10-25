// SPDX-License-Identifier: MIT

// Package errs holds the registry of irekey error codes, the typed error
// that carries a code, the exit codes, and the device-state and outcome
// enums that are used verbatim everywhere.
//
// The codes are registered in codes.yaml. go generate turns it into one
// constant per code, named after the code without its dash (LOC001 for
// LOC-001), and into docs/errors.md, which describes every code and exit
// code. A code is reserved until non-test code produces it; it is then
// marked active, and scripts/check-errors.sh requires that non-test code
// outside this package uses its constant and that a test outside this
// package refers to it.
//
// An error is classified where it is detected, by wrapping it in an *Error
// with its code and what is known about the device-side work, its Phase.
// CodeOf, ExitCodeOf and DeviceStateOf turn any error into the code it is
// reported with, the exit code of the command and the state of the device.
// No fallback claims a state that nothing has proven: an error without a
// code is reported as BUG-003, an interruption without a code as USR-006,
// and both leave the device state unknown.
//
// The package performs no I/O.
package errs

//go:generate go run ./internal/gen/errsgen -in codes.yaml -go zz_generated_codes.go -md ../../docs/errors.md
