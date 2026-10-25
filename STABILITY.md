# Stability

This document says which parts of irekey are a compatibility contract,
from when, and how the contract is enforced. The security model is in
[SECURITY.md](SECURITY.md); the change history is in
[CHANGELOG.md](CHANGELOG.md).

## Before 1.0

irekey has no release yet. Versions follow
[Semantic Versioning](https://semver.org/); before 1.0 they are
`0.MINOR.PATCH`.

Before 1.0 anything may change: commands, flags, output, file formats and
codes. Every such change has a changelog entry, under "Changed" or
"Removed", that says what to do instead.

## From 1.0

From version 1.0 on, these are promises:

| Contract | Rule |
|---|---|
| Command-line commands, flags and `--json` output | Additions only. Changing or removing anything needs a deprecation first: a warning for at least two minor versions, and removal only in a major version. |
| Exit codes and error codes | A released code never changes meaning. |
| Bundle format 1 | Every format-1 executor reads every format-1 bundle. Extensions follow the critical-extension rule: an executor refuses a bundle that carries a critical extension it does not know, and ignores unknown non-critical fields. |
| Device layout and journal format | Any newer version reads what any version since 1.0 wrote on a device, and can roll back a device updated by the previous minor version. |
| Deployment repository schemas | Every file names its schema and version (`schema: irekey/<kind>/v<N>`). A newer irekey reads older versions and provides a migration command; an older irekey refuses newer versions and names the version it needs. |
| Canonical variant encoding | Stable. A change is a new encoding version, and variant IDs computed with an older version stay resolvable. |
| Safety invariants | Never weakened. |
| Go API | Not a contract. Every package other than the commands is under `internal/`, so no other module can import it. |
| Human-readable output and the web interface layout | Not contracts. Scripts should rely on `--json` and the exit codes. |

The planned exit codes are listed in the specification's command-line
interface section, and the error codes in its error model. Only a few
exit codes are used so far: 0 (success) and 1 (usage error) by both
existing commands, and 15 (internal error) by the executor when it cannot
write its output. No error code is implemented yet.

### Safety invariants

The safety invariants are the rules irekey never breaks. They are defined
in the specification's invariants table and summarized in
[ADR 0021](docs/adr/0021-descriptive-invariant-names.md). "Never weakened"
means that a release may make an invariant stricter, never looser, and that
no command-line flag turns one off; the only overrides are the interactive
confirmations the specification lists, and they are recorded in history.

The invariants are `PinnedIdentity`, `SignedOnly`, `SnapshotBeforeChange`,
`AtomicSwitch`, `ExplicitPointOfNoReturn`, `ConnectionIndependent`,
`CrashSafe`, `FunctionalHealth`, `WholeSetRollback`, `DataUntouched`,
`DiskBudget`, `DowntimeBound`, `FailureBudget`, `DataSeparation`,
`PathBack`, `NothingPermanentRunning`, `OneOperationPerDevice`,
`HumansDecideDrift`, `NoFlagDisablesSafetyCheck` and `ApprovedStateOnly`.

## Deprecation

From 1.0, a command, flag or `--json` field that is to change or go away:

1. keeps working and prints a deprecation warning for at least two minor
   versions;
2. is listed under "Deprecated" in the changelog;
3. is removed only in a major version, and listed under "Removed".

## How the contract is enforced

Each mechanism below runs as part of the test suite in CI. Only the first
one exists today; the others come with the features they protect.

| Mechanism | What it protects | Status |
|---|---|---|
| `docs/schema-versions.txt` and its test in `internal/schema` | The schema identifiers this build reads. The test fails when the list and the code differ; `go test ./internal/schema -update` rewrites the list for review. | Exists. The list is empty, because no schema is defined yet. |
| Golden files (`testdata/*.golden`, rewritten with `-update` and reviewed in the diff) | `--json` output and the canonical variant encoding | Not yet: they come with the first `--json` output and with the variant code. |
| Compatibility zoo (`testdata/compat/<version>/`) | Bundles, journals and deployment repository files written by every released version, which the current code must still read | Not yet: the directory exists, and files are added from the first release on. |
| Tests named after each invariant (`Test<Name>_<behavior>`) | The safety invariants | Not yet: they come with the code that relies on each invariant. |
| Error code registry (`internal/errs/codes.yaml`), with generated Go constants and documentation | Error codes and the exit code of each | Not yet: designed, not implemented. |
