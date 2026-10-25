---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# License irekey under the MIT license

## Context and Problem Statement

irekey is public open-source infrastructure tooling. Teams run it on
their own machines against their own devices, and some will adapt it to
their needs or build it into internal tooling. Its license decides who may
use, modify and redistribute it, and under which conditions. It also
constrains which third-party code irekey can depend on, because the
binaries are statically linked and contain every Go dependency.

Which license does irekey use, and which licenses may its dependencies
have?

## Decision Drivers

* Anyone, including companies, can use, modify and redistribute irekey
  with as few conditions as possible.
* The license text is short and widely understood, so adopters can start
  without a long legal review.
* Statically linked binaries must carry the obligations of every
  dependency they contain.
* The policy for dependency licenses must be checkable in CI.

## Considered Options

* MIT
* Apache-2.0
* GPL-3.0

## Decision Outcome

Chosen option: "MIT", decided by the maintainer, because it is permissive,
short and widely understood, and it places the fewest conditions on the
teams that adopt or adapt irekey.

Policy that follows from this decision:

* `LICENSE` holds the MIT text with `Copyright (c) 2026 Hitoshi Yamamoto`.
* Every Go source file starts with `// SPDX-License-Identifier: MIT`.
* Go dependencies may use MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0,
  ISC or MPL-2.0. MPL-2.0 is a file-level copyleft; as the project reads
  its clause 3.2, whoever receives a binary that contains MPL-2.0 files
  must be able to obtain their source and be told how, so adding an
  MPL-2.0 dependency requires a note in an ADR that says how this is met.
  Any other license requires its own ADR.
* GPL-licensed software, such as the IEC 60870-5-104 simulator used by the
  lab (GPL-3.0), runs only inside separate lab container images, as a
  separate process. It is never a Go dependency of irekey and is never
  linked into or embedded in its binaries (ADR 0017).
* Each release ships a `THIRD_PARTY_NOTICES` file generated from the
  licenses of the Go dependencies, so the binaries carry the notices that
  those licenses require.

### Consequences

* Good, because irekey can be used, adapted and redistributed, also in
  closed internal tooling, on the single condition that the copyright and
  permission notice is kept.
* Good, because code under every allowed dependency license can be linked
  into irekey's binaries, provided the notices ship with them and, for
  MPL-2.0, the source stays available.
* Bad, because MIT has no explicit patent grant.
* Bad, because improvements made by others do not have to be published;
  the project relies on goodwill for contributions back.

### Confirmation

The specification's CI design confirms this decision: the `licenses` job
runs `go-licenses check` with the allowed licenses and fails on any other,
a guard script fails when a Go file lacks the SPDX header, and a check of
`go.mod` fails when a GPL-licensed module appears.

## Pros and Cons of the Options

### MIT

* Good, because it allows use, modification and redistribution with one
  condition: keeping the copyright and permission notice.
* Good, because its whole text fits on one screen and is known to most
  developers and legal teams.
* Bad, because it does not grant patent rights explicitly.

### Apache-2.0

This license was considered as the alternative permissive license.

* Good, because it is permissive and includes an explicit patent license
  from every contributor.
* Good, because it is widely used and well understood by companies.
* Neutral, because it asks that modified files carry notices of the
  changes and that a project's `NOTICE` file, if there is one, is passed
  on with redistributions.

### GPL-3.0

* Good, because distributed modified versions must stay open, so
  improvements come back to everyone.
* Bad, because adopters who build irekey into their own tooling would
  have to license that tooling under the GPL when they distribute it
  (clause 5(c) applies the license to the whole work).
* Bad, because irekey uploads its executor to devices owned by
  customers; whether that is conveying under the GPL, which the license
  defines as "any kind of propagation that enables other parties to make
  or receive copies", is a question each adopter would need legal advice
  to answer.

## More Information

* SPDX license list: https://spdx.org/licenses/
* `go-licenses`: https://github.com/google/go-licenses; `go-licenses
  check` takes `--allowed_licenses=<comma separated license names>`.
* License texts, checked on 2026-10-25. MPL-2.0, clause 3.2
  "Distribution of Executable Form": the software "must also be made
  available in Source Code Form" and recipients must be informed how to
  obtain it: https://www.mozilla.org/en-US/MPL/2.0/. Apache-2.0: patent
  license in clause 3, notices of changes in clause 4(b), the `NOTICE`
  file in clause 4(d): https://www.apache.org/licenses/LICENSE-2.0.
  GPL-3.0: "convey" in clause 0, the whole work in clause 5(c):
  https://www.gnu.org/licenses/gpl-3.0.html. These are the project's
  readings of the license texts, not legal advice.
* The specification's license compliance rules list the allowed
  dependency licenses and the CI checks.
