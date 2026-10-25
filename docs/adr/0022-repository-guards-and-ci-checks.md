---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Implement the repository guards and CI checks with these refinements

## Context and Problem Statement

The specification describes the repository guards (scripts that fail CI
when a project rule is broken) and the CI workflow in its engineering
standard. When they were implemented, several details turned out to be
too loose, too strict or inconsistent with other parts of the
specification:

* The guard table says the guards run inside the lint job, while the
  branch policy lists `guards` as its own required check.
* The CI table measures coverage with `-coverpkg=./...`. The guard
  program is itself Go code in the module, and today it is 726 of the
  module's 784 statements (92.6%), so it would hold up the coverage of
  the product code.
* The real-data guard is limited to examples, test data, the lab and the
  documentation, although addresses can also leak through code and
  configuration; it allows any `example.*` domain, although only
  `example.com`, `example.net` and `example.org` are reserved.
* The specification's license section asks for a guard on `go.mod` but
  the guard table does not list it.
* The executor rule lists only the operator-side packages, although the
  executor must also stay small and never initiate connections.
* The pure-package rule forbids every I/O package, which would also
  forbid `net/netip` (address values only) and `log/slog` (records and
  handlers; a handler writes to the writer it is given).

Which rules do the guards and CI enforce?

## Decision Drivers

* The required check names must be stable and match the branch policy.
* The coverage floor must measure the product, not the tooling.
* A guard must catch the realistic ways a rule is broken (no false
  negatives), without blocking legitimate text.
* Every rule must be written down where contributors look for it: the
  specification, CLAUDE.md and CONTRIBUTING.

## Considered Options

* Follow the specification's text literally
* Refine the guards and CI checks and amend the specification

## Decision Outcome

Chosen option: "Refine the guards and CI checks and amend the
specification", because the literal text either contradicts another
part of the specification or lets violations through.

The refinements:

* **A separate `guards` job** runs the guard self-tests and every
  `scripts/check-*.sh`, including the executor dependency and size
  check. It is a required check of its own, as the branch policy lists.
* **The coverage floor (80%) counts `cmd/` and `internal/` only**
  (`-coverpkg=./cmd/...,./internal/...`). The guard program in
  `scripts/` is still tested in the same run, and its own coverage is
  reported separately.
* **The real-data guard** checks IP and MAC addresses in every file, and
  domains in examples, test data, the lab, schemas, the documentation,
  root Markdown files and GitHub templates. Only `example.com`,
  `example.net`, `example.org` and the reserved `.example`, `.test` and
  `.invalid` names count as examples; intranet names (`.local`, `.lan`,
  `.corp`, `.internal`, `.intranet`, `.home.arpa`, `.localdomain`) are
  always reported unless listed. Domains are recognized against the
  IANA list of top-level domains, kept in the repository.
* **A `check-gomod.sh` guard** requires every module that `go.mod`
  requires or uses as a replacement to be listed with an allowed license
  and a justification, and rejects a `replace` into `lab/`. The license
  CI job checks the modules' real licenses with go-licenses.
* **The SPDX guard also covers shell scripts** (`.sh`, second line).
* **The executor rule** also keeps `cmd/irekey-exec` and every package
  under `internal/exec` away from `execbin`, `transport`, `transfer` and
  `rollout`, and allows no module outside the standard library unless it
  is listed. A later change that needs one (for example compression or
  signature verification) adds it to the list with a decision record.
* **Pure packages may import `net/netip` and `log/slog`.** They still may
  not import `log` itself, `os`, `os/exec`, `net`, `syscall`,
  `path/filepath` or `crypto/rand`. Because `log/slog`'s package-level
  functions write to standard error through the default logger, pure
  packages must not call them; the import check cannot see this, so it
  is a review rule.
* **The DCO check has no exemption.** Dependabot adds its own
  `Signed-off-by:` line to the commits it creates.

### Consequences

* Good, because the required checks, the guards and the specification
  agree.
* Good, because a low-coverage product change can no longer pass behind
  the guard program's tests.
* Bad, because the real-data guard can report legitimate text, such as
  a four-part version number that is lexically an address; such a case
  needs a rewording or an allowlist entry with a comment.

### Confirmation

The guard self-tests (`scripts/test-guards.sh`) run every guard against
a tree that must pass and a tree that must fail, and the guard program's
unit tests cover each rule, including the false-positive cases. The
architecture tests in `internal/archtest` check the dependency rules on
Linux, Windows and macOS, and the executor rules on amd64, arm64 and
ARMv7.

## Pros and Cons of the Options

### Follow the specification's text literally

* Good, because nothing in the specification changes.
* Bad, because the guard table and the branch policy disagree on where
  the guards run.
* Bad, because the coverage floor would be met mostly by tooling.

### Refine the guards and CI checks and amend the specification

* Good, because each refinement closes a gap or removes a contradiction.
* Neutral, because the specification's engineering standard gets small
  amendments, each pointing to this record.

## More Information

The amended parts of the specification are the dependency rule, the
testing strategy, the guard table and the CI table; each amended line
points to this record. The IANA list of top-level domains comes from
https://data.iana.org/TLD/tlds-alpha-by-domain.txt and is refreshed as
described at the top of `scripts/guard/tlds.txt`.
