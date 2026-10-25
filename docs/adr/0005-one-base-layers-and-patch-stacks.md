---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Keep one base per component and express customer differences as layers and patch stacks

## Context and Problem Statement

The same product runs on more than 200 devices, and every customer needs
small differences: configuration values, enabled modules, screen layouts
and sometimes small code changes. The devices were edited by hand over
SSH, so nobody knows exactly what runs where. irekey has to recover those
differences, store them in a form that can be reviewed, keep them working
when a component's base moves forward, and answer "what does customer X
run?" without connecting to a device.

How should per-customer differences be represented?

## Decision Drivers

* A fix in a component's base reaches every customer without one merge
  per customer.
* A device's desired state is computed from data alone, as a pure
  function: `desired_state(device) = f(release set, layers, patch stacks)`.
* Every difference is explicit and reviewable in a merge request.
* Code changes that cannot be configuration are possible, but visible
  and measured.
* Devices that end up with the same code share one build and one test.
* Differences recovered from devices by extraction and triage fit the
  representation directly.

## Considered Options

* One base with layers and patch stacks
* A branch or fork per customer
* Configuration only, without code patches
* A template engine over the whole tree

## Decision Outcome

Chosen option: "One base with layers and patch stacks", as the
recommended default of the specification, because it keeps one line of
development per component and still expresses every kind of difference
found on devices.

* **Base:** a released version of a component, identified by a git tag
  and its commit, identical for everyone.
* **Layers:** configuration files that override values, from lowest to
  highest priority `base -> profiles -> customer -> site -> device`, with
  profiles in the order listed in the device file. Maps merge key by
  key, lists and scalars are replaced, an explicit `null` deletes a key,
  and a key missing from the component's configuration schema is an
  error. Every merged value records the layer file and line it came from.
* **Patch stacks:** for changes that cannot be configuration, `git
  format-patch` files plus an ordered `series` file, under
  `patches/<component>/<scope>/` for a profile, customer or device scope.
  They are applied in the order profile stacks, customer stack, device
  stack, with `git am --3way` onto a clean checkout of the base tag in a
  temporary worktree.
* **Variants:** a base with its patch stack applied, identified by a
  content hash; devices with the same variant share it.
* Developers do not edit `.patch` files: `irekey edit` opens a customer's
  code as a normal working tree, `irekey save` regenerates the stack, and
  `irekey rebase` re-applies every stack onto a new base and classifies
  each variant as `clean`, `fuzzy` or `conflict`.
* Lines changed by customer and device patches are reported as
  technical debt by `irekey status --debt`.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once layers, patch stacks and
extraction are implemented and run against the lab's seeded divergence.

### Consequences

* Good, because a base fix is one release of the component; customer
  differences follow it through `irekey rebase`, and only conflicting
  variants need a person.
* Good, because "what does customer X run" is a query on the deployment
  repository, and `irekey graph why` explains every configuration value.
* Bad, because patch stacks can conflict with a new base; resolving a
  conflict needs human judgment, and a fuzzy application needs review.
* Bad, because developers must decide where each difference belongs;
  extraction suggests a destination, but a person decides.
* Bad, because patch operations need the system `git` program (ADR 0008).

### Confirmation

The decision is confirmed when extraction places the lab's seeded
differences in layers and patch stacks that match the generator's ground
truth, when rebasing every stack onto a new base classifies the variants
as expected, and when property tests hold for the merge rules. It is
revised if extraction on real devices shows that customer variation is
mostly code rather than configuration (components would then need
extension points before more tooling), if rebases routinely end in
conflicts, or if a component, such as a frontend whose screen layouts
cannot be loaded at runtime, turns layer values into per-site builds.

## Pros and Cons of the Options

### One base with layers and patch stacks

* Good, because each difference is a small, named file next to the
  others for the same customer, site or device.
* Good, because the merge order and rules are deterministic, so a
  rendered configuration and its digest are stable.
* Good, because it follows proven patterns: quilt and Debian patch series
  for ordered patches on a pristine base, Yocto layers for priority.
* Bad, because it needs more machinery than branches: series files,
  rebase classification and provenance tracking.

### A branch or fork per customer

* Good, because it is plain git, which every developer knows.
* Bad, because with 200 long-lived branches every base fix becomes 200
  merges, and divergence only grows.
* Bad, because what a customer runs is hidden in a branch's merge
  history, and identical devices on different branches look different.

### Configuration only, without code patches

* Good, because configuration is the easiest difference to review,
  validate against a schema and roll back.
* Bad, because devices already carry small code changes that cannot be
  expressed as configuration until components add extension points.
* Neutral, because it stays the goal: extraction suggests a configuration
  value or an upstream change first, and patches count as debt.

### A template engine over the whole tree

* Good, because one mechanism expresses any difference.
* Bad, because the base is no longer code that runs and is tested as it
  stands; every file has to be rendered first.
* Bad, because differences hide in template conditionals, and a change
  recovered from a device would have to be rewritten as template logic.
* Neutral, because templates remain available for a component's declared
  output files, rendered with Go `text/template` and `missingkey=error`.

## More Information

The specification's customization model defines the layer files, merge
semantics, patch stack layout and variant identity.

Prior art, checked 2026-10-25: Debian's `debian/patches/series` "lists
all patches that have to be applied (in the given order) on top of the
upstream source package" (dpkg-source(1),
https://manpages.debian.org/unstable/dpkg-dev/dpkg-source.1.en.html),
a series that quilt can manage
(https://savannah.nongnu.org/projects/quilt); Yocto's `BBFILE_PRIORITY`
"assigns the priority for recipe files in each layer"
(https://docs.yoctoproject.org/ref-manual/variables.html). With the
`text/template` option `missingkey=error`, "execution stops immediately
with an error" on a missing map key
(https://pkg.go.dev/text/template#Template.Option).

Related records: ADR 0006 (release sets), ADR 0007 (deployment
repository), ADR 0008 (system git for patch operations).
