# irekey — Master Prompt

> Build specification for Claude Code. This document is the single source of truth for the first versions of `irekey`. Read it completely before writing any code.
>
> **Amendments.** The project was renamed from its working name "irekeys" to "irekey" (ADR 0020). Invariants are referred to by descriptive names outside this document, instead of by their IDs (ADR 0021). The repository guards and CI checks were refined when they were implemented (ADR 0022). Each amendment is explained in the ADR named next to it.

---

## 0. How to use this document

### 0.1 Reading order

1. Sections 1–7 define *what* irekey is and the rules it never breaks.
2. Sections 8–19 define *how* it works.
3. Sections 20–22 define the engineering standard of the repository.
4. Section 23 defines the order of work (milestones) and the acceptance criteria of each one.
5. Sections 24–25 list the decisions to record as ADRs and the questions that are still open.
6. Appendix A is the canonical glossary. Appendix B is the error and exit-code reference. Appendix C lists the pinned CI actions. Appendix D is the Definition of Done.

### 0.2 Status labels

Every design point carries one of these labels, explicitly or by section:

- **Decided** — chosen by the maintainer. Do not change it. If you believe it is wrong, stop and explain why before writing code that depends on it.
- **Default** — the recommended design. Implement it unless you find a concrete technical reason not to; if you do, write an ADR proposal and ask.
- **Verify** — a fact about an external tool or library that must be confirmed in its official documentation or source before use (for example with `go doc`, the project README, or a man page). Never assume it.
- **Open** — not decided. Do not implement behavior that depends on it; leave an extension point and keep it in `docs/open-questions.md`.

### 0.3 Working rules for Claude Code

1. **Work milestone by milestone** (Section 23). Do not start a milestone before the previous one meets its acceptance criteria. Each milestone ends with a green CI run and an updated `CHANGELOG.md` `[Unreleased]` section.
2. **Small pull-request-sized steps.** One coherent change per commit series, each with tests. Use Conventional Commits (`feat(adopt): ...`, `fix(exec): ...`). Sign off every commit (`git commit -s`, Developer Certificate of Origin).
3. **Tests first for invariants.** Every invariant in Section 7 has at least one test that fails if the invariant is broken. Write it before or together with the code that relies on it.
4. **Never invent APIs.** Do not invent function names, flags, config keys, systemd directives or library behavior. Check with `go doc <pkg>.<Symbol>`, the module source, or official documentation. When something cannot be checked offline, write `// Verify: <what and where>` next to it and list it in the pull request description.
5. **Dependencies are a decision.** Adding a module requires a one-paragraph justification in the commit message (why, alternatives, license, maintenance status) and must pass the license check (Section 21.8). Prefer the standard library.
6. **English only.** Code, identifiers, comments, commit messages, CLI output, logs, documentation, test names and fixtures are in English. No word from any other natural language appears anywhere in the repository, except the Japanese terms explained in Section 1.2.
7. **No real-world data.** No real customer, site or employer names (names of third-party tools, products and services that irekey uses or learns from are fine), site names, IP addresses, MAC addresses, host keys or credentials in the repository. Use the documentation ranges `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, `2001:db8::/32`, the domain `example.com`, locally administered MAC addresses (`02:00:00:xx:xx:xx`), loopback addresses, and fictional names (`acme`, `globex`, `initech`).
8. **No employer reference.** The project is generic infrastructure tooling. Never name or describe any specific employer, product or industry customer in code, docs or commit messages.
9. **Ask before** (a) changing anything labeled Decided, (b) changing a contract in Section 22, (c) adding a network listener, (d) adding anything that runs permanently on a device, (e) adding a dependency with a non-permissive license.
10. **Create and maintain `CLAUDE.md`** at the repository root during M0: a short summary of these rules, the build/test commands, and pointers to this document's sections. Keep it under 150 lines.
11. **Keep this document in the repository** as `docs/spec/masterprompt.md`. When a decision changes, update the spec and add an ADR in the same pull request.

---

## 1. Identity

### 1.1 One-line description

**irekey manages the same software running on many offline embedded Linux computers, where every customer needs small local differences, and delivers changes over SSH with automatic rollback.**

### 1.2 Name and metaphor (Decided)

`irekey` blends two ideas:

- **irekae** (入換): in Japanese railway operations, *shunting* — moving rolling stock between the tracks of a yard to regroup and form trains. Written 入れ替え, the same reading also means "swap" or "replacement", which is what an update does to a device.
- **key**: nothing is switched without the right key. The device's pinned SSH host key proves the target, and a trusted signing key authorizes the bundle.

Taglines: *No switch without the right key.* · *Every device is a train. irekey is the yard where you inspect, switch and dispatch them.*

| Railway | irekey |
|---|---|
| train | device |
| cars | the components on the device |
| formation (編成, *hensei*) | release set |
| yard tracks | groups and waves |
| inspection (検車, *kensha*) | snapshot and verification |
| shunting (入換, *irekae*) | transfer, activation and atomic switch |
| turntable (転車台, *tenshadai*) | rollback |
| timetable (ダイヤ, *daiya*) | rollout plan |

**Rule (Decided):** the metaphor belongs to the name, the logo and the README introduction only. It never appears in commands, flags, file formats, error codes, package names or the canonical vocabulary.

### 1.3 Project facts (Decided)

| Item | Value |
|---|---|
| Repository | `github.com/hitoshyamamoto/irekey` (public) |
| Go module | `github.com/hitoshyamamoto/irekey` |
| Language | Go (use the Go version of the maintainer's other Go projects: `go 1.27` in `go.mod`; Verify it is the current stable release) |
| License | MIT, `Copyright (c) 2026 Hitoshi Yamamoto`; every source file starts with `// SPDX-License-Identifier: MIT` |
| Binaries | `irekey` (operator machine) and `irekey-exec` (device-side executor) |
| Maintainer | `@hitoshyamamoto` (sole maintainer today) |

---

## 2. Problem and context

### 2.1 The situation

A team maintains a product made of several components (for example two backend services, a frontend, a data-analysis service and an infrastructure repository). The product runs on **more than 200 embedded Linux computers**, one or more per customer site. Each device:

- has its own IP address and MAC address, and is reachable **only through the customer's VPN**, from a developer's laptop, over SSH;
- has **no internet access**;
- runs the same base product with **small per-customer differences**: configuration values, enabled modules, screen layouts, and sometimes small code changes;
- was historically modified **by hand over SSH**, so nobody knows exactly what runs where (drift);
- costs a long, expensive trip to the site if it becomes unreachable after an update.

### 2.2 What irekey changes

From "200 hand-edited copies" to **one base code + explicit, reviewed per-customer adjustments + controlled delivery**:

1. Register every device and capture what it really runs (snapshot), without changing it.
2. Recover the per-customer adjustments from the devices and store them as reviewable data and patches in a private **deployment repository**, never as per-customer branches.
3. Change them with a normal git workflow (branch, merge request, approval, merge).
4. Update N devices at once from reusable, pre-selected groups, with per-device progress, CI-style validation before anything is applied, canary first, and automatic rollback.

### 2.3 Differentiator (Default)

Delivery with rollback already exists in other tools. The differentiating capability is **extracting, maintaining and re-applying each customer's adjustments on top of a moving base**, across several repositories, for devices that are only occasionally reachable. Every design trade-off favors that capability.

### 2.4 Prior art to learn from, not to depend on (Default)

| Tool | What irekey borrows | Why it is not used directly |
|---|---|---|
| Steam (SteamPipe) | separate *transfer* from *activation*; channels; show transfer size before release | consumer distribution platform |
| Mender (standalone mode) | install → verify → commit-or-revert pattern | requires an agent and, for full rollback, bootloader/partition integration |
| quilt / Debian patch series / Yocto layers | ordered patch stacks on a pristine base; layer priority | no fleet, no delivery |
| Google `repo`, `vcstool` | a manifest of several repositories at tested versions (release set) | no per-device variation, no delivery |
| Ansible | push over SSH, no agent | no offline bundle, staging, journal or rollback built in |
| Torizon Lockbox / Uptane | signed offline bundles verified on the device | tied to its platform |
| casync / desync | content-addressed chunked transfer | deferred (Phase "later"); path is known |

### 2.5 Reference device profile (for budgets and tests)

Ubuntu or Debian, x86_64, 2 cores (Celeron-class), 4 GB RAM, ~27 GB storage, systemd, OpenSSH server; some components under Docker Compose, others as systemd services. ARM devices (arm64, armv7) must be supported by the executor build but are not the primary target. Brief downtime (minutes) during activation is acceptable (Decided).

---

## 3. Users, goals and scope

### 3.1 Users (Decided)

- **Primary user: the developer.** Only developers change adjustments and only developers deliver to devices. Every change to the deployment repository goes through a merge request with mandatory approval.
- The CLI is the main product. The web UI is a read-only companion in v1.
- Field technicians without code access are **not** users of v1.

### 3.2 Scope of the first versions (Decided): "my scenario, prepared to grow"

Build exactly what the scenario in Section 2 needs. Where it is already known that other teams will differ, put an **extension point** (a Go interface with one or two implementations), not a second implementation:

| Extension point | v1 implementations | Later |
|---|---|---|
| Transport | SSH (in-process) | USB drive, HTTPS pull |
| Runtime adapter | `systemd`, `compose` (Docker Compose v2) | plain process, Podman |
| Vault provider | `local` (age-encrypted file), `bitwarden` (via the `bw` CLI) | 1Password, KeePassXC, HashiCorp Vault |
| Forge provider | `none` (plain git remote), `gitlab`, `github` | Gitea/Forgejo |
| Device OS | Debian/Ubuntu with systemd | other distributions, Windows |

### 3.3 Goals

| ID | Goal | Status |
|---|---|---|
| G1 | Plug-and-play: a single static binary, no configuration needed for the first device | Decided |
| G2 | **Under 1 minute from download to the first device adopted** (adopted = registered + connected with pinned host key + snapshot taken). Code analysis runs in the background, outside that minute | Decided (goal) / Default (background analysis) |
| G3 | Several repositories (components) per device | Decided |
| G4 | Nothing permanently installed or running on devices: no daemon, no package, no open port | Decided |
| G5 | A dropped VPN never leaves a device broken or half-updated | Decided |
| G6 | Every change is reviewed (merge request with approval) before it can reach a field device | Decided |
| G7 | Validation before delivery: automatic simulation on every merge request; bench test optional and recorded | Decided |
| G8 | Works 100% offline after artifacts are built; forges are optional integrations | Default |

### 3.4 Non-goals for v1 (Default)

VPN management (Decided: out of scope, the maintainer's responsibility); Windows devices; transports other than SSH; delta/chunked transfer; beta channels (the data model has channels, only `stable` is used); deploy actions from the web UI; graph visualization; OS package, kernel and firmware updates; field technicians delivering updates; device holds; HTTPS pull for always-online devices.

---

## 4. Requirements

### 4.1 Functional requirements

| ID | Requirement |
|---|---|
| FR-01 | `irekey init` creates a deployment repository and sets up the vault in one guided flow. |
| FR-02 | `irekey adopt <address>` registers a device from an address and a credential; everything else (OS, architecture, MAC addresses, runtime, component locations) is discovered. |
| FR-03 | Adoption pins the device's SSH host key. Every later connection is refused if the host key differs; state-changing operations are also refused if the MAC address set differs (I1). |
| FR-04 | Adoption takes a read-only snapshot: a manifest of file hashes, without copying code. Data, logs, caches, virtual environments and files marked `secret` are excluded. Adoption never uses sudo; its only permitted write on the device is a temporary directory under the SSH account's home, removed in the same connection. |
| FR-05 | Batch adoption from a CSV file. |
| FR-06 | For each component on each device, find the closest base version (exact when `.git` exists; by similarity otherwise) and extract the delta. |
| FR-07 | Split deltas into hunks, cluster identical hunks across devices, and suggest a destination for each cluster: configuration value, upstream (base), profile patch, customer patch, device patch, or discard. The developer confirms every decision. |
| FR-08 | `irekey edit` materializes "the code of customer X" as a normal working tree; `irekey save` regenerates the patch stack and opens a merge request. The user never has to touch `.patch` files. |
| FR-09 | `irekey rebase` re-applies every patch stack onto a new base and classifies each variant as `clean`, `fuzzy` (applied with offset/fuzz, needs review) or `conflict`. |
| FR-10 | Release sets: tested combinations of component versions delivered together; compatibility validated by the dependency graph. |
| FR-11 | Groups: reusable selections of devices. Waves: ordered rollout stages that reference a group. |
| FR-12 | `irekey plan` shows, per device, what changes and **how many bytes will be transferred**, before anything is applied. |
| FR-13 | Transfer and activation are separate steps that can happen in different connections. |
| FR-14 | Canary first in every multi-device rollout; failure budget; concurrency limit; bandwidth limit. |
| FR-15 | Automatic rollback when health checks fail, when the operator cannot reconnect and confirm, and after a reboot during the confirmation window. |
| FR-16 | Per-device pending queue: work that could not run because a device was unreachable resumes at the next connection. |
| FR-17 | Every connection compares the device with its expected state. Drift is reported and offered for triage; irekey never decides on its own. |
| FR-18 | History of every delivery: who, what, when, whether it passed the bench, result per device. |
| FR-19 | Device logs are collected automatically when a health check fails. |
| FR-20 | `irekey ui` opens a local read-only web interface. |
| FR-21 | `irekey lab up` starts a complete simulated fleet in containers. |
| FR-22 | `irekey doctor` checks the operator machine (git version, vault access, deployment repository state, clock) and explains every problem with a fix. |

### 4.2 Non-functional requirements

| ID | Requirement | Measure |
|---|---|---|
| NFR-01 | Adoption time | ≤ 60 s from download to first adopted device over a link with 100 ms RTT and 1 % loss, in CI, in the lab |
| NFR-02 | Operator platforms | Linux, macOS, Windows; amd64 and arm64 |
| NFR-03 | Device platforms | Linux amd64 (primary), arm64, armv7 |
| NFR-04 | Static binaries | `CGO_ENABLED=0`; no runtime dependency except `git` (≥ 2.38, Verify the minimum needed for the commands used) for patch operations |
| NFR-05 | Device impact | staging runs at idle CPU and I/O priority (`nice`, `ionice` when available); transfers honor a bandwidth limit |
| NFR-06 | Crash safety | power loss at any step ends on the previous or the new release, never in between (I7) |
| NFR-07 | Reproducible builds | anyone can rebuild a release from the tag and obtain the published `SHA256SUMS` |
| NFR-08 | Test coverage | ≥ 80 % statement coverage (same floor as the maintainer's other Go projects) |
| NFR-09 | Scale | 500 devices, 10 components each, 50 customers, in the deployment repository and the graph without noticeable delay (< 2 s for `plan` on a laptop, excluding network) |
| NFR-10 | No telemetry | irekey never contacts any server except the devices, the configured forge, and the configured vault |

---

## 5. Canonical vocabulary (summary)

Use every term exactly as defined, in code, docs, CLI output and logs. The full glossary is Appendix A.

| Term | Meaning |
|---|---|
| **customer** | The organization that owns one or more sites. |
| **site** | A physical location of a customer. |
| **device** | One embedded computer at a site. Identity = pinned host key + MAC addresses. |
| **component** | A git repository whose software runs on devices (e.g. `backend-api`). |
| **base** | A released version of a component, identified by a git tag and its commit. Identical for everyone. |
| **layer** | A configuration file that overrides values. Order: base → profiles → customer → site → device. |
| **profile** | A layer (and optionally a patch stack) shared by devices with something in common (e.g. `iec104`). |
| **patch stack** | Ordered patches applied on top of a base for changes that cannot be expressed as configuration. |
| **variant** | The result of applying a patch stack to a base. Identified by a content hash; identical devices share it. |
| **rendered configuration** | The merged layers for one device and one component. |
| **release set** | A tested combination of components and versions delivered and rolled back as one unit. |
| **deployment repository** | The private git repository holding customers, sites, devices, layers, patches, release sets, groups and rollout plans. Never public. |
| **group** | A reusable selection of devices. Carries no configuration. |
| **wave** | An ordered stage of a rollout that references a group, with its own concurrency, failure budget and soak time. |
| **rollout** | Delivery of a release set to the devices of its waves. |
| **canary** | The first device of a rollout, updated alone before anyone else. |
| **snapshot** | A read-only record of what a device runs (file hashes, facts). |
| **drift** | Difference between a device's expected state and its snapshot. |
| **triage** | The decision recorded for each drift item. |
| **bundle** | The signed package that carries one release set's files for one device. |
| **transfer** | Moving a bundle to a device's staging area. Changes nothing that runs. |
| **activation** | Stopping, switching, starting and checking a staged release on a device. |
| **confirmation** | The operator reconnecting after activation and telling the device to keep the new release. |
| **guard** | The persistent systemd timer/service pair that rolls back an unconfirmed activation. |
| **executor** | `irekey-exec`, the device-side binary that performs state-changing work. Uploaded by irekey; runs only when invoked by irekey or by an armed guard. |
| **bench device** | A device used for hardware tests before field delivery. Labeled `labels.bench: "true"`. |
| **lab** | The containerized simulated fleet started by `irekey lab up`. |

Forbidden synonyms (never use): "fleet repository", "client" (for customer), "equipment", "plant", "agent" (for the executor), "deploy key" (ambiguous), "backend" for a provider/plugin (use "provider"), "tag" for anything but a git tag (use "label" for device labels).

---

## 6. Domain model

### 6.1 Entities and identifiers

| Entity | ID format | Stored in |
|---|---|---|
| customer | slug `[a-z0-9][a-z0-9-]{1,40}` | `customers/<customer>/customer.yaml` |
| site | slug, unique within the customer | `customers/<customer>/sites/<site>/site.yaml` |
| device | slug, globally unique (e.g. `acme-north-01`) | `devices/<device>.yaml` |
| component | slug, globally unique | `components/<component>.yaml` (pointer to its repo) |
| profile | slug | `profiles/<profile>/profile.yaml` |
| release set | `<name>@<YYYY.MM.N>` | `release-sets/<name>/<YYYY.MM.N>.yaml` |
| group | slug | `groups/<group>.yaml` |
| rollout | `r-<YYYYMMDD>-<short random>` | `rollouts/<rollout>.yaml` |
| variant | `sha256:<hex>`; display form `v-<first 12 hex>` | computed, never stored as source |
| update | `u-<ULID>` | device journal and local state |
| snapshot | `s-<ULID>` | `snapshots/<device>/latest.json.zst` |

### 6.2 Relations (the graph)

Edge kinds (Default): `belongs-to` (site → customer, device → site), `uses-profile` (device → profile, ordered), `inherits-from` (layer → layer, derived from the layer order), `applies` (variant → patch, ordered), `based-on` (variant → base), `depends-on` (component version → component version range), `contains` (release set → component base), `selects` (group → device, derived), `stage-of` (wave → rollout), `targets` (wave → group).

The graph is built in memory from the deployment repository on every run (no database). It must answer:

1. **Impact:** "if I change X (a layer key, a patch, a profile, a base version), which devices and variants are affected?" → `irekey graph impact <ref>`
2. **Validity:** "is this release set compatible on these devices?" (dependency ranges satisfied, no cycles, every referenced entity exists, platform requirements met) → `irekey graph check`
3. **Provenance:** "where did this configuration value come from on this device?" → `irekey graph why <device> <component> <key.path>`

The graph is required from the first milestone that writes layers (M3). Visualization is a later feature.

---

## 7. Invariants (never broken)

Each invariant has an ID (I1–I20) and a descriptive name. The IDs are used only inside this specification, to cross-reference its sections. Everywhere else (code comments, test names, public documentation, commit messages and pull requests) an invariant is referred to by its name (ADR 0021). Code comments that explain how code upholds an invariant use the same name, `// Safety (DataSeparation): ...`, and the tests that protect an invariant are named after it, `Test<Name>_<behavior>` (for example `TestDataSeparation_WriterRedactsAcrossChunkBoundaries`). Every invariant has at least one such test.

| ID | Name | Invariant |
|---|---|---|
| **I1 Pinned identity** | `PinnedIdentity` | No operation runs on a device unless it presents its pinned SSH host key; state-changing operations additionally require its MAC address set to match. Every bundle is bound to one device ID and host-key fingerprint, and the executor re-checks the binding. |
| **I2 Signed only** | `SignedOnly` | The executor applies only bundles whose manifest signature verifies against a key in the device's trusted-signers file, and whose every file matches its digest. |
| **I3 Snapshot before change** | `SnapshotBeforeChange` | No transfer or activation runs without a snapshot taken in the same connection. Untriaged drift blocks updates. Confirmation and rollback require only the identity check of I1; drift found at that point is reported, never blocking. In multi-device operations, a device with new drift since its last triage is skipped. |
| **I4 Atomic switch** | `AtomicSwitch` | A device switches release sets with one atomic `rename(2)` of the `current` symlink. No state mixes components from two release sets. |
| **I5 Explicit point of no return** | `ExplicitPointOfNoReturn` | Before the switch, abort leaves the device unchanged. After the switch, any failure triggers rollback. |
| **I6 Connection-independent** | `ConnectionIndependent` | Device-side state changes run detached from the SSH session. A dropped connection never leaves a half-applied device. |
| **I7 Crash-safe** | `CrashSafe` | Every executor step is idempotent and journaled (intent before, outcome after, each followed by `fsync`). After power loss, recovery deterministically ends on the previous release or completes the rollback. |
| **I8 Functional health** | `FunctionalHealth` | Success requires every critical health check to pass, including data freshness for data-producing components. Otherwise, automatic rollback. Checks that already failed before the update are reported and excluded (health is judged as regression against a baseline). |
| **I9 Whole-set rollback** | `WholeSetRollback` | Rollback restores the previous release set as a whole: code, images and managed configuration. Databases are restored only when a migration was declared irreversible. |
| **I10 Data untouched** | `DataUntouched` | Paths declared as `data_paths` by a component are never written, moved or deleted by irekey, and are excluded from snapshots. |
| **I11 Disk budget** | `DiskBudget` | No transfer starts without verified free space and inodes for staging, backup and a safety margin of `max(10 % of the filesystem, 1 GiB)` (Default). |
| **I12 Downtime bound** | `DowntimeBound` | In multi-device operations, devices whose estimated downtime exceeds the product maximum are skipped. Single-device mode requires interactive confirmation. |
| **I13 Failure budget** | `FailureBudget` | A wave halts when its failure budget is exceeded. In-flight activations finish; no new ones start. |
| **I14 Data separation** | `DataSeparation` | No plaintext secret in the deployment repository, in logs, in CLI output or in the local cache. No customer data in the public repository. |
| **I15 Path back** | `PathBack` | An activation is kept only after the operator opens a **new** SSH connection and confirms. The guard rolls back if confirmation does not arrive within the window or if the device reboots before confirmation. The guard is armed before the point of no return. |
| **I16 Nothing permanent running** | `NothingPermanentRunning` | Outside an activation window, no irekey process, timer or listener exists on the device. The only persistent artifacts are the files under `/opt/irekey`, `/var/lib/irekey` and `/etc/opt/irekey` (Section 11.1), plus two documented exceptions: the symlinks that replace legacy component paths (Section 11.1) and the container images loaded into the local Docker image store (retention: images of the current and previous release sets). |
| **I17 One operation per device** | `OneOperationPerDevice` | State-changing operations hold a device lease from the first transfer step until the update reaches a terminal state (`committed`, `rolled-back`, `unchanged`): a lease record in the journal, plus `flock` on `/var/lib/irekey/lock` while an executor process runs. A second operator is refused with `ENG-002` (exit 14), not queued. Read-only operations (snapshot, status, diff) take no lease and report an active lease when they see one. |
| **I18 Humans decide drift** | `HumansDecideDrift` | irekey never overwrites or absorbs drift without an explicit, recorded human decision. |
| **I19 No flag disables a safety check** | `NoFlagDisablesSafetyCheck` | The only overrides are interactive confirmations in single-device mode (overwriting triaged drift, exceeding the downtime bound, downgrading). They are recorded in history. |
| **I20 Approved state only** | `ApprovedStateOnly` | Field devices (not `bench`, not `lab`) receive only release sets and layers from a deployment-repository commit that is clean and reachable from the remote protected branch. |


---

## 8. Architecture

### 8.1 Zones

```
┌──────────────────────── Operator machine (developer laptop) ────────────────────────┐
│  irekey (single static binary)                                                      │
│   ├─ CLI (cobra)  ── same service layer ──  Web UI (embedded, 127.0.0.1 only)       │
│   ├─ deployment repository working copy (git)                                        │
│   ├─ component repositories (git clones, cache)                                      │
│   ├─ local state (SQLite: jobs, progress, pending queue, snapshot & transfer cache)  │
│   ├─ vault provider (local age file | Bitwarden CLI)                                 │
│   └─ embedded irekey-exec binaries (linux/amd64, arm64, armv7)                       │
└───────────────┬───────────────────────────────────────────────┬─────────────────────┘
                │ SSH over customer VPN (out of scope)          │ HTTPS (optional)
                ▼                                               ▼
┌──────────── Device (one per site, ×200+) ────────────┐   ┌──── Forge (optional) ────┐
│ sshd (existing)                                       │   │ GitLab / GitHub          │
│ /opt/irekey/releases/<id>/<component>/  current ->    │   │ merge requests, approval │
│ /var/lib/irekey/ journal, staging, blobs, lock        │   │ CI runs `irekey ci`      │
│ /etc/opt/irekey/ identity, trusted signers            │   └──────────────────────────┘
│ irekey-exec (uploaded, content-addressed, on demand)  │
│ guard unit (exists only during an activation window)  │
│ components under systemd or Docker Compose            │
└───────────────────────────────────────────────────────┘
```

### 8.2 Architectural decisions (summary; each becomes an ADR, Section 24)

1. **Push from the operator machine over SSH; no device agent** (Decided: G4). The laptop is the control plane. The executor is uploaded when needed and verified by digest.
2. **One base per component + layered data + patch stacks; never a branch or fork per customer** (Default).
3. **Release sets are the unit of delivery and rollback** (Default).
4. **The deployment repository is separate and private**; the forge enforces approval (Default).
5. **In-process SSH** with `golang.org/x/crypto/ssh` (Default). Reason: credentials supplied by the vault (including passwords), no dependency on the platform's `ssh` binary on Windows, full control of host-key pinning, multiplexing in one process. `-v` prints an equivalent `ssh` command line so any connection can be reproduced by hand. Read `~/.ssh/config` (`HostName`, `Port`, `User`, `ProxyJump`, `IdentityFile`) with `github.com/kevinburke/ssh_config` (Verify API and license). SSH agent via `golang.org/x/crypto/ssh/agent`; Windows OpenSSH agent named pipe support needs `github.com/Microsoft/go-winio` (Verify).
6. **git:** `github.com/go-git/go-git/v5` only for `init`, `add`, `commit`, `clone`, `fetch`, `push` and read access; **system `git`** for `am`, `apply`, `format-patch`, `rebase`, `rerere`, `range-diff` (Verified earlier: go-git's compatibility table lists `apply`, `am`, `cherry-pick`, `rebase` and `revert` as unsupported; re-Verify against the current go-git `COMPATIBILITY.md`). Adoption must work without system git; patch operations require it and `doctor` checks it.
7. **Local state in SQLite** with the pure-Go driver `modernc.org/sqlite` (Default; Verify current status and license). Only operational state lives there; anything another developer needs lives in the deployment repository.
8. **YAML for human-edited files, validated with JSON Schema** (`github.com/santhosh-tekuri/jsonschema/v6`, Verify). YAML library: `go.yaml.in/yaml/v3`, the maintained successor of the archived `gopkg.in/yaml.v3` (Verify). Unknown keys are errors.
9. **Web UI embedded with `embed`**, server-rendered `html/template` + vendored htmx, no Node toolchain in the build (Default; Section 18).
10. **Signatures in the OpenSSH signature format (SSHSIG)** with namespace `irekey-bundle@v1`, using developers' existing SSH keys (agent or hardware token), verified on the device against an `allowed_signers`-style trust file (Default). Library: `github.com/hiddeco/sshsig` (Apache-2.0, Verify current version) or an in-repo implementation of the published SSHSIG format if the library is unmaintained.

### 8.3 Repository layout (public repository)

```
irekey/
├── cmd/
│   ├── irekey/                  # operator CLI entry point (main only; wiring)
│   └── irekey-exec/             # device executor entry point
├── internal/
│   ├── app/                     # service layer used by CLI and UI; no I/O formatting
│   ├── cli/                     # cobra commands, flags, human and --json output
│   ├── ui/                      # web UI handlers, templates, static assets (embed)
│   ├── model/                   # domain types: Customer, Site, Device, Component, ReleaseSet, ...
│   ├── deployrepo/              # load/validate/write the deployment repository; schema migrations
│   ├── schema/                  # JSON Schemas (embedded) and validation
│   ├── layers/                  # layer merge, provenance tracking, rendering
│   ├── patchstack/              # series files, apply/save/rebase via system git
│   ├── variant/                 # variant identity (canonical hashing), dedup
│   ├── graph/                   # in-memory graph, impact/check/why, cycle detection
│   ├── extract/                 # nearest-base detection, hunk splitting, clustering, suggestions
│   ├── triage/                  # triage records and interactive flow (UI-agnostic core)
│   ├── snapshot/                # manifests, exclusions, drift computation
│   ├── facts/                   # device fact discovery (os-release, arch, MACs, runtimes)
│   ├── discover/                # component discovery heuristics on a device
│   ├── transport/               # Transport interface; ssh/ implementation
│   │   └── ssh/                 # x/crypto/ssh client, host-key pinning, multiplexing, ProxyJump
│   ├── transfer/                # content-addressed sync, resume, bandwidth limit, size estimate
│   ├── bundle/                  # bundle format v0: manifest, signing, verification, limits
│   ├── rollout/                 # plans, waves, canary, concurrency, failure budget, pending queue
│   ├── exec/                    # executor core (state machine, journal, guard, adapters, health)
│   │   ├── journal/
│   │   ├── guard/
│   │   ├── adapter/             # systemd/, compose/
│   │   └── health/
│   ├── execbin/                 # embedded executor binaries (generated, gitignored) + digests
│   ├── vault/                   # Vault interface; local/ (age), bitwarden/
│   ├── forge/                   # Forge interface; none/, gitlab/, github/
│   ├── state/                   # SQLite local state, migrations
│   ├── lab/                     # lab orchestration (compose files embedded; images by digest)
│   ├── errs/                    # error codes registry (generated from codes.yaml), typed errors
│   ├── redact/                  # secret redaction for logs and output
│   └── version/                 # build info
├── lab/                         # sources of lab images and fixtures (not embedded in the CLI)
│   ├── images/device-systemd/   # Debian + systemd + sshd test device
│   ├── images/device-compose/   # same + Docker (DinD), used sparingly
│   ├── images/modbus-sim/       # Modbus TCP simulator (pymodbus)
│   ├── images/iec104-sim/       # IEC 60870-5-104 simulator (c104, GPL-3.0; separate process only)
│   ├── components/              # the 5 fictional component repositories (seed sources)
│   ├── divergence/              # divergence generator
│   └── scenarios/               # scripted fault scenarios
├── schemas/                     # published JSON Schemas (generated copies of internal/schema)
├── docs/
│   ├── spec/masterprompt.md     # this document
│   ├── adr/                     # MADR-format decisions
│   ├── architecture.md
│   ├── threat-model.md
│   ├── assurance-case.md
│   ├── errors.md                # generated from internal/errs/codes.yaml
│   ├── glossary.md              # Appendix A, kept in sync by a test
│   ├── deployment-repository.md # file formats, with examples
│   ├── component-manifest.md
│   ├── lab.md
│   ├── open-questions.md
│   └── schema-versions.txt      # kept current by a test
├── examples/
│   ├── deployment-repo/         # a complete fictional deployment repository
│   │   ├── .github/workflows/irekey.yml
│   │   └── .gitlab-ci.yml
│   └── component/.irekey/       # a sample component manifest
├── scripts/                     # guard scripts used by CI (Section 21.4)
├── testdata/compat/             # compatibility zoo (bundles, journals, repo files per version)
├── .github/                     # workflows, templates, CODEOWNERS, allowed_signers, dependabot
├── CLAUDE.md
├── README.md  CHANGELOG.md  CONTRIBUTING.md  GOVERNANCE.md  SECURITY.md
├── CODE_OF_CONDUCT.md  ROADMAP.md  STABILITY.md  LICENSE
├── .bestpractices.json  funding.json  .well-known/funding-manifest-urls
├── .golangci.yml  .gitignore  go.mod  go.sum
```

### 8.4 Dependency rule

```
cmd/* ─► cli ─► app ─► {deployrepo, layers, patchstack, variant, graph, extract, triage,
                        snapshot, rollout, bundle, transfer, vault, forge, state}
ui ─► app
rollout ─► transfer ─► transport
exec (device side) ─► {journal, guard, adapter, health, bundle(verify only)}
pure packages (no I/O): model, layers, variant, graph, schema, errs, redact
```

- Pure packages import no I/O package and are tested exhaustively with table tests, property tests and fuzzing. `net/netip` and `log/slog` are allowed: neither opens files or connections by itself (ADR 0022).
- Only `cmd/irekey` and `cli` may import `ui` (to start the server); nothing imports `cli`. The UI never calls `transport` directly; it calls `app`.
- `cmd/irekey-exec` must not import any operator-side package (`app`, `cli`, `ui`, `vault`, `forge`, `state`, `deployrepo`). A test enforces this with `go list -deps`. The same test also keeps the executor and every package under `internal/exec` away from `execbin`, `transport`, `transfer` and `rollout`, and allows no module outside the standard library unless it is listed with a justification (ADR 0022).
- The executor binary must stay small: target < 8 MiB stripped for linux/amd64 (Default). A CI check fails above 12 MiB.

---

## 9. Customization model

### 9.1 Why not branches per customer (Default)

With 200 long-lived branches, every base fix becomes 200 merges and divergence only grows. A single base per component, versioned by tags, plus explicit layered data and small patch stacks, keeps "what does customer X run" answerable by a pure function: `desired_state(device) = f(release set, layers, patch stacks)`.

### 9.2 Configuration layers

Order, lowest to highest priority (Default):

1. **base** — the component's defaults: `.irekey/config.defaults.yaml` in the component repository, at the base tag.
2. **profiles** — in the order listed in the device file (`profiles: [iec104, low-bandwidth]`).
3. **customer** — `customers/<customer>/config/<component>.yaml`.
4. **site** — `customers/<customer>/sites/<site>/config/<component>.yaml`.
5. **device** — `devices/<device>.config/<component>.yaml`.

Merge semantics (Default): maps merge key by key, recursively; lists are replaced as a whole; scalars are replaced; an explicit `null` deletes the key. A key that does not exist in the component's config schema (`.irekey/config.schema.json`) is an error. Every merged value carries its provenance (layer file, line) so `graph why` can explain it.

Secrets are never values in layers. A value of the form `secret://<item-reference>` is resolved from the vault at transfer time, written on the device with mode `0600`, and never cached in plaintext on the laptop.

Rendering (Default): the component manifest declares output files and their format (`yaml`, `json`, `env`, or `template` using Go `text/template` with `missingkey=error` and no custom functions beyond a small allowlist). Rendering is deterministic (sorted keys) so the rendered digest is stable.

### 9.3 Patch stacks

For changes that cannot be expressed as configuration.

- Stored as `git format-patch` files plus an ordered `series` file (quilt-style), under `patches/<component>/<scope>/` where scope is `profile/<profile>`, `customer/<customer>` or `device/<device>`.
- Application order: profile stacks (in profile order) → customer stack → device stack.
- Applied with `git am --3way` onto a clean checkout of the base tag in a temporary worktree.
- The total number of changed lines in customer and device patches is reported by `irekey status --debt` and tracked as technical debt (Default).
- A change that spans several components is recorded as one **change set** (same `Change-Id:` trailer in each patch) and is applied, reverted and reported as a unit (Default).

### 9.4 Variants

`variant_id = sha256(canonical({component, base_commit, [patch_digest...], build_recipe_digest}))`

- Canonical encoding: JSON with sorted keys, no insignificant whitespace, UTF-8, numbers as strings. Document it in `docs/architecture.md` and pin it with golden tests; it is part of the compatibility contract.
- Devices with the same variant share one build and one test result. The rendered configuration is *not* part of the variant; it has its own digest.
- **Device desired state** = `{release_set, per component: variant_id + rendered_config_digest}`; its digest is the device's `desired_digest`.

### 9.5 Release sets

```yaml
schema: irekey/release-set/v1
name: product
version: 2026.11.0
channel: stable                 # only "stable" in v1
components:
  backend-api:   { base: v1.8.0 }     # git tag; the commit is resolved and recorded on save
  backend-ingest: { base: v2.3.1 }
  frontend:      { base: v4.0.2 }
  analytics:     { base: v0.9.0 }
  infra:         { base: v1.2.0 }
requires:                       # optional platform requirements
  os: { id: [debian, ubuntu], version_min: { debian: "12", ubuntu: "22.04" } }
max_downtime: 5m
```

- Each component's manifest declares `depends_on` ranges (SemVer) on other components; `graph check` validates every release set against them.
- The infrastructure repository is a component like any other (Default).
- A device runs the subset of components listed in its device file; the release set must contain all of them.
- Once a release set is used by any rollout, its file is immutable (`REPO-004` if edited); create a new version instead.

### 9.6 Groups, waves and rollouts

```yaml
# groups/early-adopters.yaml
schema: irekey/group/v1
select:
  devices: [acme-north-01]
  labels: { ring: early }          # AND within a selector
  customers: [globex]              # OR between selector kinds
exclude:
  devices: [globex-east-02]
```

```yaml
# rollouts/r-20261110-a1b2.yaml
schema: irekey/rollout/v1
release_set: product@2026.11.0
canary: { device: acme-north-01, soak: 30m }   # default: first device of the first wave
waves:
  - group: early-adopters
    concurrency: 3                 # default 3
    failure_budget: 0              # default 0
    soak: 2h
  - group: everyone
    concurrency: 5
    failure_budget: 1
    soak: 0s
bench_required: false              # Decided: bench is optional; result is recorded either way
```

Groups select; they never carry configuration (Default). Groups are evaluated at plan time and the resolved device list is written into the plan.

---

## 10. End-to-end workflows

Every workflow below must be exercised by an automated lab scenario (Section 19.4).

### 10.1 First run: `irekey init` (target: < 20 s of the 60-second budget)

1. Ask: "Do you already use a password manager?" Options: Bitwarden/Vaultwarden (via `bw`), or **create a local encrypted vault now**.
2. Local vault: an age-encrypted file (`filippo.io/age`, scrypt passphrase recipient) at the user config directory. Never implement custom cryptography. Lock the vault again after each operation that unlocked it (Default).
3. Bitwarden: detect `bw`, check `bw status`, unlock with the session key in memory only, `bw lock` when done (Verify current `bw` CLI commands and JSON output).
4. Create or clone the deployment repository (`irekey init --repo <path|url>`), write `irekey.yaml` with `schema: irekey/repo/v1`, commit.
5. Print the next command (`irekey adopt <address> --customer <c> --site <s>`).

When a second developer appears (a second distinct `author` in the deployment repository history), suggest migrating to a shared vault with `irekey vault migrate` (Default).

### 10.2 Adoption: `irekey adopt <address> --customer C --site S [--name N] [--user U] [--port P] [--jump SPEC]`

1. Resolve credentials (vault, SSH agent, `~/.ssh/config`).
2. Connect. Show the host-key fingerprint (SHA256), hostname and MAC addresses; ask the human to confirm (trust on first use). Refuse identities already enrolled for another device (cloned images): `ID-003`.
3. Discover facts with read-only commands run with `LC_ALL=C` (`/etc/os-release`, `uname -m`, `ip -j link` or `/sys/class/net/*/address`, `systemctl --version`, `docker compose version`, free space, inodes, clock). Never parse localized text.
4. Discover components (Default heuristics, in order): Docker Compose projects (`docker compose ls --format json`; if the Docker socket is not accessible to the SSH account, record `compose: inaccessible` and ask the layout question instead; never escalate), systemd units with a `WorkingDirectory=` or `ExecStart=` under configured roots, git working trees under configured roots (default roots: `/opt`, `/srv`, `/home/*/apps`). Unknown layouts trigger one question; the answer is stored in the device file so it is never asked again.
5. Snapshot without uploading anything: `find ... -printf` (findutils) + `sha256sum` (coreutils) over discovered component paths, excluding `data_paths`, logs, caches, `node_modules`, `__pycache__`, `.venv`, `venv`, `*.pyc`, `.git/objects`. If the tools are missing, upload the executor to a temporary directory in the SSH account's home, run `irekey-exec scan` unprivileged, and remove it afterwards. Adoption never uses sudo and never writes outside that temporary directory.
6. Write `devices/<device>.yaml`, regenerate `known_hosts`, commit locally (the developer pushes and opens a merge request; adoption is a repository change like any other).
7. Start background analysis (Section 10.4) and return. Adoption is complete when steps 1–6 succeed.

Batch: `irekey adopt --csv devices.csv` with columns `address,customer,site,name,user,port,jump,host_key_sha256`; confirmations are collected first, then devices are adopted concurrently. A pre-supplied fingerprint (`host_key_sha256` column or `--host-key SHA256:...` flag), obtained out of band, counts as the human confirmation; a mismatch is `ID-001`. This is how the lab and CI adopt without prompts.

### 10.3 Snapshot, status and drift

- `irekey snapshot <device|--group G>` — read-only; stores the latest manifest in `snapshots/<device>/latest.json.zst` and full history in local state. Only `adopt` and `snapshot` write snapshots into the deployment repository; snapshots taken implicitly by other commands go to local state only, so they never leave the repository dirty (REPO-003).
- `irekey status [<device>|--group G|--rollout R]` — connects, reads the device journal, refreshes observed state, prints a matrix: device × component → expected variant, observed variant, drift, last result.
- Drift = files whose digest differs from the expected release (added, modified, removed, mode changed). Every connection, for any command, computes drift first (I3).
- `irekey diff <device> [<component>]` — shows the drift as a unified diff when the file content can be retrieved, otherwise as a digest list.

### 10.4 Extraction and triage (the differentiator)

Background analysis after adoption, and on demand with `irekey extract <device|--all>`:

1. **Nearest base.** With `.git`: `git describe --tags`, merge-base with the component repository, plus uncommitted changes. Without `.git`: compare the device manifest with each base tag's file digests and pick the tag maximizing the Jaccard similarity of `(path, digest)` pairs; report the score and the runner-up.
2. **Delta.** Fetch only the differing files (bounded, with a size limit per file and per device), compute the diff against the nearest base.
3. **Hunks.** Split into hunks; normalize (line endings, trailing whitespace) and hash each hunk.
4. **Cluster.** Group identical hunks across devices. A cluster present on most devices of a component suggests **upstream**; present on all devices of a profile or customer suggests **profile** or **customer patch**; inside a file declared as configuration with a key/value shape suggests **configuration value** (and the layer level that covers exactly the affected devices); unique to one device suggests **device patch**; generated or temporary files suggest **discard**.
5. **Triage.** `irekey triage <device|--cluster ID>` presents each cluster with the suggestion; the developer accepts, changes or postpones. Decisions are written to `triage/<device>/<snapshot>.yaml`, and the corresponding layer values or patches are generated in the deployment repository. Upstream items produce a branch in the component repository (`irekey extract --upstream`) ready for a merge request.
6. The core of triage is UI-agnostic (`internal/triage`), so the web UI can drive it later.

Plug-and-play has a limit here (stated in the docs): classifying hunks and resolving patch conflicts require human judgment. irekey guides, groups and suggests; it never decides alone (I18).

### 10.5 Editing and maintaining adjustments

- `irekey edit <component> --customer C [--site S | --device D | --profile P]` — creates a worktree at `~/.cache/irekey/workspaces/<component>/<scope>`, checks out the base tag, applies the stack(s) up to that scope as commits, enables `rerere`, prints the path. The developer edits with any editor.
- `irekey save` — regenerates the scope's patch stack from the worktree commits with `git format-patch`, rewrites `series`, recomputes affected variants, runs `graph check`, commits on a branch in the deployment repository and, if a forge is configured, opens a merge request with a summary (affected devices, `git range-diff` of the stack).
- `irekey rebase <component> --to <new-tag>` — re-applies every stack onto the new base in temporary worktrees, classifies each variant `clean | fuzzy | conflict`, writes refreshed patches for `clean` and `fuzzy` (fuzzy marked for review), and opens one merge request. `conflict` variants are listed with the command to resolve each one (`irekey edit ...`).

### 10.6 Verification gates

| Gate | What | Where | Mandatory |
|---|---|---|---|
| V1 | Schema validation, cross-references, `graph check` | laptop and deployment-repository CI | always |
| V2 | Every affected patch stack applies to its base | laptop and CI | always |
| V3 | Every affected variant builds (component build recipe) | CI | always |
| V4 | Simulation: deploy to lab devices with simulators, run health checks and component contract tests | deployment-repository CI (`irekey ci`) on every merge request | always (Decided) |
| V5 | Bench device | developer's choice | optional (Decided); result recorded; warning when the change touches `hardware_sensitive_paths` without a bench pass |
| V6 | Canary | first step of every rollout | always |

`irekey ci` is the entry point for the deployment repository's own CI; ship templates in `examples/deployment-repo/` for GitHub Actions and GitLab CI.

### 10.7 Planning: `irekey plan <release-set> (--device D | --group G | --rollout R)`

For each device: current and target release set, per-component variant change, configuration keys changed (with provenance), **bytes to transfer** (from the cached device manifest; exact value recomputed at transfer), estimated downtime, gates passed, blockers (drift, disk, platform, lock). `--json` for approvals. Plans are deterministic for the same inputs.

### 10.8 Delivery

- `irekey transfer <release-set> (--device D | --group G)` — pre-flight, upload executor if missing, content-addressed sync of missing blobs to staging, verify, prepare the release directory (hardlinks for unchanged files), load container images (`docker load`) without touching running containers. Nothing that runs changes.
- `irekey activate (--device D | --group G)` — arms the guard, runs hooks, stops, switches, starts, runs health checks, then the laptop reconnects and confirms (Section 11.4).
- `irekey deploy` = `transfer` + `activate` in one command.
- `irekey rollback <device>` — whole-set rollback to the previous release set.
- `irekey rollout run <rollout>` — executes the next eligible wave (canary first), honoring soak, concurrency, failure budget; idempotent (running it again continues). Also `rollout status`, `rollout pause` (no new activations; in-flight finish), `rollout resume`, `rollout revert [--wave N]`.
- `irekey pending [list|run]` — the per-device queue of work blocked by unreachability; any command that connects to a device first offers to run its pending items.

### 10.9 History

Every delivery appends a summary to `history/<YYYY>/<rollout-or-update>.json` in the deployment repository (who, what, when, gates, bench result, per-device outcome and error codes). Detailed progress, logs and timings stay in local state; collected device logs stay local (they may contain customer data).


---

## 11. Device side

### 11.1 Layout (Default; follows the FHS conventions for add-on software)

```
/opt/irekey/
├── releases/<release-id>/<component>/   # code, images references, rendered config; read-only after preparation
│                                        # release-id = desired digest of the device, display form d-<12 hex>,
│                                        # so a configuration-only change also gets its own release directory
├── current -> releases/<release-id>     # replaced atomically: symlink to tmp name, then rename(2)
├── previous -> releases/<release-id>    # rollback target
└── bin/<digest>/irekey-exec             # content-addressed executor binaries (max 2 kept)
/var/lib/irekey/
├── lock                                 # flock target (I17)
├── journal.jsonl                        # update journal (I7)
├── staging/<update-id>/                 # incoming manifest, blobs, partial transfers
├── blobs/<sha256>                       # content-addressed pool (hardlinked into releases)
├── backups/<update-id>/                 # hook-produced backups; retention: last 2
├── pending-confirmation                 # marker file present only during an activation window
└── logs/<update-id>/                    # collected logs on failure (bounded)
/etc/opt/irekey/
├── identity.json                        # device ID, host-key fingerprint, adoption time
└── trusted-signers                      # allowed_signers-format list of public keys (namespace irekey-bundle@v1)
# identity.json and trusted-signers are written by the first state-changing operation (first transfer),
# from the `signers` list in the deployment repository's irekey.yaml at an approved commit (I20).
# Adoption is read-only and never creates them.
/etc/systemd/system/irekey-guard-<update-id>.{service,timer}   # exist only during an activation window
```

- `releases/` must be on the same filesystem as `staging/` and `blobs/` (hardlinks and atomic rename). If not, refuse with `PLT-003`.
- Retention: current + previous release sets; executor binaries: current + previous; backups: last 2.
- First managed deployment converts the legacy layout: original component paths become symlinks into `/opt/irekey/current/<component>/`, so existing systemd units and compose files keep working. This conversion is rehearsed in the lab and on the bench before any field device (`PLT-006` when not automatically convertible).
- `data_paths` (I10) stay where they are and are never inside `releases/`.

### 11.2 Privilege (Default; Open for a restricted mode)

Writing to `/opt`, `/etc/systemd/system` and stopping services requires root. v1 uses the SSH account's existing `sudo` (like Ansible's `become`): passwordless sudo, or a sudo password supplied by the vault and sent on stdin (`sudo -S -p ''`), never on the command line and never logged. When stdin also carries data (blob stream, secret values), it uses length-prefixed frames (`4-byte big-endian length + 1-byte frame type + payload`; types: sudo password, secret values, tar stream, end), and the sudo password is the first frame, consumed by a thin `sudo -S` reader before the executor starts (Verify the exact handoff in the lab; document it in an ADR). Only `irekey-exec` is run under sudo; every other remote command runs unprivileged. `doctor <device>` reports the sudo mode.

Open (Q3): an optional `irekey bootstrap --restrict-sudo <device>` that installs `/etc/sudoers.d/irekey` allowing only a fixed, root-owned executor path, validated with `visudo -cf` before install. This is a permanent artifact and needs an ADR before implementation.

### 11.3 Executor (`irekey-exec`)

- Static Go binary. Installation: the laptop streams it gzip-compressed and the remote shell writes it with `gzip -dc` into a temporary directory under the SSH account's home; then `sudo <tmp>/irekey-exec install-self --sha256 <digest>` re-checks the SHA-256 as root and copies itself to `/opt/irekey/bin/<digest>/` (root-owned, `0755`); the temporary copy is removed. Afterwards it is executed as `sudo /opt/irekey/bin/<digest>/irekey-exec <subcommand>`, and the laptop verifies the installed digest before every use.
- Never long-running except during an activation; never listens on a port; never initiates network connections.
- Subcommands (Default): `install-self`, `capabilities`, `scan`, `receive`, `prepare`, `activate`, `confirm`, `rollback`, `guard`, `recover`, `status`, `collect-logs`, `gc`.
- `activate` runs **detached** from the SSH session: the laptop runs `sudo irekey-exec activate --detach --update <update-id>`, which itself starts `systemd-run --unit=irekey-activate-<update-id> --collect ...` and returns; the laptop then only polls the journal (I6). Verify the exact `systemd-run` flags in `systemd-run(1)`. A transient unit is acceptable here because the guard, not the transient unit, provides reboot safety.
- Output to the laptop is newline-delimited JSON on stdout; diagnostics on stderr; all messages carry error codes.

### 11.4 Activation state machine

```
staged ─► preflight ─► arm-guard ─► hooks:pre-stop ─► backup ─► stop ─►
SWITCH (point of no return) ─► migrate ─► start ─► health (baseline-aware, with soak) ─►
await-confirmation ─► (new SSH connection) confirm ─► committed ─► disarm-guard ─► gc

any failure before SWITCH  ─► abort ─► restart the services that were stopped ─► disarm-guard ─► unchanged
any failure after SWITCH   ─► rollback ─► (restore DB only if migration irreversible) ─► health ─► rolled-back
no confirmation in window  ─► guard fires ─► rollback
reboot before confirmation ─► guard fires at boot ─► rollback
rollback health fails      ─► needs-intervention (exit 8), logs collected, device left on previous release
```

Terminal per-device states (rollout outcome enum): `committed`, `rolled-back`, `unchanged`, `skipped`, `needs-intervention`, `unknown`. They map to the device-state enum of Section 16.1 as: committed → `new-release`; rolled-back → `previous-release`; unchanged and skipped → `unchanged`; needs-intervention → `needs-intervention`; unknown → `unknown`. Both enums live in `internal/errs` and are used verbatim everywhere.

### 11.5 Journal

`/var/lib/irekey/journal.jsonl`, one JSON object per line:

```json
{"ts":"2026-11-10T18:05:11.123Z","update":"u-01J...","step":"switch","phase":"intent","from":"product@2026.10.0","to":"product@2026.11.0"}
{"ts":"2026-11-10T18:05:11.130Z","update":"u-01J...","step":"switch","phase":"done"}
```

- Every step writes `intent` before acting and `done` or `failed` (with error code) after, each followed by `fsync` of the file and, on creation, of the directory.
- `recover` reads the last incomplete step and deterministically finishes the rollback or the commit (I7). A truncated last line is normal after power loss: it is ignored, recovery proceeds, and `ENG-004` is reported as a warning. A journal that cannot be parsed otherwise is `ENG-006` (exit 8).
- The journal format is versioned (`"v":1` in the first record of each update) and is part of the compatibility contract.

### 11.6 Guard (confirm-or-revert; Decided: persistent timer)

Transient systemd units are never written to disk and do not survive a reboot (verified earlier from systemd maintainers' statements; re-Verify in `systemd.unit(5)`/`systemd-run(1)`). Therefore the guard is a **persistent** unit pair written to `/etc/systemd/system/` and enabled at `arm-guard`, and removed at `disarm-guard`:

```ini
# irekey-guard-<update-id>.timer
[Unit]
Description=irekey confirmation guard for <update-id>

[Timer]
OnActiveSec=<max_downtime + soak + confirmation window>
OnBootSec=60s
AccuracySec=1s
Unit=irekey-guard-<update-id>.service

[Install]
WantedBy=timers.target
```

```ini
# irekey-guard-<update-id>.service
[Unit]
Description=irekey confirmation guard for <update-id>

[Service]
Type=oneshot
ExecStart=/opt/irekey/bin/<digest>/irekey-exec guard --update <update-id>
```

- `OnActiveSec=` is relative to the timer's activation and `OnBootSec=` to boot (both documented in `systemd.timer(5)`). That an enabled timer is re-activated at boot, so the guard runs after a reboot, is an inference: **the lab scenario "reboot during the confirmation window" and a bench test with real power loss are mandatory before 0.1.0** (Section 19.4, scenario S05).
- `arm-guard` records the current boot ID (`/proc/sys/kernel/random/boot_id`). Because a timer activated after its `OnBootSec=` point elapses immediately (`systemd.timer(5)`, Verify), the guard fires once right after arming: in that case (same boot ID, activation unit still active) `guard` does nothing and exits. It rolls back only when (a) the boot ID differs from the recorded one and the update is not confirmed, or (b) the `OnActiveSec=` window elapsed without confirmation. The window is `max_downtime + soak + confirmation window` (Default confirmation window: 15 min).
- Monotonic timers only: the guard must not depend on the device's wall clock (devices may have skewed clocks; `TIME-*` errors).
- `guard` checks the `pending-confirmation` marker and the journal: if the update is not confirmed, it rolls back; if it is confirmed, it disarms itself. It is idempotent.
- Alternative when systemd is absent (Open, not v1): cron `@reboot` plus a sleep-based watchdog.

### 11.7 Runtime adapters

Interface (Default):

```go
type Adapter interface {
    Name() string                                     // "systemd" | "compose"
    Detect(ctx context.Context, f Facts) (bool, error)
    Stop(ctx context.Context, c ComponentRuntime) error
    Start(ctx context.Context, c ComponentRuntime) error
    Status(ctx context.Context, c ComponentRuntime) (RuntimeStatus, error)
    PrepareArtifacts(ctx context.Context, c ComponentRuntime, staged string) error // e.g. docker load; must not affect running workloads
}
```

- `systemd`: `systemctl stop|start|is-active|show -p ...` with `--no-pager`, parsing `systemctl show` key=value output, never human-readable status text.
- `compose`: Docker Compose v2 (`docker compose -p <project> -f <file> ...`), images referenced **by digest** in the rendered compose file; `docker load` during transfer; `docker compose up -d --no-build --pull never` during start (Verify flags for the Compose version detected).
- Unchanged components are not restarted unless their rendered configuration changed.

### 11.8 Health checks

Declared per component in its manifest:

```yaml
health:
  - name: api-http
    type: http                 # http | tcp | systemd | compose | command | freshness
    critical: true
    target: http://127.0.0.1:8080/healthz
    expect: { status: 200 }
    timeout: 5s
    interval: 5s
    retries: 12
  - name: ingest-fresh
    type: freshness            # data-producing components (I8)
    critical: true
    path: /var/opt/backend-ingest/last-sample   # or: command printing an RFC 3339 timestamp
    max_age: 2m
soak: 3m                       # keep checking for this long after first success
```

- Pre-flight runs every check once (baseline). Checks already failing are excluded from success criteria and reported (`HLT-003`).
- Intermittent results during soak count as failure (`HLT-004`).
- On failure: `collect-logs` gathers bounded `journalctl -u <unit> --since <activation>` and `docker compose logs --since`, redacted, max 2 MiB per component.

### 11.9 Hooks (Default)

Optional executables declared in the component manifest, run by the executor with a timeout and a clean environment: `pre_stop`, `backup` (must write into the provided backup directory), `migrate` (runs after SWITCH with the new code; must declare `reversible: true|false`, where `reversible: true` means the previous release's code runs correctly on the migrated schema, so rollback needs no restore; `false` means rollback restores the backup), `post_start`. A failing hook before SWITCH aborts; after SWITCH it triggers rollback. Built-in database handlers (PostgreSQL, InfluxDB) are later work; v1 relies on component hooks.

### 11.10 Disk budget and downtime estimate

- Required space = staged bytes (missing blobs + images) + declared backup estimate + margin (I11). Checked before transfer and again before activation.
- Downtime estimate = sum of (stop + migrate + start + first healthy) from the device's last activations, or the component manifest's `expected_restart` when no history exists. Compared with `max_downtime` of the release set (I12).

---

## 12. Transport

### 12.1 SSH client (Default)

- `golang.org/x/crypto/ssh`, one multiplexed client connection per device per operation, sessions opened per command, keepalives (`keepalive@openssh.com` requests) every 15 s; connection considered lost after 3 missed replies.
- Host-key pinning: `HostKeyCallback` that compares against the device's pinned key in the deployment repository's generated `known_hosts` (`knownhosts` package of x/crypto); **never** `ssh.InsecureIgnoreHostKey()` (CI guard, Section 21.4). Trust-on-first-use only inside `adopt` with explicit confirmation.
- MAC check after login for state-changing operations (I1).
- Every remote command is built from an argument vector and quoted with a single, tested shell-quoting function; never by string concatenation of untrusted values. Remote environment: `LC_ALL=C`.
- Authentication order: agent → identity files → vault-provided key → vault-provided password (keyboard-interactive/password).
- `ProxyJump` support (one or more jumps) from the device file or `~/.ssh/config`.
- Concurrency: global limit (default 3, `--concurrency`), per-wave limit, never more than one operation per device (I17).

### 12.2 Content-addressed transfer (Default)

1. Laptop computes the bundle manifest: every file of the target release with SHA-256, size, mode, owner name (not numeric ID), and image references by digest.
2. Executor `receive --manifest` replies with the digests it already has (blob pool, current and previous releases, loaded image IDs).
3. Laptop streams only missing blobs as a zstd-compressed tar over the SSH session; the executor writes each blob to `staging/<update-id>/blobs/<digest>.partial`, verifies, then renames.
4. Resumable: on reconnect, already-complete blobs are skipped; a partial blob restarts (blobs above 64 MiB are split into 8 MiB parts, Default).
5. Bandwidth limit: token bucket on the laptop side (`--bwlimit 2MiB/s`, per device and global).
6. Size estimate for `plan`: computed from the last cached device manifest; the exact number is printed before streaming starts.
7. No `rsync` dependency (Windows laptops). Container images in v1 are transferred as whole `docker save` archives, deduplicated by image ID. Layer-level and chunked delta transfer (casync/desync) is later work.

---

## 13. Bundle format v0

A bundle is a directory (on the wire: a tar stream) with:

```
manifest.json          # see below
manifest.json.sig      # SSHSIG over manifest.json, namespace irekey-bundle@v1
blobs/<sha256>         # only blobs the device does not have
```

```json
{
  "format": 0,
  "update_id": "u-01J...",
  "created": "2026-11-10T17:40:00Z",
  "device": { "id": "acme-north-01", "host_key_sha256": "SHA256:..." },
  "release_set": "product@2026.11.0",
  "deployment_repo_commit": "9f1c...",
  "components": [
    {
      "name": "backend-api",
      "variant": "sha256:...",
      "rendered_config": "sha256:...",
      "runtime": "compose",
      "files": [ { "path": "app/main.py", "sha256": "...", "size": 1234, "mode": "0644", "owner": "app", "group": "app" } ],
      "images": [ { "ref": "registry.example.com/backend-api@sha256:...", "archive": "sha256:..." } ],
      "health": "...", "hooks": "..."
    }
  ],
  "signer": "SHA256:<key fingerprint>",
  "critical_extensions": []
}
```

Rules:

- The executor refuses unknown `format` values and any unknown entry in `critical_extensions` (`BND-003`); it ignores unknown non-critical fields.
- Path safety: no absolute paths, no `..`, no symlinks pointing outside the component root, no device files, no setuid/setgid bits, normalized UTF-8 (`BND-004`). Fuzz-tested.
- Limits (Default): max 200 000 files, max 20 GiB per bundle, max path length 1024 bytes (`BND-005`).
- Secrets are rendered into files at `prepare` from values sent in a separate, never-persisted session stream, not inside `manifest.json`. Files containing secrets are marked `"secret": true` and carry an HMAC-SHA256 keyed with a random per-update key (sent in the same secret stream) instead of a plain digest, so low-entropy secrets cannot be guessed from the manifest; they are excluded from snapshots and drift.
- Signer rotation travels as the critical extension `trusted_signers` (the complete new list); the executor applies it only if the bundle is signed by a key in the current list.
- From 1.0, every format-1 executor reads every format-1 bundle (Section 22).

---

## 14. Deployment repository format

### 14.1 Layout

```
irekey.yaml                                    # repo_format, defaults, policies, `signers` (public keys allowed to sign bundles)
known_hosts                                    # generated from devices/*.yaml; never edited by hand
components/<component>.yaml                    # repo URL, default branch, build recipe pointer
profiles/<profile>/profile.yaml                # description, layer files per component
profiles/<profile>/config/<component>.yaml
customers/<customer>/customer.yaml
customers/<customer>/config/<component>.yaml
customers/<customer>/sites/<site>/site.yaml
customers/<customer>/sites/<site>/config/<component>.yaml
devices/<device>.yaml
devices/<device>.config/<component>.yaml
patches/<component>/{profile/<p>|customer/<c>|device/<d>}/{series,*.patch}
release-sets/<name>/<YYYY.MM.N>.yaml
groups/<group>.yaml
rollouts/<rollout>.yaml
triage/<device>/<snapshot>.yaml
snapshots/<device>/latest.json.zst
history/<YYYY>/<id>.json
```

### 14.2 Device file

```yaml
schema: irekey/device/v1
id: acme-north-01
customer: acme
site: north
address: 192.0.2.15
port: 22
user: maint
jump: []                                  # optional ProxyJump chain
identity:
  host_key: ssh-ed25519 AAAA...           # pinned
  host_key_sha256: SHA256:...
  macs: ["02:00:00:00:00:15"]
platform: { os_id: debian, os_version: "12", arch: amd64, systemd: "252", compose: "2.24.0" }
labels: { ring: early, bench: "false" }
profiles: [iec104]
components:
  backend-api:   { runtime: compose, path: /opt/backend-api, project: backend-api }
  frontend:      { runtime: systemd, path: /opt/frontend, unit: frontend.service }
credentials: { ref: "secret://bitwarden/acme-north-01" }   # reference only
adopted: { at: "2026-11-01T12:00:00Z", by: "dev@example.com", tool: "irekey 0.1.0" }
```

### 14.3 Component manifest (in each component repository: `.irekey/component.yaml`)

```yaml
schema: irekey/component/v1
name: backend-api
build:
  kind: container               # container | files
  dockerfile: Dockerfile        # for container
  # files: { command: ["make", "dist"], output: dist/ }   # for files
runtime: { default: compose, compose_file: deploy/compose.yaml, service: api }
paths:
  exclude: [logs/, tmp/, "**/__pycache__/"]
  data: [/var/opt/backend-api/]  # data_paths (I10)
config:
  schema: .irekey/config.schema.json
  defaults: .irekey/config.defaults.yaml
  outputs:
    - { path: config/settings.yaml, format: yaml }
    - { path: .env, format: env, mode: "0600" }
depends_on: { backend-ingest: ">=2.3.0 <3.0.0" }
hardware_sensitive_paths: ["drivers/**", "serial/**"]
expected_restart: 40s
health: [ ... ]                  # Section 11.8
hooks: { migrate: { command: [bin/migrate], reversible: false } }
```

### 14.4 Versioning and migration (Default)

- Every file carries `schema: irekey/<kind>/v<N>`. The repository root `irekey.yaml` carries `repo_format: <N>`.
- A newer irekey reads older formats and offers `irekey repo migrate` (one commit, reviewable). An older irekey refuses newer formats (`REPO-002`) with the version needed.
- `docs/schema-versions.txt` lists every schema and version and is kept current by a test.

---

## 15. Command-line interface

### 15.1 Conventions (Default)

- `cobra` (`github.com/spf13/cobra`, Verify version) for commands; flags in kebab-case.
- Human-readable output by default; `--json` emits stable machine-readable output; long operations emit newline-delimited JSON events. Human text may change; `--json` is a contract.
- `-v` prints resolution (device → address, port, user, jumps), chosen providers and an equivalent `ssh` command line.
- Every failure message states: what failed, the error code, the state the device was left in, and the next command to run.
- Commands are idempotent: running the same command again continues where it stopped.
- Output never prints secrets (`internal/redact` is applied to every writer; tested).
- Colors only on a TTY and never as the only carrier of meaning (state words or symbols too); `NO_COLOR` honored.
- Interactive prompts only on a TTY; with `--yes` they are answered "yes" only where I19 allows; otherwise the command fails with `USR-002`.

### 15.2 Commands (v1)

| Command | Purpose |
|---|---|
| `irekey init [--repo PATH\|URL]` | create/clone the deployment repository, set up the vault |
| `irekey doctor [<device>]` | check the operator machine, or a device's readiness |
| `irekey adopt <address> --customer C --site S [--name N] [--user U] [--port P] [--jump SPEC]` / `--csv FILE` | adopt devices |
| `irekey reidentify <device>` | replace a device identity after hardware change (confirmation + reason) |
| `irekey status [<device>\|--group G\|--rollout R] [--debt]` | version matrix, drift, last results |
| `irekey snapshot <device>\|--group G` | read-only snapshot |
| `irekey diff <device> [<component>]` | show drift |
| `irekey extract <device>\|--all [--upstream]` | nearest base, hunks, clusters, suggestions |
| `irekey triage <device>\|--cluster ID` | record decisions, generate layers/patches |
| `irekey edit <component> --customer C [--site S\|--device D\|--profile P]` | open a workspace |
| `irekey save` | regenerate patches, commit, open a merge request |
| `irekey rebase <component> --to <tag>` | move every stack to a new base |
| `irekey graph check\|impact <ref>\|why <device> <component> <key>` | graph queries |
| `irekey release new <name> [--from <version>]` | create the next release-set version |
| `irekey verify <release-set> [--group G]` | run gates V1–V3 locally |
| `irekey ci` | run V1–V4 inside the deployment repository's CI |
| `irekey plan <release-set> (--device D\|--group G\|--rollout R)` | the plan, with bytes to transfer |
| `irekey transfer` / `activate` / `deploy` / `rollback <device>` | delivery |
| `irekey rollout new\|run\|status\|pause\|resume\|revert` | fleet rollouts |
| `irekey pending list\|run` | pending queue |
| `irekey vault init\|unlock\|lock\|migrate\|status` | vault management |
| `irekey ui [--port N]` | local read-only web UI |
| `irekey lab up\|down\|status\|scenario <name>` | simulated fleet |
| `irekey repo migrate` | migrate deployment repository formats |
| `irekey version` | version, commit, supported formats, providers, embedded executor digests |

### 15.3 Exit codes (contract from 1.0)

| Code | Meaning | Device state |
|---|---|---|
| 0 | success | as requested |
| 1 | usage or input error | unchanged |
| 2 | device unreachable | unchanged |
| 3 | identity mismatch | unchanged |
| 4 | verification failed (signature, format, schema, gate) | unchanged |
| 5 | pre-flight blocked (disk, drift, downtime, platform) | unchanged |
| 6 | aborted before the switch | unchanged |
| 7 | failed after the switch; rolled back successfully | previous release set |
| 8 | failed after the switch; rollback failed, or state ambiguous | needs-intervention |
| 9 | multi-device operation finished with mixed results | see the JSON report |
| 10 | halted: failure budget exceeded | per device; see the report |
| 11 | authentication failed | unchanged |
| 12 | outcome unknown: contact lost after device-side work started | unknown until reconciled |
| 13 | environment error (laptop, deployment repository, artifact sources) | unchanged |
| 14 | device busy (lock held) | unchanged |
| 15 | internal error (crash report written) | see the report |
| 130 | interrupted by the operator; device-side work already started continues | unchanged or unknown |

### 15.4 Progress display

One line per device, updated in place on a TTY: `queued`, `preflight`, `transferring 42% (12.3/29.1 MiB, 1.8 MiB/s)`, `staged`, `arming`, `stopping`, `backup`, `migrating`, `switching`, `health 3/5`, `soak 1m20s`, `confirming`, then a terminal state (Section 11.4). A summary line closes the operation. Without a TTY: one plain line per state change. With `--json`: events `{"device":..,"state":..,"progress":..,"code":..}`.

---

## 16. Error model

### 16.1 Principles

1. **Stable codes** of the form `<CATEGORY>-<NNN>` (e.g. `NET-003`), registered in `internal/errs/codes.yaml`, generated into Go constants and `docs/errors.md` with `go generate`. A released code never changes meaning.
2. **Classify where the error is detected**, from structured signals (exit statuses, `errno`, systemd unit properties, SQLSTATE, HTTP status), never from localized text. Remote commands run with `LC_ALL=C`.
3. Every error states the **device state**: `unchanged`, `previous-release`, `new-release`, `unknown`, `needs-intervention`.
4. **Fail safe on the unknown:** an unclassified error aborts before the switch and rolls back after it (`BUG-003`).
5. **Unknown is not success:** if the state cannot be proven, report `unknown` and block further changes to that device until `irekey status <device>` reconciles it.
6. **Retry only what is transient and idempotent:** connection setup, blob upload, read-only commands; exponential backoff with jitter, max 3 attempts (Default). Never retry a step after its `intent` without consulting the journal.
7. Every code has: condition, identification signal, handling, exit code, resulting device state, and at least one test that produces it.
8. **Phase-dependent codes.** A few codes can occur before or after SWITCH (marked "by phase" in Appendix B). Their exit code is set by the phase: 6 before SWITCH (device `unchanged`), 7 after SWITCH with a successful rollback (`previous-release`), 8 when the rollback fails (`needs-intervention`).

### 16.2 Categories

| Prefix | Category | Covers |
|---|---|---|
| USR | operator input | unknown device/release/group, declined confirmation, interruption, empty selection |
| LOC | operator machine | missing git, full disk, incomplete cache, interrupted previous run |
| REPO | deployment repository | invalid schema, newer schema, uncommitted changes, release set changed after use, not on protected branch (I20) |
| ART | artifacts and builds | build failure, digest mismatch, wrong architecture, unsatisfiable dependency |
| VER | verification | invalid layer, unknown key, patch does not apply, gate failure |
| NET | network and transport | unreachable, closed port, connection lost before/after trigger, stalled transfer |
| ID | identity and authentication | host-key mismatch, MAC mismatch, duplicate identity, authentication failure, missing credential |
| PRV | privilege | sudo unavailable, wrong sudo password, mandatory access control denial |
| PLT | device platform | OS/runtime version, systemd missing, staging on another filesystem, `noexec` mount, missing user, legacy layout not convertible |
| STO | storage | free space, inodes, disk full during operation, read-only filesystem, I/O error |
| RES | resources | low memory, OOM kill, thermal throttling |
| BND | bundle integrity | signature, digest, format, unsafe entries, limits |
| DB | data hooks | backup failed, migration failed/irreversible, restore failed |
| SVC | services | unit/service does not stop or start, port in use, invalid unit |
| HLT | health | failed check, stale data, check already failing, intermittent, regression during soak |
| ENG | executor | crash, hang, lock held, damaged journal, contradictory state |
| RCV | recovery | missing previous release, previous release also unhealthy, ambiguous state |
| RO | rollouts | wave conditions unmet, failure budget exceeded, invalid plan |
| TIME | time | skewed device clock, laptop clock moving backwards |
| BUG | internal | panic, violated invariant, unclassified error |

Seed the registry with the codes needed by the implemented paths in each milestone; an earlier design draft identified about 118 codes across these 20 categories, so expect the registry to grow to that order of magnitude by 0.1.0.


---

## 17. Security

### 17.1 Assets

Customer devices and the processes they control; the path back to each device (network, sshd, credentials); device credentials for 200+ sites; the integrity of what reaches devices; the deployment repository (customer data); the integrity of the irekey release binaries.

### 17.2 Threat model (write `docs/threat-model.md` in M0; update with every security-relevant change)

| Source | What it can do |
|---|---|
| Network path / VPN | intercept, drop or alter traffic; impersonate a device (spoofed address) |
| Compromised operator laptop | holds VPN access, SSH keys and vault session; can push arbitrary code |
| Careless or malicious insider | unreviewed changes; direct edits on devices over SSH |
| Tampered bundle | in transit or in the laptop cache |
| Malicious or broken device | returns hostile output to the laptop (huge output, escape sequences, malformed JSON) |
| Compromised dependency or CI | modified irekey release |
| Physical access to a device | out of scope beyond detecting drift |

### 17.3 Controls (Default)

- **Identity:** host-key pinning per device; MAC check; device-bound bundles; duplicate-identity detection at adoption; human confirmation on first use.
- **Credentials:** personal SSH keys in an agent or hardware token preferred; vault for passwords and keys otherwise; vault locked after use; secrets only in memory; redaction everywhere.
- **Least privilege:** only `irekey-exec` runs under sudo; restricted-sudo mode is Open (Q3).
- **Signing:** bundles signed with SSHSIG by the developer's key; devices trust only keys in `trusted-signers`, written by the first transfer from the `signers` list of the deployment repository at an approved commit (I20), and changed afterwards only by a bundle signed by a currently trusted key (key rotation). Residual risk: a laptop compromised *before the first transfer* to a device can plant its own key on that device; documented in the assurance case.
- **Approval:** I20 — field devices receive only state from the protected branch of the deployment repository; the forge enforces merge-request approval; `doctor` warns when branch protection cannot be confirmed through the forge API.
- **Hostile device output:** every read from a device is size-bounded and parsed strictly; terminal output is sanitized (no control characters except `\n` and `\t`).
- **UI:** binds only to `127.0.0.1`/`::1`, random port by default, random per-session token in the URL, strict Content-Security-Policy, no external assets, no state-changing endpoints in v1.
- **Supply chain of irekey itself:** Section 21.6 (pinned actions, provenance, SBOM, reproducible builds, signed tags).
- **No telemetry** (NFR-10).
- **Reference:** IEC TR 62443-2-3 (patch management in industrial automation and control systems) informs the approval and history flow; cite it only as a reference, without claiming compliance.

### 17.4 Security documents

- `SECURITY.md` in the same structure as the maintainer's other projects: private reporting via GitHub private vulnerability reporting; response times (acknowledge within 5 business days, assessment within 14 days); supported versions (latest release only); scope; **"Security model: what you can and cannot expect"**; how reports are handled (private advisory fork, regression test, CVE when applicable, credit).
- `docs/assurance-case.md`: assets, threat model, trust boundaries, secure design principles applied (fail-safe defaults, least privilege, complete mediation, economy of mechanism, defense in depth, psychological acceptability), common weaknesses countered (CWE-20, CWE-22, CWE-78, CWE-295, CWE-312, CWE-400, CWE-532, CWE-778) with evidence (test names, CI jobs), residual risks.

---

## 18. Web UI (`irekey ui`)

### 18.1 Principles (Default)

- **Read-only in v1** (Decided by scope: deploy actions from the UI come later).
- **No logic of its own:** handlers call `internal/app`, the same service layer as the CLI. JSON returned by the UI's API uses the same types as `--json`.
- Embedded in the binary with `embed`; server-rendered `html/template` + htmx (vendored file with its license, pinned version, Subresource Integrity not needed when served locally but keep the file digest in a test). No Node toolchain, no bundler, no CDN.
- Organization inspired by Spotify's layout (Decided), with irekey's **own visual identity**: no Spotify colors, logo, typography or assets.

### 18.2 Information architecture

| Spotify concept | irekey view |
|---|---|
| left sidebar: Home, Search, Library | **Home**, **Search**, **Fleet** (customers → sites → devices) |
| "Now playing" bar, fixed at the bottom | **Rollout in progress** bar, visible on every page: release set, wave, progress, failures |
| play queue | **Queue**: pending items per device (FR-16) |
| recently played | **Recent deliveries** (history) |
| artist page | **Customer page**: sites, devices, profiles, debt (patch lines), last results |
| album with track list | **Release set** as an "album": component list with versions and the devices running it |
| track detail | **Device page**: identity, platform, components (expected vs observed), drift, timeline |

Also: **Diff viewer** (drift and patch stacks, side by side), **Graph pages** as tables (impact, why) until visualization exists.

### 18.3 Visual and accessibility rules

- Dark theme by default, light theme available, both defined as CSS custom properties (design tokens) in one file.
- Status always shown with color **and** icon **and** word.
- Keyboard navigable; visible focus; WCAG 2.2 AA contrast; responsive down to 360 px width.
- Live updates of the rollout bar through server-sent events from the same event stream as `--json`.

---

## 19. Lab (`irekey lab up`)

### 19.1 Purpose

Simulate the whole workflow with containers only, in CI and on a laptop, so every failure that happens by chance in the field can be provoked on purpose. Hardware-specific problems (serial ports, drivers, real power loss) remain the bench's job.

### 19.2 Roles

| Real role | Lab container | Notes |
|---|---|---|
| customer devices | `device-systemd` (Debian + systemd as PID 1 + sshd), scaled N times | needs the container setup systemd requires (cgroup, tmpfs on `/run`); Verify for Docker and Podman |
| devices with Docker | `device-compose` (Docker-in-Docker) | heavier; use few |
| git server | Gitea (default, light) | GitLab CE as an optional profile (`--profile gitlab`): it needs several GB of RAM |
| vault | Vaultwarden | compatible with Bitwarden clients |
| field instruments | Modbus TCP simulator (pymodbus) | data source for freshness checks |
| telecontrol | IEC 60870-5-104 simulator (Python `c104`, built on lib60870-C, **GPL-3.0**) | separate container and process only; never linked or embedded in irekey |
| operator laptops | two `irekey` containers | concurrency between two developers |
| bad networks | Toxiproxy and/or `tc netem` | latency, loss, bandwidth, cuts |

### 19.3 Distribution

- Lab image sources live in `lab/images/`. A workflow (`lab-images.yml`) builds them multi-arch, publishes to `ghcr.io/hitoshyamamoto/irekey-lab-*`, and attests provenance.
- The `irekey` binary embeds only the compose file and scenario definitions, referencing images **by digest** pinned at release time. `irekey lab up` requires Docker (or Podman, Verify compatibility) on the operator machine.
- The IEC 104 simulator image carries its own license notice; the README states that the lab is optional and separate from the irekey binary.

### 19.4 Fixtures and scenarios

- **Five fictional components** (Decided), small (a few hundred lines each) with real dependencies between them: `backend-api` (HTTP service), `backend-ingest` (reads the Modbus simulator, writes samples, freshness check), `frontend` (static site served by a small server), `analytics` (batch job under a systemd timer), `infra` (compose files and unit templates). Each has `.irekey/component.yaml`, a config schema, defaults, health checks and several tagged versions, including one deliberately broken version (fails health) and one with an irreversible migration.
- **Divergence generator** (`lab/divergence`): seeds realistic drift — changed values in config files, the same change repeated across a group (to test clustering), unique changes, missing `.git` on some devices, stray files, permission changes. Deterministic given a seed.
- **Scenarios** (each is an automated test; ⚠ marks those mandatory before 0.1.0):

| ID | Scenario | Expected |
|---|---|---|
| S01 ⚠ | Adopt 200 devices with 100 ms RTT, 1 % loss | first adoption ≤ 60 s from a clean machine, including download and `init`; every further adoption ≤ 60 s |
| S02 | Divergence seeded on all devices | extraction finds the nearest base; clusters match the generator's ground truth ≥ 95 % |
| S03 ⚠ | VPN drops during transfer | resume on reconnect; nothing running changed |
| S04 ⚠ | VPN drops during activation, before and after SWITCH | before: unchanged; after: guard rolls back unless confirmed |
| S05 ⚠ | Device reboots during the confirmation window | guard rolls back at boot; journal consistent |
| S06 ⚠ | Executor killed (`SIGKILL`) at every journal step | `recover` ends on previous or new release, never in between |
| S07 | Device unreachable for the whole rollout | pending queue holds it; resumes later |
| S08 ⚠ | Two operators target the same device | second gets `ENG-002`, exit 14 |
| S09 | Manual edit on a device between snapshot and activation | activation blocked by new drift (I3) |
| S10 ⚠ | Broken release fails health on canary | canary rolls back; rollout halts; failure budget respected |
| S11 | Disk full during staging | `STO-*`, unchanged, staging cleaned |
| S12 ⚠ | Host key changed (device replaced or spoofed) | refused with `ID-001`, exit 3 |
| S13 | Irreversible migration then health failure | rollback restores backup; reported |
| S14 | Rebase of all stacks to a new base | classification matches the generator's ground truth |
| S15 | Release set incompatible with a device's component versions | `graph check` fails before any connection |

---

## 20. Testing strategy

| Layer | Tooling | Notes |
|---|---|---|
| Unit | `go test`, table tests | pure packages to ≥ 90 % coverage |
| Property-based | `testing/quick` or `pgregory.net/rapid` (Verify; justify if added) | layer merge (associativity of overrides, null deletion), variant hashing (order sensitivity), graph (no false cycles) |
| Fuzzing | native Go fuzzing (`testing.F`) | bundle manifest, tar entry validation, journal parser, YAML loaders, patch/series parser, device output parsers, shell quoting round-trip |
| Golden files | `testdata/*.golden`, `-update` flag | `--json` outputs, human output of `plan` and `status`, rendered configs, canonical variant encoding |
| Integration | real `sshd` and systemd in containers (lab images), started by the test via the Docker CLI | build constraint `integration` |
| Scenario / e2e | lab scenarios S01–S15 | build constraint `e2e`; nightly and on release branches; mandatory subset on every PR touching `exec`, `transfer`, `transport` or `rollout` |
| Crash consistency | fault injection hooks compiled only with build constraint `faultinject` | kill/power-loss at every journal step (S06) |
| Compatibility zoo | `testdata/compat/<version>/` | bundles, journals and repository files from every released version must be read by the current code |
| Architecture tests | `go list -deps` assertions | dependency rule (Section 8.4), executor size limit |

Rules: tests never need hardware or internet (except the lab image pull in e2e); bug fixes come with a regression test; invariant tests are named after the invariant, `Test<Name>_*` (Section 7); race detector always on in CI; coverage floor 80 % over the product code, `cmd/` and `internal/` (CI-enforced, same awk check as the maintainer's other Go projects; the guard program in `scripts/` is tested but not counted, ADR 0022).

---

## 21. Repository engineering standard

This section mirrors the maintainer's existing projects (`modtop` for Go, `soapbar` for the broader governance and OpenSSF practices). When in doubt, open those repositories and copy their structure, wording style and workflow patterns, adapting names.

### 21.1 Go standards

- `gofmt`; `golangci-lint` v2 with `.golangci.yml` `version: "2"`, `linters.default: standard`, plus `errorlint`, `gosec`, `bodyclose`, `contextcheck`, `noctx`, `errname`, `exhaustive`, `gocritic`, `revive` (Verify each name exists in the pinned golangci-lint version); `gosec` excluded only in `_test.go`; every exclusion has a comment explaining why.
- Errors wrap with `%w`; typed errors carry codes; no `panic` outside `main` and impossible states (which become `BUG-002`).
- `context.Context` first parameter for anything doing I/O; every network or process call has a timeout.
- Logging with `log/slog`, structured, redacted, levels; default human output is not logs.
- No `unsafe`, no cgo (`CGO_ENABLED=0`).
- Effective Go and Go Code Review Comments.

### 21.2 Community and governance files

| File | Content (follow the maintainer's existing style) |
|---|---|
| `README.md` | badges (CI, release, Go version, license, OpenSSF Best Practices, OpenSSF Scorecard); one-line description; railway introduction (the only place for the metaphor); 60-second quick start (`curl` download, `sha256sum --check`, `irekey init`, `irekey adopt`); `irekey lab up` demo; installation; provenance verification (`gh attestation verify`); what it is not; documentation links |
| `CONTRIBUTING.md` | dev setup, the exact CI commands, golden-file update, coding style, DCO, rules (tests required, English only, dependency justification, Conventional Commits, SHA-pinned actions, changelog), releasing steps, reproducing a release, security pointer |
| `GOVERNANCE.md` | **the verb**: irekey exists to *inspect, adjust and deliver* software to offline devices with per-customer differences; feature acceptance criteria (0 demand with a real field case; 1 verb test, veto; 2 zero-configuration default, negotiable; 3 maintainable by one person, negotiable; 4 contracts, veto; 5 operational safety, veto unless mitigated: anything new on devices must be optional, explicit, confirmed and logged); contract inventory; deprecation policy; "one theme per version"; proposal template; decision making; roles table; continuity (bus factor 1, mitigations) |
| `SECURITY.md` | Section 17.4 |
| `CODE_OF_CONDUCT.md` | Contributor Covenant 2.1 (copy from the maintainer's other projects) |
| `ROADMAP.md` | Now / Next / Later / Not planned, 12-month horizon, intent not promises |
| `STABILITY.md` | Section 22 |
| `CHANGELOG.md` | Keep a Changelog 1.1.0; SemVer; sections Added, Changed, Deprecated, Removed, Fixed, Security; `[Unreleased]` always present; an "Ideas on record" section at the end |
| `LICENSE` | MIT |
| `THIRD_PARTY_NOTICES` | generated at release from `go-licenses` (Section 21.8) and shipped with binaries |
| `.github/CODEOWNERS` | `* @hitoshyamamoto` |
| `.github/allowed_signers` | the maintainer's SSH signing key (copy from `modtop`) |
| `.github/ISSUE_TEMPLATE/` | `bug_report.yml` (version, OS, device platform, command, what happened, `--json` excerpt, security warning), `feature_request.yml` (real field case, verb, contracts affected, operational safety), `config.yml` |
| `.github/PULL_REQUEST_TEMPLATE.md` | What / How it was tested / Checklist (vet+race tests, regression test, invariants touched, changelog, docs, no real-world data) |
| `.bestpractices.json` | OpenSSF Best Practices criteria answers (same format as `modtop`); target "passing" at 0.1.0, then "silver" |
| `funding.json` + `.well-known/funding-manifest-urls` | same structure as `soapbar` |
| `.github/FUNDING.yml` | `github: hitoshyamamoto` |
| `docs/adr/` | MADR template; `0000-template.md`; numbered ADRs |

### 21.3 Branch and merge policy

- `main` protected by a ruleset: pull requests only, required status checks (lint, test, build, dco, codeql, licenses, guards), branch up to date, no bypass actors, linear history, squash merge with the PR title as the commit message (Conventional Commits).
- Signed tags (SSH, `gpg.format=ssh`, verified with `git tag -v` against `.github/allowed_signers`).
- Dependabot PRs may be auto-merged after required checks (optional, same pattern as `soapbar`, only when a GitHub App token is configured).

### 21.4 Guard scripts (`scripts/`, run in the CI guards job, ADR 0022)

| Script | Fails when |
|---|---|
| `check-hostkey.sh` | `InsecureIgnoreHostKey` or a host-key callback returning nil unconditionally appears outside tests |
| `check-shell.sh` | remote commands are assembled with `fmt.Sprintf`/`+` into a shell string outside `internal/transport/ssh/quote.go` |
| `check-realdata.sh` | an IPv4/IPv6 address outside the documentation and loopback ranges or a globally administered MAC address appears anywhere in the repository; or a domain other than `example.com`, `example.net`, `example.org`, the reserved `.example`, `.test` and `.invalid` names and an allowlist of tool, module and hosting domains (`github.com`, `golang.org`, `go.dev`, `ghcr.io`, ...), or any intranet name (`.local`, `.lan`, `.corp`, `.internal`, ...), appears in `examples/`, `testdata/`, `lab/`, `schemas/`, docs, root Markdown files or GitHub templates (ADR 0022) |
| `check-language.sh` | non-ASCII letters appear outside an allowlist (`docs/spec/masterprompt.md` and `README.md` for the Japanese terms of Section 1.2, test fixtures for UTF-8 handling) |
| `check-exec-deps.sh` | `cmd/irekey-exec` depends on an operator-side package, or the executor exceeds the size limit |
| `check-spdx.sh` | a `.go` file lacks `// SPDX-License-Identifier: MIT` on its first line, or a `.sh` file lacks `# SPDX-License-Identifier: MIT` on its second line |
| `check-gomod.sh` | a module that `go.mod` requires or uses as a replacement is not listed in `scripts/guard/allowed-modules.txt` with an allowed license, or a `replace` points into `lab/` (the go.mod guard of Section 21.8, ADR 0022) |
| `check-errors.sh` | an error code is used but not registered, or registered but undocumented/untested |

### 21.5 CI workflows (`.github/workflows/`)

Global rules (copy from `modtop`/`soapbar`): `permissions: contents: read` at the top and minimal per-job additions with a comment for each; every action pinned to a full commit SHA with a version comment; `actions/checkout` with `persist-credentials: false`; `timeout-minutes` on every job and on apt steps; `concurrency` groups (cancel in progress for PRs only); apt with `-o Acquire::Retries=3 -o Acquire::http::Timeout=30`; no long-lived secrets in release paths (workflow token and keyless signing only).

| Workflow | Trigger | Jobs |
|---|---|---|
| `ci.yml` | push to main, PR | **lint** (golangci-lint, `actionlint`), **guards** (guard self-tests and every `scripts/check-*.sh`, including the executor dependency and size check), **test** (`go vet`, `go test -race -coverpkg=./cmd/...,./internal/... -coverprofile`, coverage floor 80 % over the product code, short fuzz 30 s per target, `govulncheck` pinned), **dco** (Signed-off-by on every PR commit), **build** (cross-compile executors then CLI for all targets, `-trimpath -ldflags "-s -w -X ...version=ci"`, clean-machine smoke test: `irekey version`, `irekey doctor` with `HOME` in a temp dir), **integration** (sshd/systemd containers), **e2e-smoke** (lab scenarios S01-small, S04, S05, S06 on PRs touching device-side code; full set nightly), **licenses** (`go-licenses` check) |
| `codeql.yml` | push, PR, weekly | `github/codeql-action` init/analyze, `languages: go`, `build-mode: autobuild` or manual (Verify for Go), `queries: security-extended` |
| `scorecard.yml` | push to main, weekly, branch protection changes | `ossf/scorecard-action` with `publish_results: true`, SARIF upload |
| `fuzz.yml` | weekly, manual | long fuzzing (5 min per target), crash artifacts uploaded on failure |
| `nightly.yml` | daily | full lab scenario suite S01–S15, compatibility zoo |
| `release.yml` | push of `v*` tags | tests; release notes from the CHANGELOG section (fail if missing); reproducible build; `SHA256SUMS`; SBOM; provenance attestation; GitHub release with assets and `.sigstore.json` bundle |
| `lab-images.yml` | release, manual | build and push lab images multi-arch, attest, smoke test each image |
| `zizmor.yml` (optional) | PR touching workflows | static analysis of GitHub Actions workflows (Verify tool and action) |

Dependabot (`.github/dependabot.yml`): `gomod`, `github-actions` and `docker` (for `lab/images/*`), weekly on Monday, grouped, `open-pull-requests-limit: 5`, labels.

### 21.6 Release process (Decided pattern from `modtop`: curated changelog + signed tags)

1. In `CHANGELOG.md`, rename `[Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`; merge.
2. `git tag -s vX.Y.Z -m "irekey X.Y.Z" && git push origin vX.Y.Z`.
3. `release.yml`:
   - runs the test suite;
   - extracts the changelog section into `$RUNNER_TEMP/notes.md` (outside the working tree, so the build is clean and reproducible);
   - builds `irekey-exec` for `linux/{amd64,arm64,arm/v7}`, records their digests, embeds them, then builds `irekey` for `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, `windows/{amd64,arm64}` with `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X github.com/hitoshyamamoto/irekey/internal/version.Version=${TAG}"`;
   - writes `SHA256SUMS`;
   - generates a CycloneDX SBOM with `cyclonedx-gomod` (Verify module path and flags) per binary;
   - attests build provenance with `actions/attest-build-provenance` for binaries and SBOMs;
   - publishes with `gh release create --verify-tag --notes-file`, attaching binaries, `SHA256SUMS`, SBOMs, `THIRD_PARTY_NOTICES` and the provenance bundle renamed `irekey-X.Y.Z-provenance.sigstore.json` (the suffix the Scorecard Signed-Releases check recognizes).
4. Reproducibility instructions in `CONTRIBUTING.md`, verified by a CI job that rebuilds a released tag and compares checksums (nightly).
5. Pre-1.0 versions: `0.MINOR.PATCH`; `0.x` may break contracts, with a changelog entry under "Changed" or "Removed".

Not chosen: release-please (it creates tags with a bot identity, which conflicts with maintainer-signed tags). Packages for Homebrew, Scoop/winget, `.deb`/`.rpm` (nfpm) are later work (ROADMAP "Later").

### 21.7 OpenSSF

- **Scorecard** workflow from M0; target: no "Low" findings attributable to configuration (token permissions, pinned dependencies, branch protection, signed releases, SAST, fuzzing, dependency update tool, security policy, license, CII badge).
- **Best Practices badge**: register the project; keep `.bestpractices.json` answers in the repository (same format as `modtop`); "passing" is an acceptance criterion of 0.1.0.
- **Assurance case** (`docs/assurance-case.md`) as described in Section 17.4.

### 21.8 License compliance

- MIT for irekey code. Allowed dependency licenses (Default): MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC, MPL-2.0 (file-level copyleft; requires an ADR note when added). Anything else requires an ADR.
- CI: `go install github.com/google/go-licenses/v2@<pinned>` then `go-licenses check ./... --allowed_licenses=...` (Verify flag names in the pinned version), and `go-licenses report` to generate `THIRD_PARTY_NOTICES` at release.
- GPL and AGPL software (the IEC 104 simulator, GPL-3.0; Vaultwarden, AGPL-3.0) exists only inside lab container images, runs as a separate process, and is never a Go dependency (a guard checks `go.mod`; ADR 0017).

---

## 22. Compatibility contract (`STABILITY.md`)

Before 1.0 anything may change, with a changelog entry. From 1.0 these are promises:

| Contract | Rule |
|---|---|
| CLI commands, flags and `--json` output | additions only; changes require deprecation (warning for at least two minor versions, removal only in a major version) |
| Exit codes (Section 15.3) and error codes (Section 16) | a released code never changes meaning |
| Bundle format 1 | every format-1 executor reads every format-1 bundle; extensions follow the critical-extension rule |
| Device layout and journal format | any newer version reads what any version since 1.0 wrote, and can roll back a device updated by the previous minor version |
| Deployment repository schemas | versioned by `schema`; newer reads older; migration command provided |
| Canonical variant encoding | stable; a change is a new encoding version, and old IDs remain resolvable |
| Safety invariants (Section 7) | never weakened |
| Go API | **not** a contract; everything stays under `internal/` |
| Human-readable output, UI layout | not contracts |

Enforced in CI by golden files for `--json`, the compatibility zoo, and `docs/schema-versions.txt`.


---

## 23. Milestones

Each milestone ends with: green CI, updated `CHANGELOG.md`, updated docs, ADRs for decisions taken, and a short report in the final pull request listing every `// Verify:` left open.

### M0 — Repository foundation

Scope: module, directory skeleton (Section 8.3), `CLAUDE.md`, all community and governance files (Section 21.2), ADRs 0001–0019 (Section 24) as "Accepted" or "Proposed" per their labels, `.golangci.yml`, guard scripts, every workflow of Section 21.5 (e2e jobs as placeholders that pass with a skip message), Dependabot, `internal/errs` registry and generator, `internal/version`, `internal/redact`, `irekey version`, `irekey doctor` (local checks only), `docs/glossary.md` generated from Appendix A with a sync test.

Acceptance:
- `go vet`, `go test -race`, lint and all guards pass; coverage floor enforced.
- `release.yml` dry-run on a `v0.0.0-rc.1` tag in a fork or with `workflow_dispatch` produces all assets, SBOMs and a provenance bundle; `gh attestation verify` passes; a second build reproduces `SHA256SUMS`.
- Scorecard and CodeQL run and upload results.

### M1 — Lab foundation

Scope: `lab/images/*`, the five fictional components with tags, the divergence generator, `irekey lab up|down|status|scenario`, Gitea + Vaultwarden + simulators, netem/Toxiproxy wiring, `lab-images.yml`.

Acceptance: `irekey lab up --devices 20` on a clean Linux machine with Docker reaches "ready" in under 5 minutes (Default target); every device accepts SSH; divergence is reproducible by seed; images are published with provenance.

### M2 — Census (read-only)

Scope: `init` (deployment repository + local vault + Bitwarden provider), transport/ssh with pinning and MAC check, facts, component discovery, snapshot, `adopt` (single and CSV), `status`, `diff`, drift, local state, `known_hosts` generation, deployment-repository loader with schemas v1 for device/customer/site/component, `repo migrate` skeleton.

Acceptance: S01 passes (first adoption ≤ 60 s including `init`, each further adoption ≤ 60 s, with netem 100 ms/1 %); S12 passes; no state-changing remote command exists yet (a test asserts the M2 remote command allowlist: read-only commands plus the temporary unprivileged `irekey-exec scan` of FR-04).

### M3 — Customization model, extraction and triage

Scope: layers (merge, provenance, schema validation, rendering), patch stacks (series, apply via system git), variants (canonical hashing, dedup), graph (`check`, `impact`, `why`), `extract`, `triage`, release sets and groups schemas.

Acceptance: S02 and S15 pass; property tests for merge semantics; golden tests for canonical encoding; `graph why` explains every key of a rendered config in the lab.

### M4 — Edit, save, rebase, verify, CI gates

Scope: workspaces, `edit`, `save`, `rebase` with classification, forge providers (`none`, `gitlab`, `github`), `verify` (V1–V3), `irekey ci` (V1–V4 against the lab), deployment-repository CI templates, I20 enforcement.

Acceptance: S14 passes; a merge request in the lab's optional GitLab profile is opened by `save` with a correct summary; `irekey ci` runs in the example deployment repository's GitHub Actions template.

### M5 — Safe single-device delivery

Scope: bundle v0 (manifest, SSHSIG signing and verification, path safety, limits), executor (all subcommands), journal, guard, lock, adapters (`systemd`, `compose`), health checks, hooks, disk budget, downtime estimate, content-addressed transfer with resume and bandwidth limit, `plan`, `transfer`, `activate`, `deploy`, `rollback`, log collection, legacy layout conversion.

Acceptance: S03, S04, S05, S06, S08, S09, S10 (single device), S11, S13 pass; fuzzing of manifest/tar/journal runs in CI; the executor binary is under the size limit; a documented bench procedure exists for real power loss (to be executed by the maintainer).

### M6 — Fleet rollouts

Scope: rollouts with canary, waves, concurrency, failure budget, soak, pause/resume/revert, pending queue, progress display, history, `status --rollout`.

Acceptance: S07 and S10 (fleet) pass; a 200-device lab rollout completes with deterministic reports; `--json` event stream covered by golden tests.

### M7 — Web UI (read-only)

Scope: Section 18.

Acceptance: every view renders from lab data; keyboard navigation and contrast checks pass (automated with an HTML/accessibility linter run in CI, Verify tool choice); no network request leaves `127.0.0.1`; CSP verified by a test.

### M8 — Hardening and 0.1.0

Scope: threat model and assurance case complete, OpenSSF Best Practices "passing", compatibility zoo seeded with 0.1.0 artifacts, reproducibility nightly job, documentation review, README 60-second quick start verified on a clean Linux, macOS and Windows machine.

Acceptance: all ⚠ scenarios pass; zero open `BUG-*` issues; release `v0.1.0` published with signed tag, provenance, SBOMs and notices.

---

## 24. ADRs to write in M0 (MADR format)

| # | Title | Status |
|---|---|---|
| 0001 | Record architecture decisions | Accepted |
| 0002 | Go as implementation language | Accepted (Decided) |
| 0003 | MIT license | Accepted (Decided) |
| 0004 | Laptop-driven push over SSH; no device agent; nothing permanent running | Accepted (Decided: G4) |
| 0005 | One base per component + layers + patch stacks; no per-customer branches | Proposed (Default) |
| 0006 | Release sets as unit of delivery and whole-set rollback | Proposed |
| 0007 | Separate private deployment repository; approval enforced by the forge | Proposed |
| 0008 | go-git for simple operations, system git for patch operations | Proposed (go-git limits verified earlier) |
| 0009 | In-process SSH with x/crypto/ssh | Proposed |
| 0010 | Persistent guard timer for confirm-or-revert | Accepted (Decided: persistent timer) |
| 0011 | Privilege through the account's sudo; restricted sudo deferred | Proposed |
| 0012 | Vault providers: local age file and Bitwarden CLI | Proposed |
| 0013 | SQLite (pure Go) for local operational state | Proposed |
| 0014 | YAML + JSON Schema for repository files | Proposed |
| 0015 | Embedded server-rendered UI without a Node toolchain | Proposed |
| 0016 | SSHSIG signatures for bundles | Proposed |
| 0017 | Containerized lab; GPL simulators isolated in images | Proposed |
| 0018 | Release process: curated changelog and maintainer-signed tags | Accepted |
| 0019 | Stable error-code registry | Proposed |
| 0020 | Project name irekey | Accepted (maintainer decision; renames the project throughout and amends Section 1.2) |
| 0021 | Descriptive invariant names outside the specification | Accepted (maintainer decision; amends Sections 7 and 20) |
| 0022 | Repository guards and CI checks as implemented | Accepted (amends Sections 8.4, 20, 21.4 and 21.5) |

---

## 25. Assumptions and open questions

### 25.1 Assumptions

- **A1** Customer variation is mostly data. Early extraction (M3) must confirm it; if much code diverges, extension points in the components come before more tooling.
- **A2** Components can be described by a manifest with common paths, runtime and health checks (precondition for plug-and-play).
- **A3** Data written by components can live outside the release directories (`data_paths`).
- **A4** Developers' machines have `git` installed (required only for patch operations).
- **A5** Devices run systemd and an OpenSSH server; most have GNU `find` (findutils) and `sha256sum` (coreutils).

### 25.2 Open questions (keep in `docs/open-questions.md`; do not implement behavior that depends on them)

| ID | Question | Affects |
|---|---|---|
| Q1 | Can a laptop reach several sites at once (VPN concentrator), or one customer VPN at a time? | default concurrency, rollout UX |
| Q2 | Must all drift be triaged before any update (current default, I3), or may updates go first with site adjustments later? | I3 policy |
| Q3 | Restricted sudo bootstrap (`/etc/sudoers.d/irekey`)? It is a permanent artifact | privilege model, G4 |
| Q4 | CLI message localization beyond English | `internal/cli` |
| Q5 | Is a hardware watchdog available on the reference device model? | recovery from hangs |
| Q6 | Distribution of OS versions across devices and end-of-support policy | `requires`, test matrix |
| Q7 | Per-person device accounts vs one maintenance account with per-person keys | audit trail |
| Q8 | Can the frontend load screen layouts at runtime, or does every site need its own build? | variants vs layers for the frontend |
| Q9 | Second signature level (a CI release key attesting variant builds, in addition to the developer's key) | bundle format 1 |
| Q10 | Confirm the pure-Go SQLite driver choice | 0013 |
| Q11 | Does any existing tool already extract and manage per-customer patches across a fleet? (search terms: "fleet configuration management embedded", "per-customer patch management", "edge device GitOps offline") | positioning |
| Q12 | cron-based guard fallback for devices without systemd | 11.6 |
| Q13 | May the executor's health checks connect to the device's own services on loopback (http and tcp checks), given that the executor never initiates network connections? | 11.3, 11.8, executor dependency rule |

---

## Appendix A — Glossary

Terms are alphabetical. "Not:" lists confusions to avoid.

- **activation** — the device-side sequence from `arm-guard` to `await-confirmation` for one staged update. Not: transfer.
- **adapter (runtime adapter)** — the code that stops, starts and inspects a component under a given runtime (`systemd`, `compose`).
- **adopt / adoption** — registering a device: connect, pin host key, discover facts and components, take the first snapshot, write the device file. Not: install (nothing is installed).
- **base** — a released version of a component (git tag + commit). Not: variant.
- **bench device** — a field-identical device used for optional hardware tests (`labels.bench: "true"`).
- **bundle** — the signed manifest plus missing blobs for one update of one device. Not: release set.
- **canary** — the first device of a rollout, updated alone and soaked before others.
- **change set** — patches in several components sharing a `Change-Id:`, handled as a unit.
- **channel** — a release stream (`stable` only in v1).
- **component** — a git repository whose software runs on devices. Not: service (a component may run several services).
- **component manifest** — `.irekey/component.yaml` in a component repository.
- **confirmation** — the operator's new SSH connection telling the device to keep an activation. Not: approval.
- **customer** — the organization owning sites. Not: user.
- **deployment repository** — the private git repository with customers, sites, devices, layers, patches, release sets, groups, rollouts, triage records, snapshots and history. Not: component repository; never called "fleet repository".
- **desired digest** — digest of a device's desired state (release set + variants + rendered config digests).
- **device** — one embedded computer at a site, identified by its pinned host key and MAC addresses.
- **drift** — difference between expected and observed device state.
- **executor** — `irekey-exec`, the uploaded device-side binary. Not: agent or daemon.
- **extraction** — finding the nearest base and the delta of a device's component, split into hunks.
- **failure budget** — maximum failed devices in a wave before it halts.
- **forge** — a git hosting service with merge requests (GitLab, GitHub).
- **gate** — a verification step that a change must pass before it reaches field devices: schema and graph checks, patch application, builds, simulation in the lab, the optional bench test, and the canary.
- **graph** — the in-memory relation model used for impact, validity and provenance queries.
- **group** — a reusable device selection without configuration. Not: wave.
- **guard** — the persistent timer/service pair that rolls back an unconfirmed activation.
- **health check** — a declared functional check; critical checks decide success.
- **hook** — a component-provided executable run at a defined step (`pre_stop`, `backup`, `migrate`, `post_start`).
- **hunk** — a contiguous changed region of a file in a delta.
- **journal** — the device's fsync'ed JSON-lines log of update steps.
- **lab** — the containerized simulated fleet.
- **label** — a key/value attribute of a device used by groups. Not: git tag.
- **layer** — a configuration file at one level (base, profile, customer, site, device).
- **patch stack** — ordered `git format-patch` files plus a `series` file, applied on a base for changes that cannot be configuration. Scopes: profile, customer, device.
- **pending queue** — per-device work blocked by unreachability, resumed at the next connection.
- **plan** — the deterministic per-device description of what an operation will do, including bytes to transfer.
- **point of no return** — the SWITCH step; after it, failures roll back.
- **profile** — a shared layer and optional patch stack for devices with a common trait.
- **provider** — an implementation of an extension point (vault, forge, transport). Not: backend.
- **release set** — a tested combination of component bases delivered and rolled back together. Not: release (of irekey itself).
- **rendered configuration** — the merged layers for one device and component, written into the component's declared files.
- **rollout** — delivery of a release set through ordered waves.
- **site** — a physical location of a customer.
- **snapshot** — a read-only record of a device's files (digests) and facts.
- **soak** — time a release must stay healthy before a step is considered successful.
- **staging** — the device area where transferred blobs and the next release are prepared.
- **switch** — the atomic rename of `current`.
- **transfer** — moving missing blobs to a device's staging area; changes nothing that runs.
- **triage** — recording a human decision for each drift item or cluster.
- **trusted signers** — the device's list of public keys allowed to sign bundles.
- **variant** — a base plus an ordered patch stack, identified by a content hash.
- **wave** — an ordered rollout stage referencing a group, with concurrency, failure budget and soak.

---

## Appendix B — Seed error codes

Seed `internal/errs/codes.yaml` with at least these (condition → exit code → device state). Add codes as paths are implemented.

| Code | Condition | Exit | Device state |
|---|---|---|---|
| USR-001 | unknown device, component, release set, group or rollout | 1 | unchanged |
| USR-002 | confirmation required but no TTY | 1 | unchanged |
| USR-003 | confirmation declined | 1 | unchanged |
| USR-004 | selection resolved to zero devices | 1 | unchanged |
| LOC-001 | system git missing or too old for patch operations | 13 | unchanged |
| LOC-002 | operator machine disk full | 13 | unchanged |
| LOC-003 | vault locked or unavailable | 13 | unchanged |
| REPO-001 | file fails its JSON Schema | 4 | unchanged |
| REPO-002 | schema newer than this irekey | 13 | unchanged |
| REPO-003 | uncommitted changes in the deployment repository | 5 | unchanged |
| REPO-004 | release set modified after use | 4 | unchanged |
| REPO-005 | commit not reachable from the remote protected branch (I20) | 5 | unchanged |
| VER-001 | configuration key not in the component schema | 4 | unchanged |
| VER-002 | patch stack does not apply | 4 | unchanged |
| VER-003 | `graph check` failed | 4 | unchanged |
| VER-004 | gate failed | 4 | unchanged |
| ART-001 | variant build failed | 4 | unchanged |
| ART-002 | artifact architecture does not match the device | 4 | unchanged |
| NET-001 | device unreachable (TCP connect failed or timed out) | 2 | unchanged |
| NET-002 | SSH port closed | 2 | unchanged |
| NET-003 | connection lost before device-side work started | 2 | unchanged |
| NET-004 | connection lost after device-side work started | 12 | unknown |
| NET-005 | transfer stalled | 2 | unchanged |
| ID-001 | host key differs from the pinned key | 3 | unchanged |
| ID-002 | MAC address set differs | 3 | unchanged |
| ID-003 | identity already enrolled for another device | 3 | unchanged |
| ID-004 | authentication failed | 11 | unchanged |
| ID-005 | credential missing in the vault | 11 | unchanged |
| ID-006 | bundle bound to another device | 4 | unchanged |
| PRV-001 | sudo not available for the account | 5 | unchanged |
| PRV-002 | sudo password rejected | 11 | unchanged |
| PRV-003 | mandatory access control denied an action | by phase | by phase |
| PLT-001 | platform requirement unmet | 5 | unchanged |
| PLT-002 | systemd missing or too old | 5 | unchanged |
| PLT-003 | staging not on the same filesystem as releases | 5 | unchanged |
| PLT-004 | executor path mounted `noexec` | 5 | unchanged |
| PLT-005 | required user or group missing | 5 | unchanged |
| PLT-006 | legacy layout not automatically convertible | 5 | unchanged |
| STO-001 | insufficient free space | 5 | unchanged |
| STO-002 | insufficient inodes | 5 | unchanged |
| STO-003 | disk full during operation | by phase | by phase |
| STO-004 | read-only filesystem | 5 | unchanged |
| RES-001 | insufficient memory for the operation | 5 | unchanged |
| BND-001 | manifest signature invalid or signer not trusted | 4 | unchanged |
| BND-002 | blob digest mismatch | 4 | unchanged |
| BND-003 | unknown format or critical extension | 4 | unchanged |
| BND-004 | unsafe entry (path traversal, device file, setuid, outside symlink) | 4 | unchanged |
| BND-005 | bundle limits exceeded | 4 | unchanged |
| DB-001 | backup hook failed (before SWITCH) | 6 | unchanged |
| DB-002 | migration failed (after SWITCH; rolled back) | 7 | previous-release |
| DB-003 | restore after irreversible migration failed | 8 | needs-intervention |
| SVC-001 | service did not stop (before SWITCH; stopped services restarted) | 6 | unchanged |
| SVC-002 | service did not start | 7 | previous-release |
| SVC-003 | port already in use | 7 | previous-release |
| HLT-001 | critical health check failed | 7 | previous-release |
| HLT-002 | data freshness exceeded | 7 | previous-release |
| HLT-003 | check already failing before the update (informational) | — | — |
| HLT-004 | intermittent result during soak | 7 | previous-release |
| ENG-001 | executor crashed | 12 | unknown |
| ENG-002 | device lock held by another operation | 14 | unchanged |
| ENG-003 | executor hung (step timeout) | 12 | unknown |
| ENG-004 | truncated last journal line after power loss; recovered (warning) | — | as recovered |
| ENG-006 | journal damaged beyond the last line | 8 | needs-intervention |
| ENG-005 | contradictory state between journal and filesystem | 8 | needs-intervention |
| RCV-001 | previous release missing | 8 | needs-intervention |
| RCV-002 | previous release also unhealthy | 8 | needs-intervention |
| RCV-003 | state ambiguous after recovery | 8 | needs-intervention |
| RO-001 | wave conditions not met (soak, previous wave) | 5 | unchanged |
| RO-002 | failure budget exceeded | 10 | per device |
| RO-003 | invalid rollout plan | 4 | unchanged |
| TIME-001 | device clock skew beyond tolerance (warning; blocks only time-dependent checks) | — | — |
| BUG-001 | panic in irekey | 15 | unknown |
| BUG-002 | invariant violated | 15 | unknown |
| BUG-003 | unclassified error (fail safe) | by phase | by phase |

---

## Appendix C — Pinned CI actions (starting point)

These pins are taken from the maintainer's repositories as of October 2026. Before use, re-check each SHA against its tag (`gh api repos/<owner>/<repo>/git/ref/tags/<tag>`); Dependabot will keep them current afterwards.

| Action | Version | SHA |
|---|---|---|
| actions/checkout | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| actions/setup-go | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| golangci/golangci-lint-action | v9.3.0 (golangci-lint v2.14.0) | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` |
| actions/attest-build-provenance | v4.2.2 | `4d101475d8b20a2381f78447822ac1eab6504dd8` |
| actions/upload-artifact | v7.0.1 | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` |
| actions/download-artifact | v8.0.1 | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` |
| github/codeql-action (init, analyze, upload-sarif) | v4.38.2 | `2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2` |
| ossf/scorecard-action | v2.4.4 | `2d1146689b8cda280b9bc96326124645441f03bc` |
| docker/setup-buildx-action | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| docker/login-action | v4.6.0 | `dbcb813823bdd20940b903addbd779551569679f` |
| docker/metadata-action | v6.2.0 | `dc802804100637a589fabce1cb79ff13a1411302` |
| docker/build-push-action | v7.4.0 | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` |
| actions/create-github-app-token (optional) | v3.2.0 | `bcd2ba49218906704ab6c1aa796996da409d3eb1` |

`govulncheck`: `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` (re-check for a newer version).

---

## Appendix D — Definition of Done (every pull request)

- [ ] Tests added or updated; bug fixes include a regression test; invariant tests untouched or strengthened.
- [ ] `go vet ./...`, `go test -race ./...`, `golangci-lint run ./...` and `scripts/*.sh` pass locally.
- [ ] Coverage did not drop below 80 % total.
- [ ] New error paths have registered, documented and tested codes.
- [ ] `--json` changes updated golden files and are listed in the changelog.
- [ ] Docs updated (`docs/`, README, spec) when behavior changed; ADR added for any decision.
- [ ] No real-world data; English only; SPDX header present.
- [ ] New dependency justified (purpose, alternatives, license, maintenance).
- [ ] `// Verify:` notes listed in the PR description.
- [ ] `CHANGELOG.md` `[Unreleased]` updated for user-visible changes.
- [ ] Commits signed off; PR title in Conventional Commits form.
