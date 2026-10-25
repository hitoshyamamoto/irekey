# Open questions

This file lists the design points of irekey that are not decided yet, and
the assumptions the design rests on. The
[specification](spec/masterprompt.md) is the source of both lists; this
file explains each entry and tracks it until it is answered.

An open question is a design point that the specification labels Open:
nobody has decided it yet. irekey does not implement behavior that depends
on an open question. Where the code meets one, it leaves an extension
point instead, such as a Go interface with a single implementation, so
that either answer can be added later without reworking what exists. Each
entry below states what irekey does while the question is open.

When a question is answered, the answer is recorded in an architecture
decision record (see [docs/adr](adr/README.md)) and in the specification,
in the same pull request. The entry is then removed from this file, or
moved to "Resolved questions" at the end with a one-line answer and a link
to that record. Questions found while irekey is being built are added
here.

Each open question has four parts:

* **Question**: what is not decided.
* **Why it matters**: the parts of the design that depend on the answer.
* **While open**: what irekey does until the question is answered.
* **What would answer it**: the facts, measurements or decision needed.

## Assumptions

The design rests on the assumptions below. None of them is proven yet.
Each one is checked as described; an assumption that turns out to be false
is added to this file as an open question, together with the parts of the
design that depend on it.

### Customer variation is mostly data

Most of what differs between customers can be expressed as configuration
values in layers; only a small part needs code changes in patch stacks.

Checked by the first extraction runs on adopted devices, before more
tooling is built on top of extraction. Triage records the destination of
every cluster of changes, so the share of clusters that become
configuration values, compared with those that become patches, measures
the assumption. Afterwards, `irekey status --debt` keeps the measure
current by reporting the total changed lines in customer and device
patches. If much code diverges, extension points in the components
themselves come before more irekey tooling.

### Components can be described by a manifest

Every component can be described by a component manifest
(`.irekey/component.yaml`) with its paths, runtime, health checks and
hooks, using the common fields the manifest schema defines. This is the
precondition for plug-and-play use: irekey needs no code specific to a
component.

Checked by writing a manifest for each of the five fictional components of
the lab, then for each component of the product being managed. Component
discovery during adoption checks the device side: a layout it cannot place
triggers one question, and the answers show which layouts the discovery
heuristics miss. A component that needs something the manifest cannot
express is recorded here before the schema grows.

### Component data can live outside the release directories

Data that components write at runtime can live in declared data paths
(`paths.data` in the component manifest), outside the release directories
under `/opt/irekey/releases/`, which are read-only once prepared; every
switch makes a different release directory current.

Checked by declaring the data paths when each component's manifest is
written. Snapshots cover each component's paths and exclude declared data
paths, logs and caches, so files that a component writes inside its own
paths without declaring them show up as drift at every connection, even
when no update took place; such drift points to an undeclared data path.
A component that must write inside its own code directory cannot be
delivered by irekey until it is changed to write elsewhere.

### Developer machines have git

The machines of the developers who change adjustments have `git`
installed. irekey needs it only for patch operations (`git am`, `apply`,
`format-patch`, `rebase`, `rerere`, `range-diff`).

Checked by `irekey doctor`, which checks the git version on the operator
machine and explains how to fix a problem; patch operations fail with
`LOC-001` when git is missing or too old. Adoption must work without
system git, so a missing git never blocks the first device. The minimum
git version still has to be verified against the commands irekey uses.

### Devices run systemd and an OpenSSH server

Devices run systemd and an OpenSSH server, and most of them have GNU
`find` (from findutils) and `sha256sum` (from coreutils).

Checked by adoption itself: it connects over SSH, reads
`systemctl --version` and other facts with read-only commands, and records
them in the device file. State-changing operations refuse a device without
a usable systemd with `PLT-002`. When `find` or `sha256sum` is missing,
the snapshot uploads the executor to a temporary directory and runs an
unprivileged scan instead, so missing tools cost an upload but do not
block adoption. The platform facts of the adopted devices show how far the
assumption holds; devices without systemd are the subject of the question
on a guard for devices without systemd, below.

## Open questions

### Reaching several sites at once

**Question.** Can an operator machine reach several customer sites at the
same time, for example through a VPN concentrator, or only one customer
VPN at a time?

**Why it matters.** It sets the useful default concurrency of
multi-device operations and shapes how rollouts are run: whether one
`irekey rollout run` can update devices of several customers, or whether
the operator works through one customer VPN after another.

**While open.** VPN management is out of scope, and irekey assumes nothing
about how the VPN is set up. Concurrency is a setting: a global limit
(default 3, `--concurrency`) and a limit per wave. A device that cannot be
reached is held in its pending queue, and the work resumes at the next
connection; `irekey rollout run` is idempotent, so running it again
continues the rollout. No scheduling that depends on which VPN is
connected is implemented.

**What would answer it.** A description of how operators connect to
customer VPNs in practice, and a test from one operator machine with two
or more customer VPNs connected at once.

### Triage before every update

**Question.** Must all drift on a device be triaged before any update
reaches it, or may an update go first, with the site's adjustments
recorded later?

**Why it matters.** It is the policy of the SnapshotBeforeChange
invariant. Devices that were edited by hand over SSH before adoption may
carry a lot of drift, and triaging it is the main work before their first
update. An update that goes first replaces the files that carry untriaged
adjustments, so those adjustments stop running until they are recorded in
layers or patches.

**While open.** Untriaged drift blocks updates. In multi-device
operations, a device with new drift since its last triage is skipped. No
flag overrides the block (NoFlagDisablesSafetyCheck). Triage lets the
developer postpone a cluster; whether a postponed cluster counts as
triaged is part of this question, and until it is answered, it does not.

**What would answer it.** The triage effort measured on the first adopted
devices, and a decision by the maintainer. An answer that lets updates go
first must still satisfy HumansDecideDrift: drift may be left unrecorded
only through an explicit, recorded human decision. Such an answer changes
a safety invariant, so it needs an ADR and an amendment of the invariant
in the specification.

### Restricted sudo bootstrap

**Question.** Should irekey offer an optional
`irekey bootstrap --restrict-sudo <device>` that installs
`/etc/sudoers.d/irekey`, allowing the SSH account to run only a fixed,
root-owned executor path under sudo?

**Why it matters.** It decides the privilege model. Today irekey uses
whatever sudo rights the SSH account already has; a restricted mode would
let a device allow only the executor, which narrows what a stolen device
credential can do as root. The sudoers file is a permanent artifact
outside the directories irekey owns on the device, so it touches the goal
that nothing is permanently installed or running on devices, and the list
of persistent artifacts in NothingPermanentRunning. It also changes how
the executor is installed and updated: today the first sudo call runs the
uploaded copy from a temporary directory, and installed copies live under
a path that contains their digest, so the path changes with every
executor version.

**While open.** irekey uses the SSH account's existing sudo, like
Ansible's `become`: passwordless sudo, or a sudo password from the vault
sent on standard input, never on the command line and never logged. Only
`irekey-exec` runs under sudo; every other remote command runs
unprivileged. `irekey doctor <device>` reports the sudo mode. irekey
writes no sudoers file. ADR 0011 (privilege through the account's sudo;
restricted sudo deferred) records this choice.

**What would answer it.** A decision by the maintainer, recorded in an ADR
before any implementation, as the specification requires for this
permanent artifact. The ADR would fix the executor path the sudoers rule
allows, how that path is updated, and how the file is validated
(`visudo -cf`) before it is installed. Like anything new on a device, the
mode must be optional, explicit, confirmed and logged.

### CLI messages in languages other than English

**Question.** Should the command-line interface offer its messages in
languages other than English?

**Why it matters.** It affects the human output of the CLI
(`internal/cli`). The repository is English only; translated messages
would be the first text in another natural language, so the answer also
changes that rule.

**While open.** Messages are in English only. Human-readable output is
not a compatibility contract, while `--json` output, exit codes and error
codes are; scripts therefore never depend on message text, and
translations could be added later without breaking them.

**What would answer it.** Demand from real users who cannot work with
English messages, and a decision on where translations would live given
the English-only rule.

### Hardware watchdog on the reference device

**Question.** Does the reference device model have a hardware watchdog
that irekey could rely on?

**Why it matters.** It decides how a device recovers from a hang. The
guard is a systemd timer: it rolls back an unconfirmed activation when the
confirmation window elapses or after a reboot, but it cannot run while the
whole operating system is hung. A hardware watchdog would restart such a
device, after which the guard rolls back at boot.

**While open.** irekey neither uses nor configures a watchdog. A step that
exceeds its timeout is reported as a hung executor (`ENG-003`), with the
device state unknown. The guard covers devices that stay up or reboot; a
device that hangs completely stays as it is until someone restarts it.

**What would answer it.** The hardware documentation of the reference
device model, or a check on a bench device for a watchdog device (for
example `/dev/watchdog`). If one exists, using it is a persistent change
to the device's configuration and needs an ADR.

### Operating system versions and end of support

**Question.** Which operating system versions run across the devices, and
when does irekey stop supporting an old one?

**Why it matters.** It sets the platform requirements that release sets
declare (`requires`) and the test matrix: which Debian and Ubuntu versions
the lab images and integration tests cover.

**While open.** A release set may declare minimum operating system
versions; a device that does not meet them is refused with `PLT-001`. No
end-of-support policy is defined. The lab's device image is Debian with
systemd.

**What would answer it.** The platform facts that adoption records for
every device (from `/etc/os-release`), which give the actual distribution
of versions, and a decision by the maintainer on the support policy.

### Device accounts per person or shared

**Question.** Should each developer have a personal account on every
device, or should devices keep one maintenance account that developers
reach with their own keys?

**Why it matters.** It decides the audit trail: what the records on the
device can show about who connected and what they did. It also decides
how device files and the vault hold credentials.

**While open.** A device file names one SSH account and one credential
reference. Personal SSH keys are preferred; the vault holds passwords and
keys otherwise. Every delivery records who ran it in the history of the
deployment repository.

**What would answer it.** A decision by the maintainer, based on how
accounts are managed on the existing devices and on what the audit trail
must show.

### Frontend screen layouts at runtime

**Question.** Can the product's frontend load screen layouts at runtime,
or does every site need its own build?

**Why it matters.** It decides whether per-site layouts are configuration
in layers, rendered into the frontend's declared files with no rebuild, or
patch stacks that create one variant, and one build, per layout. Devices
with the same variant share one build and one test result, so the number
of variants sets how many builds and tests a release set needs.

**While open.** Nothing in irekey is specific to the frontend. Layout
differences found on devices go through extraction and triage like any
other change: irekey suggests a destination for each cluster and the
developer decides.

**What would answer it.** Reading the frontend component's code to see
whether layouts are loaded from files at runtime or compiled into the
build, and the extraction results for the frontend on adopted devices.

### A second signature on bundles

**Question.** Should bundles carry a second signature, from a release key
of the deployment repository's CI that attests the variant builds, in
addition to the developer's signature?

**Why it matters.** It shapes bundle format 1, which every format-1
executor must read from version 1.0 on. Today a developer's key alone
authorizes a bundle, so a compromised operator machine holding a trusted
key can sign anything; a second signature would also require proof that
the artifacts were built by CI.

**While open.** Bundles use format 0 with one SSHSIG signature by the
developer's key (namespace `irekey-bundle@v1`), verified on the device
against its trusted-signers file. The format already has critical
extensions: an executor refuses a bundle with a critical extension it does
not know (`BND-003`), so a second signature can be added later without an
older executor silently ignoring it.

**What would answer it.** A decision before bundle format 1 is fixed,
weighing the threat of a compromised operator machine against the cost of
managing a CI key, recorded in an ADR together with the format change.

### The SQLite driver

**Question.** Is the pure-Go driver `modernc.org/sqlite` the right choice
for irekey's local operational state?

**Why it matters.** The operator binary is built with `CGO_ENABLED=0` for
Linux, macOS and Windows, so the driver must be pure Go. It is part of
every operator build, so its license and maintenance status matter.
ADR 0013 (SQLite, pure Go, for local operational state) depends on the
answer.

**While open.** `modernc.org/sqlite` is the specification's default,
still to be verified. Only operational state lives in SQLite (jobs,
progress, the pending queue, snapshot and transfer caches); anything
another developer needs lives in the deployment repository. Local state
stays inside one package (`internal/state`) with its own migrations, which
keeps a change of driver inside that package.

**What would answer it.** Verifying the driver's current maintenance
status and its license, and those of its dependencies, against the allowed
licenses; building the operator binary with it for every operator
platform with `CGO_ENABLED=0`; and recording the result in ADR 0013.

### Existing tools for per-customer patches

**Question.** Does any existing tool already extract and manage
per-customer patches across many devices?

**Why it matters.** It decides irekey's positioning. Extracting,
maintaining and re-applying each customer's adjustments on top of a moving
base is the capability that sets irekey apart; if another tool already
does it, irekey should learn from it, work with it, or narrow its scope.

**While open.** irekey is designed on the basis that no existing tool
covers this. The specification's prior-art table lists the tools irekey
learns from (for example Mender, quilt, Google `repo` and Ansible) and why
none of them is used directly.

**What would answer it.** A documented search, starting with the terms
"fleet configuration management embedded", "per-customer patch
management" and "edge device GitOps offline", with the findings added to
the prior-art table and, if they change the design, to an ADR.

### A guard for devices without systemd

**Question.** Should devices without systemd get a guard built from cron
(`@reboot`) and a sleep-based watchdog?

**Why it matters.** The guard is the path back after an activation
(PathBack): it rolls back when confirmation does not arrive in time or the
device reboots before confirmation. Without a guard an activation cannot
be made safe, so devices without systemd cannot be updated at all. A
cron-based guard would have to give the same guarantees as the systemd
guard: fire when the confirmation window elapses, fire after a reboot, and
not depend on the device's wall clock.

**While open.** The guard requires systemd. State-changing operations
refuse a device without systemd (`PLT-002`); adoption and snapshots are
read-only and do not need the guard. The first versions support Debian and
Ubuntu with systemd; other device operating systems are an extension point
for later.

**What would answer it.** First, the platform facts of the adopted
devices: whether any device lacks systemd at all. If some do, a design
recorded in an ADR, proven on a lab device image without systemd by the
same lab scenarios as the systemd guard (a device rebooting during the
confirmation window, the VPN dropping during activation) and by a bench
test with real power loss.

### Network health checks and the executor's no-network rule

This question was raised while the repository was being set up.

**Question.** The specification says the executor never initiates network
connections, but the health checks it runs include `http` and `tcp` checks
against the device's own services, for example a local health endpoint at
`http://127.0.0.1:8080/healthz`. Should the rule become "never connects
to other hosts", allowing connections to loopback addresses only, or
should network health checks run in a separate process, so that the
executor itself never opens a connection?

**Why it matters.** Health checks must run on the device: activation runs
detached from the SSH session (ConnectionIndependent), and the decision to
roll back cannot wait for the operator machine. So the answer decides:

* the executor's dependency rules: whether the health checks may use the
  standard library's network packages, and which executor packages may;
* the architecture tests: what the `go list -deps` assertions and the
  guard script `scripts/check-exec-deps.sh` check, besides the existing
  ban on operator-side packages. A test on imports alone may not be
  enough, because libraries the executor needs for other reasons can
  import `net` (to verify with `go list -deps` on the executor once it
  exists);
* how health check targets are validated: a loopback-only rule needs every
  target checked when the bundle is verified and again when a connection
  is opened, and literal loopback addresses avoid name resolution, which
  can itself contact another host;
* the executor's size, which has a target and a limit enforced in CI,
  since HTTP support adds code.

**While open.** The stricter reading applies: the executor opens no
network connection. The check types that need no network (`systemd`,
`compose`, `command`, `freshness`) can be implemented; `http` and `tcp`
checks in the executor wait for this answer. Check types stay separate in
the health checks package, so either answer adds the network types without
changing the others.

**What would answer it.** A decision by the maintainer, recorded in an
ADR, with the specification's "Executor", "Health checks" and "Dependency
rule" sections updated to match. The decision should also say whether
processes that the executor starts (`systemctl`, `docker`, hooks and
`command` checks) fall under the rule, since a `command` check can already
open a connection from a separate process. A prototype of each option,
measured for executor size and checked with `go list -deps`, would show
what each one costs.

## Resolved questions

None yet.
