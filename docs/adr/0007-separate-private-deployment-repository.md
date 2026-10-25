---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Keep customer data in a separate private deployment repository, with approval enforced by the forge

## Context and Problem Statement

irekey needs a home for everything that describes the devices and what
they should run: customers, sites, devices with their addresses, pinned
host keys and MAC addresses, layers, patch stacks, release sets, groups,
rollouts, triage records, snapshots and delivery history. This data names
customers and their sites and decides what reaches field devices, so it
must stay private, while irekey's own code is public. The specification
decides that every change is approved in a merge request before it can
reach a field device.

Where does this data live, and how is approval enforced?

## Decision Drivers

* No customer data in the public repository and no plaintext secret in
  the data (DataSeparation).
* Approval is enforced by the forge's branch protection, not only by a
  check on the operator machine.
* One commit describes the desired state of every device.
* Several developers share the same data and history.
* Plain git works without a forge, and offline after a fetch.

## Considered Options

* Separate private deployment repository
* Data in the public code repository
* A database
* Configuration in each component repository

## Decision Outcome

Chosen option: "Separate private deployment repository", as the
recommended default of the specification, because it keeps customer data
out of public code, gives every change the review workflow the team
already uses, and lets one commit describe what every device should run.

* The deployment repository is a private git repository, separate from
  irekey's public repository and from the component repositories, and it
  is never public. `irekey init` creates or clones it. Every file carries
  a schema version.
* Secrets are never stored in it: a value of the form
  `secret://<item-reference>` is resolved from the vault at transfer time.
* Every change goes through a merge request with mandatory approval on
  the team's forge, whose branch protection enforces it. The deployment
  repository's own CI runs `irekey ci` on every merge request, which
  validates the change and runs the simulation in the lab.
* Field devices, that is devices that are neither bench nor lab devices,
  receive only release sets and layers from a commit that is clean and
  reachable from the remote protected branch (ApprovedStateOnly);
  otherwise the operation is refused before the device is changed
  (`REPO-003` for uncommitted changes, `REPO-005` for a commit that is
  not reachable from the protected branch).
* The forge is reached through a provider: `none` (a plain git remote),
  `gitlab` or `github`. `irekey doctor` warns when branch protection
  cannot be confirmed through the forge's API.
* irekey builds its graph from the repository on every run. Local
  operational state stays on the operator machine; anything another
  developer needs lives in the deployment repository.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once the forge providers and the
ApprovedStateOnly check are implemented and exercised in the lab.

### Consequences

* Good, because the public repository holds only code and fictional
  examples; customer names, addresses and host keys stay private.
* Good, because review, history, blame and revert come from git and the
  forge, without a new tool.
* Good, because every bundle records the deployment-repository commit it
  was built from, so each delivery traces back to an approved change.
* Bad, because approval is only as strong as the forge's branch
  protection; with the `none` provider irekey cannot confirm it, and says
  so.
* Bad, because everyone with access to the repository sees the data of
  every customer.
* Bad, because history summaries and snapshots make the repository grow
  over time.

### Confirmation

The decision is confirmed when tests named after ApprovedStateOnly show
that irekey refuses to deliver to a field device from a dirty working
copy, from a local-only commit and from a commit not reachable from the
remote protected branch, while it still delivers such state to bench and
lab devices; and when a merge request opened by `irekey save` in the lab
passes `irekey ci`. It
is revised if forges cannot report branch protection reliably through
their APIs, if a repository with 500 devices makes `irekey plan` slower
than the specification's two-second target on a laptop, or if teams need
access control per customer, which would split the data into several
repositories.

## Pros and Cons of the Options

### Separate private deployment repository

* Good, because the workflow (branch, merge request, approval, merge) is
  the one developers already use.
* Good, because the data is plain files that can be read and compared,
  and it works offline after a fetch.
* Good, because changes to customer data are independent of irekey's own
  releases.
* Bad, because git accepts any file, so irekey has to validate schemas
  and cross-references itself.

### Data in the public code repository

* Good, because there is only one repository to clone.
* Bad, because customer names, addresses and host keys would be public.
* Bad, because changes to customer data would mix with changes to
  irekey's code and releases.

### A database

* Good, because queries and concurrent writes are straightforward.
* Bad, because it needs a server or a shared file, while irekey runs from
  a laptop and must work offline.
* Bad, because review, approval, history and revert would have to be
  built, while git and the forge already provide them.

### Configuration in each component repository

* Good, because a component's per-customer configuration sits next to
  its code.
* Bad, because a device's desired state is spread across several
  repositories, and no single commit describes it.
* Bad, because customer, site and device data concern several components
  and would be duplicated in each of them.
* Bad, because component repositories may be shared more widely than
  customer data may be, and approval would have to be enforced in each.

## More Information

The specification's deployment repository format defines the layout and
the file formats. Related records: ADR 0005 (bases, layers and patch
stacks), ADR 0006 (release sets), ADR 0012 (vault providers), ADR 0013
(local operational state), ADR 0014 (YAML and JSON Schema).
