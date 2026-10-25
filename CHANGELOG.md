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
- Governance and community files: README (status, the name, planned
  quick start, building from source, what irekey is not, sponsors),
  CONTRIBUTING, GOVERNANCE, SECURITY (including what the security model
  covers today and what is only designed), STABILITY (the compatibility
  contract), ROADMAP, CODE_OF_CONDUCT (Contributor Covenant 2.1), the
  threat model in `docs/threat-model.md`, the OpenSSF Best Practices
  answers in `.bestpractices.json`, funding metadata (`funding.json`,
  `.well-known/funding-manifest-urls`, `.github/FUNDING.yml`) and issue
  forms for bug reports and feature proposals.
- ADRs 0002 to 0018, the founding architecture decisions, with their
  sources (Go, the MIT license, push over SSH with nothing permanent on
  devices, layers and patch stacks, release sets, the private deployment
  repository, go-git and system git, in-process SSH, the persistent
  confirmation guard, privilege through sudo, vault providers, SQLite for
  local state, YAML with JSON Schema, the embedded web UI, SSHSIG bundle
  signatures, the containerized lab, the release process).
- `docs/open-questions.md`: the assumptions the design rests on and every
  open question, with the behavior while it stays open.
- `docs/glossary.md`, generated from the specification's glossary and
  kept in sync by a test, which also rejects specification labels in it.
- The error-code registry, `internal/errs/codes.yaml`: the twenty code
  categories, every seed code of the specification and four additions
  (`USR-005` invalid command line, `USR-006` interrupted, `LOC-004`
  directory not writable, `TIME-002` operator machine clock behind the
  build time), 78 codes in all. Every code is reserved until a code path
  produces it.
- Package `errs`: a constant per code, the typed error that carries a code
  and the phase of the operation, the exit codes, the device-state and
  outcome enums, and the resolution of any error to an exit code and a
  device state. No fallback claims a state that nothing has proven: an
  error without a code is reported as `BUG-003` and an interruption as
  `USR-006`, both with the device state `unknown`.
- `errsgen`, the generator run by `go generate ./internal/errs`: it
  validates the registry (form and order of the codes, categories, and
  exit codes that agree with device states) and writes the Go constants
  and `docs/errors.md`, the reference of every error code, exit code and
  device state. It uses `go.yaml.in/yaml/v3`, which no irekey binary
  links.
- `scripts/check-errors.sh`, a guard that fails when code spells an error
  code as a string, uses a code that is not registered or still reserved,
  imports package `errs` with a dot, or leaves an active code unused or
  without a test that refers to it, when Markdown mentions an unregistered
  code, or when the registry is invalid or the generated files are stale.
- ADR 0019 (stable error-code registry), proposed.
- The redaction package (`internal/redact`), which keeps secrets out of
  output and logs. Registered secrets of at least four bytes, and the
  escaped forms they take in JSON, quoted and sanitized text, become
  `[REDACTED]` in strings, in streamed output whatever its chunk
  boundaries, and in `log/slog` records: the message, keys, group names
  and values, checked in every form the stock handlers print and against
  the secrets known when each record is logged. Overlapping secrets are
  redacted whole. The package also escapes, in text read from devices,
  control and format characters (bidirectional text controls among them),
  line and paragraph separators, private-use and unassigned code points
  and invalid UTF-8 before the text reaches a terminal.
- Package `internal/execbin`, which embeds the device-side executors:
  `go generate ./internal/execbin` builds `irekey-exec` for linux/amd64,
  linux/arm64 and linux/arm/v7 with a fixed set of go command settings,
  fails if the linux/amd64 executor exceeds 12 MiB, and stores each
  executor as a single gzip member with a fixed header, next to a manifest
  of the SHA-256 digest and size of each executor and of its compressed
  file.
  The same commit in a clean checkout, built with the same Go toolchain,
  gives the same executors, and compressing identical executor bytes is
  deterministic. The digest that identifies an executor is that of the
  uncompressed binary, the value a release's `SHA256SUMS` will list for
  `irekey-exec-linux-<arch>`. The package loads the manifest and verifies
  the executors, compressed and decompressed; without the generated files,
  its API reports that no executor is embedded. The operator command line
  does not use the package yet. The CI build job builds the executors with
  the generator, requires the embedded set to verify, and checks the
  executor release assets against the manifest.

### Changed

- The project is named irekey. The specification used the working name
  irekeys; every name derived from it (module path, binaries, device
  paths, schema identifiers, signature namespace, image names) now uses
  irekey. See ADR 0020.
- Safety invariants are referred to by descriptive names (for example
  `DataSeparation`) in code, tests and public documentation, instead of
  by their numeric IDs. See ADR 0021.
- Specification: the glossary entry for "gate" describes the gates in
  words; `find` is attributed to findutils; a new open question asks
  whether the executor's health checks may reach the device's own
  services on loopback; AGPL software (Vaultwarden) gets the same
  isolation as GPL software in the lab.

## Ideas on record

Suggestions outside the current scope, kept here so they are not lost.
Each one needs a real field case before it is considered.
