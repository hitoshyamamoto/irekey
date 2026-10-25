---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Keep local operational state in SQLite with a pure-Go driver

## Context and Problem Statement

Besides the deployment repository, irekey needs state that matters only
to the operator machine it runs on: jobs, per-device progress, the
pending queue of work blocked by unreachable devices, cached device
manifests for transfer size estimates, the full snapshot history
(including snapshots taken implicitly by commands other than `adopt` and
`snapshot`), timings, and device logs collected after a failed health
check. A command may write while `irekey ui` reads, two commands may run
at once, and a laptop may be closed or killed mid-operation. The binary
is static and built without cgo for Linux, macOS and Windows on amd64
and arm64.

Where does irekey keep this state, and what may it contain?

## Decision Drivers

* Anything another developer needs lives in the deployment repository,
  never only on one machine.
* No plaintext secret in local state (DataSeparation); collected logs
  may contain customer data, so they stay local and redacted.
* Atomic updates that survive a killed process; separate processes read
  and write at once.
* No cgo, a permissive license, active maintenance, and no noticeable
  delay at 500 devices with 10 components each.

## Considered Options

* SQLite with a pure-Go driver
* bbolt
* JSON files
* No local state

## Decision Outcome

Chosen option: "SQLite with a pure-Go driver", because it gives
transactions, crash recovery, concurrent readers and queries in a single
file without cgo.

* `internal/state` owns the database and its migrations; the default
  driver is `modernc.org/sqlite`.
* It stores only the operational state listed above, never secrets,
  triage decisions, release sets, history summaries or anything else
  another developer needs; the graph is built in memory on every run.
  Losing the database loses caches, the local queue and local logs,
  never a decision.
* The executor must not import `internal/state`, so SQLite never reaches
  the device binary.

This is the recommended default, and the specification keeps the driver
choice open until it is confirmed. It is confirmed or revised when local
state is implemented, with evidence from building and testing on all six
operator targets, from a command writing while the web UI reads, from
crash tests, and from the effect on binary size and maintenance status.
If `modernc.org/sqlite` fails those checks, another cgo-free driver is
evaluated; if local state stays small and simple, JSON files could
replace the database.

### Consequences

* Good, because queue and progress updates are transactional, so a
  killed process leaves either the old or the new state.
* Good, because SQLite lets readers work while another process writes
  (in write-ahead log mode), so the web UI can follow a running
  operation, and history can be queried by device or rollout.
* Bad, because the driver is a large dependency, translated
  mechanically from SQLite's C sources; it adds to the build time and
  size of the operator binary and is harder to review than ordinary Go.
  The local schema also needs its own migrations.
* Neutral, because work queued on one machine is invisible to another
  developer; the device journal and lease (OneOperationPerDevice) and
  the deployment repository's history remain the shared record.

### Confirmation

* The architecture test that `cmd/irekey-exec` does not depend on
  `internal/state`.
* DataSeparation tests write known secret values through every command
  path and search the database file for them.
* A test deletes the local database and checks that commands still work
  from the deployment repository, rebuilding their caches.
* Code review: a table or column holding information another developer
  needs is not merged; it belongs in the deployment repository.

## Pros and Cons of the Options

### SQLite with a pure-Go driver

* Good, because SQLite is a widely used embedded database, and
  `modernc.org/sqlite` needs no cgo and supports Linux, macOS and
  Windows on amd64 and arm64.
* Neutral, because cgo-based SQLite drivers are excluded by the
  static-binary requirement, so the choice is among cgo-free drivers.
* Bad, because the dependency is large.

### bbolt

`go.etcd.io/bbolt`, an embedded key/value store in pure Go.

* Good, because it is small, pure Go and transactional.
* Bad, because its file lock lets no other process open the database
  while one has it open for writing, so the web UI could not read during
  a running command; every query and index is hand-written code.

### JSON files

* Good, because there is no dependency and any editor can read them.
* Bad, because atomic updates, locking between processes and crash
  safety would be hand-written for every file, and every query reads
  and parses whole files that grow with history.

### No local state

Everything is kept in the deployment repository or recomputed.

* Good, because there is nothing local to migrate, lose or corrupt.
* Bad, because progress, queues and implicit snapshots would leave the
  deployment repository dirty during ordinary commands, and collected
  device logs, which may contain customer data, must not go there.

## More Information

* `modernc.org/sqlite` v1.61.0 (2026-10-21), BSD-3-Clause, pure Go,
  canonical source https://gitlab.com/cznic/sqlite, supports linux,
  darwin and windows on amd64 and arm64:
  https://pkg.go.dev/modernc.org/sqlite. Its package documentation calls
  it "a CGo-free port of the C SQLite3 library", and its README says
  that "most of the Go here is generated from C":
  https://gitlab.com/cznic/sqlite/-/blob/v1.61.0/README.md
* SQLite, "Write-Ahead Logging": "readers do not block writers and a
  writer does not block readers", for processes on the same host and not
  over a network filesystem: https://www.sqlite.org/wal.html
* bbolt, module `go.etcd.io/bbolt`, v1.5.0, MIT. Its README: "Bolt
  obtains a file lock on the data file so multiple processes cannot open
  the same database at the same time"
  (https://github.com/etcd-io/bbolt/blob/v1.5.0/README.md); `Open` in
  `db.go` takes an exclusive lock unless the database is opened
  read-only, which takes a shared lock:
  https://github.com/etcd-io/bbolt/blob/v1.5.0/db.go
* All checked 2026-10-25.
* Specification sections "Zones", "Architectural decisions",
  "Dependency rule", "Snapshot, status and drift", "Delivery" and
  "History"; its open questions include confirming the pure-Go SQLite
  driver.
