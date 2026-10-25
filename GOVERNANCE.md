# irekey governance

This document describes what goes into irekey, who decides, and what
happens to the project if the maintainer becomes unavailable. For the
compatibility contracts see [STABILITY.md](STABILITY.md); for security
reports see [SECURITY.md](SECURITY.md).

## The verb

irekey exists to **inspect, adjust and deliver software to offline devices
with per-customer differences**. That verb breaks down into three:

- **Inspect:** know what each device really runs, and how it differs from
  what it should run.
- **Adjust:** keep each customer's differences as reviewed configuration
  and patches on top of one base, and carry them onto every new base.
- **Deliver:** move a tested release set to devices safely, with a way
  back.

Every feature must serve at least one of them.

## Feature acceptance criteria

### 0. Demand (prerequisite)
Is there a real field case, described concretely (the devices, the
situation, the current friction)? Speculative proposals are not evaluated.

### 1. Verb (veto)
- Sentence test: can the feature be described as "inspect, adjust or
  deliver better, or in more situations"? If it needs another verb
  (monitor, alert, provision, manage a network), it is rejected.
- Separate project test: would it make sense as a standalone tool? If so,
  it is another project.
- Audience test: is the user the developer who maintains the devices? If
  not, it is another product.

### 2. Zero configuration (negotiable with mitigation)
The basic uses keep working with only the binary: the first device is
adopted without writing a configuration file, and nothing has to be
installed on the devices beforehand. Features with risk or cost are off
by default.

### 3. Maintainable by one person (negotiable with mitigation)
Two years from now, can a bug in this feature be fixed in an afternoon,
with only tests, documentation and code, without the original devices?
- Testable in CI and in the simulated device lab, without hardware.
- New dependencies justified in writing (see
  [CONTRIBUTING.md](CONTRIBUTING.md)).
- Platform-specific code isolated.

### 4. Contracts (veto)
No existing contract is broken (see the inventory below and
[STABILITY.md](STABILITY.md)).

### 5. Operational safety (veto, unless mitigated)
If the feature does anything new on a device (writes a file, runs a
command, changes a service, opens a port), it only goes in as optional,
explicit, confirmed and logged, with the default unchanged. Nothing
permanent may run on a device: outside an activation window, no irekey
process, timer or listener exists there. No safety invariant is weakened.

## Contract inventory

[STABILITY.md](STABILITY.md) holds the full rules. Before 1.0 any contract
may change, with a changelog entry; from 1.0 they are promises.

| Contract | Rule |
|---|---|
| CLI commands, flags and `--json` output | additions only; changes require deprecation |
| Exit codes and error codes | a released code never changes meaning |
| Bundle format | every executor of a format reads every bundle of that format |
| Device layout and journal format | newer versions read what earlier versions wrote, and can roll back a device updated by the previous minor version |
| Deployment repository schemas | versioned; newer versions read older files; a migration command is provided |
| Canonical variant encoding | stable; a change is a new encoding version, and old IDs stay resolvable |
| Safety invariants | never weakened |
| Go API | not a contract; everything stays under `internal/` |
| Human-readable output, web interface layout | not contracts |

## Deprecation

1. Version N marks the item as deprecated; it keeps working and emits a
   warning with the alternative.
2. The warning stays for at least two minor versions.
3. File formats get an automatic migration command.
4. Removal only in a major version, documented in the changelog.

## Release discipline

One theme per version. A version never mixes themes.

## Proposal template

    ## Field case
    ## Verb (inspect / adjust / deliver)
    ## Basic usage (unchanged? optional?)
    ## Maintenance (how it is tested without hardware; new dependencies)
    ## Contracts affected
    ## Operational safety (anything new on devices? confirmed, logged?)

The feature proposal form of the issue tracker asks for the same fields.

## Decision making

irekey is maintained by one person,
[@hitoshyamamoto](https://github.com/hitoshyamamoto), who makes the final
decisions. Decisions are made in the open:

- **Proposals are discussed in GitHub issues before implementation**, and
  judged against the criteria above.
- **Changes land only through pull requests.** A branch ruleset on `main`
  enforces it. Pull requests are merged by squash, the only merge method
  allowed, and the history of `main` must stay linear. A pull request is
  merged only when the required status checks `lint`, `test`, `build`,
  `dco`, `codeql`, `licenses` and `guards` pass on a branch that is up to
  date with `main`. The ruleset has no bypass actors, so it applies to the
  maintainer too.
- **No approving review is required.** With a single maintainer, a
  required approval could never be given to the maintainer's own pull
  requests, so the ruleset requires zero approvals. The maintainer reviews
  every pull request from others; the maintainer's own changes are checked
  by CI but reviewed by nobody else. Dependency updates from Dependabot can
  be merged automatically once every required check passes, if the
  optional auto-merge workflow is enabled; it is disabled today.
- **Disagreements** are discussed in the issue or pull request thread. The
  maintainer decides and records the reasoning there, so it stays
  discoverable.
- **Decisions that change the design** get an architecture decision record
  in [docs/adr/](docs/adr/) and an update of the specification, in the
  same pull request.

### Sponsorship and decisions

Sponsorship never buys influence. No goods or services are expected or
promised in return: sponsors get no support, no priority for their
issues or pull requests, and no influence on the roadmap or on reviews;
their proposals pass the same criteria as everyone else's. The only
acknowledgement is a logo or name and a link in the README, if the
sponsor wants it.

## Roles and responsibilities

Today every role below is held by the maintainer. They are listed
separately because they carry different duties, and because separating
them is the first step when a co-maintainer joins.

| Role | Responsibilities | Held by |
|---|---|---|
| Maintainer | Final say on scope and design; triage issues; merge pull requests; keep the documentation and the roadmap current; apply this governance | [@hitoshyamamoto](https://github.com/hitoshyamamoto) |
| Reviewer | Review every pull request from contributors for correctness, safety, contracts and tests; review is requested automatically through [.github/CODEOWNERS](.github/CODEOWNERS) | [@hitoshyamamoto](https://github.com/hitoshyamamoto) |
| Security responder | Handle vulnerability reports and coordinate disclosure as described in [SECURITY.md](SECURITY.md) | [@hitoshyamamoto](https://github.com/hitoshyamamoto) |
| Release manager | Prepare the changelog, sign and push version tags, check the published release and its provenance | [@hitoshyamamoto](https://github.com/hitoshyamamoto) |
| Code of conduct enforcement | Apply [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | [@hitoshyamamoto](https://github.com/hitoshyamamoto) |

## Continuity

**The project's bus factor is 1.** Only the maintainer can merge changes
and publish releases. Anyone who considers irekey for critical work
should weigh that.

What limits the damage:

- **The MIT license permits an immediate fork.** Anyone may fork the
  repository and continue the project without asking permission.
- **Everything needed to build and verify is in the repository**: the
  history, the tests, the CI workflows and, once it is added, the release
  workflow. Release provenance is designed to use keyless signing tied to
  the workflow identity, so no private signing material will be needed to
  build a release or to verify one. Release tags will be signed with the
  maintainer's SSH key; its public half is already in
  [.github/allowed_signers](.github/allowed_signers), so anyone will be
  able to check a tag, and a fork signs its own tags with its own key.
- **The release steps will be documented** in
  [CONTRIBUTING.md](CONTRIBUTING.md) when the release workflow is added;
  today that file states the principles: a curated changelog and signed
  tags.

What is missing: a second person who could merge and release within a
week. Two arrangements would close that gap, and neither is arranged yet:

1. **A co-maintainer** with independent rights to merge and release. This
   is the preferred remedy, and it would raise the bus factor above 1.
2. **Credential escrow:** the account recovery material held by a trusted
   party, together with the rights needed to continue the project.

This section will be updated when either exists.

## Changing this document

Changes to governance follow the same path as any other change: a pull
request, open discussion, and the required checks.
