# Changelog

All notable changes to irekey are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/). Each version uses the sections
Added, Changed, Deprecated, Removed, Fixed and Security, as needed.

## [Unreleased]

### Added

- The project specification (`docs/spec/masterprompt.md`), the MIT license
  and the repository bootstrap files.
- Architecture decision records in `docs/adr/`: the template, an index,
  ADR 0001 (record architecture decisions), ADR 0020 (project name) and
  ADR 0021 (descriptive invariant names).
- The Go module `github.com/hitoshyamamoto/irekey` (Go 1.27, built with
  the go1.27.3 toolchain) and the package layout, with a package comment
  in every package that has no code yet.
- `irekey version`, a first minimal command that prints the version,
  commit, Go version and platform; the full command line comes later.
- `irekey-exec capabilities`, the first subcommand of the device-side
  executor: it prints one JSON line describing the executor.
- Architecture tests that enforce the package dependency rule, including
  that the executor never depends on operator-side packages.
- Repository guards in `scripts/`: SPDX headers, no disabled host-key
  checking, no remote commands built by string formatting, no real-world
  data, English-only text, allowed dependency licenses, and executor
  dependencies and size; each guard has its own self-test.
- CI workflows: lint, guards, tests with an 80% coverage floor, short
  fuzzing, vulnerability check, Developer Certificate of Origin check,
  cross-platform builds with a smoke test, license check, CodeQL,
  OpenSSF Scorecard, weekly fuzzing, workflow analysis with zizmor, and
  optional Dependabot auto-merge; Dependabot updates for Go modules and
  GitHub Actions.
- `CLAUDE.md`, a summary of the working rules and commands for coding
  assistants.
- ADR 0022, which records how the repository guards and CI checks refine
  the specification (separate guards job, coverage over the product code,
  broader real-data guard, the go.mod guard, stricter executor rule).

### Changed

- The project is named irekey. The specification used the working name
  irekeys; every name derived from it (module path, binaries, device
  paths, schema identifiers, signature namespace, image names) now uses
  irekey. See ADR 0020.
- Safety invariants are referred to by descriptive names (for example
  `DataSeparation`) in code, tests and public documentation, instead of
  by their numeric IDs. See ADR 0021.

## Ideas on record

Suggestions outside the current scope, kept here so they are not lost.
Each one needs a real field case before it is considered.
