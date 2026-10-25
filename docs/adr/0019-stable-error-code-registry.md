---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Register stable error codes in one file and generate code and documentation from it

## Context and Problem Statement

irekey changes software on devices that are far away and often offline.
When something fails, the operator must learn, from one message, what
failed, what state the device was left in, and what to run next. Scripts
and the deployment repository's CI must be able to tell failures apart
without parsing English text. The specification therefore asks for stable
error codes of the form `<CATEGORY>-<NNN>` (for example `NET-003`) in twenty
categories, a code that never changes meaning once released, an exit code
and a resulting device state for every code, and a registry in
`internal/errs/codes.yaml` that is turned into Go constants and
`docs/errors.md` with `go generate`. It seeds the registry with a table of
codes and asks that every code have a test that produces it.

Almost none of the code paths that raise these codes exist yet. A code
whose producing test is required before its code path exists would block
the registry, while a registry without its seed codes would lose the
specification's design work and invite ad-hoc codes later.

How are the codes kept stable, documented and honest about what the
program actually produces?

## Decision Drivers

* A released code never changes meaning, and an exit code and device state
  belong to every code.
* One source of truth: the Go constants, the documentation and the
  checks must not drift apart.
* The documentation must not claim behavior that no code implements.
* Mistakes (a typo in a code, a code spelled as a string, an inconsistent
  exit code and device state) are caught by a machine, not by review.
* No new dependency in any irekey binary.

## Considered Options

* Registry file and generator
* Hand-written constants
* No codes

## Decision Outcome

Chosen option: "Registry file and generator", because it gives one
reviewable file from which the constants and the documentation are
generated and checked, as the specification asks.

### The registry and the generator

`internal/errs/codes.yaml` lists the categories, in order, and every code
with these fields: `code`, `category`, `title`, `condition`, `signal`,
`handling`, `exit` (an exit code, `by-phase` or `none`), `device_state`
(`unchanged`, `previous-release`, `new-release`, `unknown`,
`needs-intervention`, or one of the rules `by-phase`, `per-device`,
`as-recovered`, `none`), `kind` (`error`, `warning` or `info`), `status`
and `since`. Every code registered before the first release carries
`since: "0.1.0"`, the version of the first planned release, because no
version has been released yet.

The generator is a library, `internal/errs/internal/gen`, with a thin
command, `errsgen`. `go generate ./internal/errs` runs it to write
`internal/errs/zz_generated_codes.go` (one constant per code, named after
the code without its dash, such as `errs.LOC001`, and the registry that
`errs.Lookup` and `errs.All` read) and `docs/errors.md`. The registry is
decoded strictly: an unknown or duplicate key is an error. Generation
fails when a code does not have the form `<CATEGORY>-<NNN>`, names an
unknown category or not its own, is registered twice or out of order, has
an exit code outside the exit-code table, has an exit and device state
that disagree, is an error with exit `none`, or is a warning or
information code with any exit other than `none`. The exit codes and their
allowed device states are:

| Exit code | Allowed device state |
|---|---|
| 1, 2, 3, 4, 5, 6, 11, 13, 14 | `unchanged` |
| 7 | `previous-release` |
| 8 | `needs-intervention` |
| 10 | `per-device` |
| 12 | `unknown` |
| 15 | `unknown` |
| 130 | `unchanged` or `unknown` |
| `none` | `none` or `as-recovered` |
| `by-phase` | `by-phase` |

Exit codes 0 (success) and 9 (mixed results) describe a whole operation,
never one error, so no code carries them. Every seed code of the
specification fits this table.

The YAML library is `go.yaml.in/yaml/v3`, the maintained successor of
`gopkg.in/yaml.v3` (MIT for the files ported from libyaml, Apache-2.0 for
the rest). Only the generator and the tests import it; package `errs`
itself stays free of I/O and of dependencies, so the library is linked
into no irekey binary. The module also ships a `NOTICE` file under the
Apache-2.0 terms. It is not distributed, since no binary links the
module. `go-licenses` v2.0.1, the version CI runs, classifies the module
as MIT only, so if a binary ever links it, its notices must be checked by
hand.

Every code of the specification's seed table is registered, and a test
checks that the registry holds exactly the codes of that table, with the
same conditions, exit codes and device states. A new code is therefore
added to `codes.yaml` and to the specification's error table in the same
pull request: the specification stays the single source of truth for what
each code means, and the test keeps the two from drifting apart.

### Reserved and active codes

Every code of the specification's seed table is registered with
`status: reserved`: registered, with its condition, exit code and device
state, but produced by no code path yet. A code becomes `active` in the
change that makes non-test code produce it. Only an active code must have
a signal, a handling and a test that produces it; the check below can only
see that a test outside `internal/errs` refers to the code's constant. A
reserved code has a signal and a handling only where the specification
states them, with a comment in `codes.yaml` naming the part of the
specification they come from; otherwise they stay empty, and `docs/errors.md` shows "not defined
yet" and marks the code "reserved: not produced by any code path yet". No
signal or handling is invented ahead of the code that implements it.

`scripts/check-errors.sh` runs `errsgen -check` on the repository and
fails when:

* non-test Go code outside `internal/errs` writes a code as a string
  literal, or as a constant concatenation of literals, instead of using
  its generated constant;
* non-test code uses a code constant that is not registered, or a code
  that is still reserved;
* a file, test files included, imports package `errs` with a dot, which
  would hide the codes it uses;
* an active code is not used by non-test code outside `internal/errs`, or
  no test outside `internal/errs` refers to its constant;
* a Markdown file of the repository, except the specification,
  `docs/errors.md` and files under a `testdata` directory, mentions a code with a registered prefix that is not
  registered;
* a Go file cannot be parsed, so its use of codes cannot be checked;
* the registry is invalid, or the generated Go file or `docs/errors.md` is
  stale.

### Four additions

The seed table has no code for four situations, and reusing a nearby code
would give it a second meaning. They are added, and marked as additions
in the specification's table:

| Code | Condition | Exit code | Device state |
|---|---|---|---|
| `USR-005` | invalid command line (unknown command, unknown flag, wrong arguments) | 1 | `unchanged` |
| `USR-006` | interrupted by the operator | 130 | `unknown` |
| `LOC-004` | configuration or cache directory not writable | 13 | `unchanged` |
| `TIME-002` | operator machine clock earlier than the build time of this irekey binary (warning) | `none` | `none` |

`USR-005` covers usage errors, which exit code 1 includes. `USR-006` is
the interruption that exit code 130 describes; its handling states that
device-side work already started continues and that, if it had started,
the device state is reported as unknown until `irekey status` reconciles
it. It is registered as `unknown`, the safe default, and resolves to
`unchanged` only when the caller states that no device-side work was
involved. `LOC-004` differs from `LOC-002` (a full disk) in cause and remedy.
`TIME-002` is a heuristic that needs no stored state, unlike detecting
that the operator machine's clock moved backwards; `TIME-001` stays the
device's clock.

### Exit codes and device states of an error

An error is classified where it is detected by wrapping it in an
`errs.Error` with its code and, for device-side work, its phase: what is
known about the device-side work of the operation. `errs.CodeOf`,
`errs.ExitCodeOf` and `errs.DeviceStateOf` resolve any error. The rules
follow the error model's "fail safe on the unknown" and "unknown is not
success": no fallback claims a state that nothing has proven.

* The zero phase is `PhaseUnknown`: nothing is known about device-side
  work, and the phase adds nothing to what the code itself states.
  `PhaseLocal` is a statement the caller makes when the operation touched
  no device. A result is `unchanged` in three cases only: the code is
  registered as `unchanged` and the phase does not make it more
  pessimistic; a `by-phase` code is resolved in `PhaseLocal` or
  `PhaseBeforeSwitch`; or `USR-006` is resolved in `PhaseLocal`. No
  fallback for an error without a code yields `unchanged`.
* A code whose exit is `by-phase` (`PRV-003`, `STO-003`, `BUG-003`) takes
  its exit code and device state from the phase, as the specification's
  error model describes, and also in the three cases the error model did
  not cover, which the specification now states: nothing is known about
  device-side work (exit 15, `unknown`), the caller states that no
  device-side work was involved (exit 15, `unchanged`), and device-side
  work started with an unknown outcome (exit 12, `unknown`):

  | Phase | Exit code | Device state |
  |---|---|---|
  | nothing is known about device-side work | 15 | `unknown` |
  | no device-side work was involved, as the caller states | 15 | `unchanged` |
  | before the switch | 6 | `unchanged` |
  | after the switch, rollback succeeded | 7 | `previous-release` |
  | after the switch, rollback failed | 8 | `needs-intervention` |
  | device-side work started, outcome unknown | 12 | `unknown` |

* Any other error code has its registered exit code and device state, and
  the phase can only make that result more pessimistic, never more
  optimistic, in the order `unchanged`, `previous-release`, `unknown`,
  `needs-intervention`. After the switch with a successful rollback, an
  `unchanged` result becomes exit 7 and `previous-release`. When
  device-side work started with an unknown outcome, a result more
  favorable than `unknown` becomes exit 12 and `unknown`. A failed
  rollback always gives exit 8 and `needs-intervention`. A result that is
  already at least as pessimistic keeps its own exit code: a code whose
  state is `needs-intervention` stays 8, and one whose state is `unknown`
  keeps its exit code (15 for `BUG-001`, 10 for `RO-002`). A `per-device`
  state is `unknown` for a single error.
* The one result a phase makes more favorable is an interruption: `USR-006`
  (exit 130) is registered as `unknown` and is `unchanged` only in
  `PhaseLocal`.
* A warning or information code ends no command; an error that carries
  one is a programming mistake and resolves to exit 15 and `unknown`
  (or to exit 8 and `needs-intervention` when the rollback failed).
* The specification's design for an error that no code classifies is to
  abort before the switch and roll back after it. The package cannot do
  either; it reports such an error, which has no phase, as `BUG-003` in
  `PhaseUnknown`: exit 15, device `unknown`. `CodeOf` returns `BUG-003`
  for it, so that every failure states a code. An `errs.Error` whose code
  is not registered or empty is reported as `BUG-003` in its own phase,
  and a nil `*errs.Error` in an error chain as `BUG-003` in
  `PhaseUnknown`.
* An interruption that carries no code, an error that wraps
  `context.Canceled` without an `errs.Error` in its chain, is reported as
  `USR-006` in `PhaseUnknown`: exit 130, device `unknown`. A caller that
  knows no device was touched wraps it in an `errs.Error` with `USR-006`
  and `PhaseLocal`, which gives exit 130 and `unchanged`.
* An `errs.Error` in the chain always takes precedence over an
  interruption it wraps: its code and its resolution are kept, never
  replaced by exit 130.

Tests check every rule by example, and a property test and a fuzz target
check, for every code in every phase, that the result is a pair the
exit-code table allows and is never more optimistic than the code's
registered state (with the interruption exception above).

Diagnostic commands follow a convention: for each problem they find, they
report the code that a real operation would raise for the same problem,
and have no codes of their own.

### Consequences

* Good, because the constants, the registry that the program reads and
  `docs/errors.md` come from one file and cannot drift apart: a test and
  `check-errors.sh` fail when the generated files are stale.
* Good, because every seed code of the specification is registered now,
  with the condition, exit code and device state the specification gives
  it, and a test compares the registry with the specification's table.
* Good, because the documentation never describes behavior that is not
  implemented: reserved codes say so.
* Good, because no error, classified or not, can exit with 0 or claim a
  device state that nothing has proven.
* Neutral, because adding a code means editing `codes.yaml` and running
  `go generate ./internal/errs`.
* Bad, because the generator and its YAML library are more code to
  maintain than a list of constants.

### Confirmation

* Tests in `internal/errs` compare the generated files with what the
  registry renders to, compare every code with the specification's seed
  table (condition, exit code and device state, with invariants named),
  check that the registry holds exactly the seed codes plus the four
  additions, and cover every phase and every case of `ExitCodeOf`,
  `DeviceStateOf` and `CodeOf`. A property test and a fuzz target check
  the resolution of every code in every phase, as the specification asks
  of pure packages.
* Tests of the generator cover every validation rule, the rendering
  (golden files) and every finding of the repository check, on fixture
  trees.
* `scripts/check-errors.sh` runs in CI with the other guards, and
  `scripts/test-guards.sh` runs it on a tree that must pass and on one
  that must fail.

## Pros and Cons of the Options

### Registry file and generator

* Good, because the registry is data that can be validated, rendered and
  checked against the code that uses it.
* Good, because it is what the specification describes.
* Bad, because it needs a generator and a YAML library, though neither is
  part of any binary.

### Hand-written constants

* Good, because it needs no generator and no dependency.
* Bad, because the documentation would be written separately and drift
  from the constants.
* Bad, because rules such as "the exit code and device state agree" would
  have to be enforced by reflection over hand-written tables, or by
  review.

### No codes

* Good, because it costs nothing now.
* Bad, because failures could only be told apart by their English text,
  which scripts and CI cannot rely on and which changes.
* Bad, because exit codes alone cannot say which of many problems
  happened, and the specification's stability contract promises codes.

## More Information

The specification was amended in the same pull request: its error model
states that codes are registered as reserved or active and that the
producing test applies to active codes, and gives the exit codes of
phase-dependent codes when nothing is known about device-side work, when
the caller states that none was involved, and when its outcome is unknown; the guard-script table lists what `check-errors.sh` rejects;
and the four additions appear in its seed error-code table, marked
"(addition, ADR 0019)". The status of this record
stays "proposed", as the specification labels it.
