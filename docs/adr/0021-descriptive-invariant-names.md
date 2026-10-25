---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Refer to safety invariants by descriptive names outside the specification

## Context and Problem Statement

The specification defines twenty safety invariants, the rules irekey never
breaks (for example: no plaintext secret in logs or output; a device is
switched with one atomic rename). It gives each one a number in its
invariants table, and originally asked for that number to be used
everywhere: code comments of the form `// Invariant I<N>`, tests named
`TestInvariant_I<N>_...`, and the documentation.

A number says nothing to a reader who does not have the specification's
table open. In a failing test name, a code review, a commit message or the
assurance case, the number has to be looked up, while a name such as
"data separation" is understood immediately.

How should code, tests and public documents refer to an invariant?

## Decision Drivers

* A reader of a test name, a comment or a document should understand which
  rule is meant without opening the specification.
* Every invariant must still be traceable to its definition and to the
  tests and code that protect it, with a plain text search.
* Names must be stable, because tests are named after them.

## Considered Options

* Descriptive names outside the specification
* Numbers everywhere
* Numbers and names together

## Decision Outcome

Chosen option: "Descriptive names outside the specification", decided by
the maintainer, because names are self-explanatory where they are read,
and the specification's table still maps each number to its name for
anyone who needs the cross-reference.

The names are fixed in the specification's invariants table. The short
meanings below are summaries; the table holds the full rules.

| Name | Short meaning |
|---|---|
| `PinnedIdentity` | no operation runs unless the device presents its pinned SSH host key; state-changing operations also need a matching MAC address set |
| `SignedOnly` | only bundles signed by a trusted signer, with every file matching its digest, are applied |
| `SnapshotBeforeChange` | no transfer or activation without a snapshot taken in the same connection; untriaged drift blocks updates |
| `AtomicSwitch` | a release set is switched with one atomic rename of the `current` symlink |
| `ExplicitPointOfNoReturn` | before the switch, abort leaves the device unchanged; after it, any failure triggers rollback |
| `ConnectionIndependent` | device-side state changes run detached from the SSH session |
| `CrashSafe` | every executor step is idempotent and journaled, so recovery after power loss is deterministic |
| `FunctionalHealth` | success requires every critical health check to pass, judged against the pre-update baseline |
| `WholeSetRollback` | rollback restores the previous release set as a whole |
| `DataUntouched` | declared data paths are never written, moved or deleted by irekey, and are not snapshotted |
| `DiskBudget` | no transfer starts without verified free space and inodes, including a safety margin |
| `DowntimeBound` | in multi-device operations, devices whose estimated downtime exceeds the maximum are skipped |
| `FailureBudget` | a wave halts when its failure budget is exceeded |
| `DataSeparation` | no plaintext secret in the deployment repository, logs, CLI output or local cache; no customer data in the public repository |
| `PathBack` | an activation is kept only after the operator confirms it over a new SSH connection |
| `NothingPermanentRunning` | no irekey process, timer or listener exists on a device outside an activation window |
| `OneOperationPerDevice` | one state-changing operation per device at a time |
| `HumansDecideDrift` | drift is never overwritten or absorbed without an explicit, recorded human decision |
| `NoFlagDisablesSafetyCheck` | no command-line flag turns a safety check off |
| `ApprovedStateOnly` | field devices receive only state from a clean commit reachable from the remote protected branch |

Conventions:

* Tests that protect an invariant are named after it:
  `Test<Name>_<behavior>`, for example
  `TestDataSeparation_WriterRedactsAcrossChunkBoundaries`.
* Code comments that explain how a piece of code upholds an invariant use
  the same name: `// Safety (DataSeparation): ...`. A search for the name
  therefore finds both the tests and the code.
* Public documentation (stability contract, architecture, threat model,
  assurance case), commit messages and pull requests use the names.
* The specification keeps the numbers for its own cross-references, next
  to the names.

### Consequences

* Good, because test output and code review are readable without the
  specification at hand.
* Good, because all tests for one invariant share a prefix and can be run
  together, for example with `go test -run '^TestDataSeparation' ./...`.
* Bad, because a renamed invariant would require renaming its tests and
  comments; the names are therefore treated as stable.

### Confirmation

Code review, and a search of the Go source files (`*.go`) for
`TestInvariant_` or for `Invariant I` followed by a digit, which must find
nothing. This record quotes the old convention, so it is not part of that
search.

## Pros and Cons of the Options

### Descriptive names outside the specification

* Good, because every occurrence explains itself.
* Neutral, because the specification needs one extra column to map
  numbers to names.

### Numbers everywhere

* Good, because numbers are short and never ambiguous.
* Bad, because every occurrence needs a lookup to be understood.

### Numbers and names together

* Good, because it is both traceable and readable.
* Bad, because it is long and repeats the same information in every test
  name and comment.

## More Information

The specification's invariants section and testing strategy were amended
in the same pull request as this record.
