---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Deliver and roll back release sets as one unit

## Context and Problem Statement

A device runs several components, for example two services, a frontend,
an analytics job and the infrastructure repository. Each component is a
git repository with its own versions, some run under Docker Compose and
others as systemd services, and their versions depend on each other. If
components were updated one at a time, devices would collect combinations
that were never tested together, and rolling back one component could
create a combination that never existed anywhere. A failed update must
return the device to a state that is known to work.

What is the unit that irekey delivers to a device and rolls back?

## Decision Drivers

* Every combination of component versions on a field device has been
  tested as that combination.
* A rollback returns to a state that existed before, not to a new mix.
* A power loss or a dropped connection ends on the previous or the new
  state, never in between.
* Dependencies between component versions are checked before anything
  is delivered.
* Components under different runtimes are handled the same way.
* Device storage (about 27 GB on the reference device) stays bounded.

## Considered Options

* Release sets as the unit of delivery
* Per-component delivery
* Container-image-only delivery

## Decision Outcome

Chosen option: "Release sets as the unit of delivery", as the
recommended default of the specification, because it is the only option
in which a device always runs one tested combination, after an update as
well as after a rollback.

* A release set is a file in the deployment repository, named
  `<name>@<YYYY.MM.N>` (for example `product@2026.11.0`), that lists a
  base (a git tag) for each component, optional platform requirements and
  the maximum downtime. Once a rollout uses it, it is immutable; a change
  is a new version.
* Each component manifest declares `depends_on` version ranges on other
  components, and `irekey graph check` validates every release set
  against them before any connection.
* A device runs the components listed in its device file; the release
  set must contain all of them.
* On the device, every desired state gets its own directory,
  `/opt/irekey/releases/<release-id>/`, where the release ID is the
  device's desired digest, so a configuration-only change also gets its
  own directory. Unchanged files are hardlinked from a content-addressed
  blob pool.
* The switch is one atomic `rename(2)` of the `current` symlink: a new
  symlink is created under a temporary name and renamed over `current`
  (AtomicSwitch). `previous` points at the rollback target.
* Rollback restores the previous release set as a whole: code, container
  images and managed configuration (WholeSetRollback). Databases are
  restored only when a migration was declared irreversible.
* The current and previous release sets are kept; components whose code
  and rendered configuration did not change are not restarted.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once single-device delivery is
implemented and the lab's fault scenarios run against it.

### Consequences

* Good, because field devices only run combinations that passed the
  verification gates as a whole.
* Good, because a rollback is a switch back to a directory that is
  already on the device, with nothing to download.
* Good, because a device's state is described by one release-set version
  plus its own variants and rendered configuration.
* Bad, because even an urgent fix to one component needs a new
  release-set version, which goes through the gates.
* Bad, because two release directories and the container images of two
  release sets occupy device storage; the disk budget accounts for them.
* Bad, because release directories, staging and the blob pool must share
  one filesystem for hardlinks and the atomic rename; devices where this
  is not possible are refused.

### Confirmation

The decision is confirmed when lab scenarios show that an executor
killed at every journal step ends on the previous or the new release
set, that a reboot during the confirmation window rolls back the whole
set, that a broken release on the canary is rolled back as a whole, and
that a release set incompatible with a device's components fails
`irekey graph check` before any connection. It is revised if two release
sets with their images do not fit the storage of typical devices, if
new release-set versions for single-component fixes become a bottleneck,
or if common legacy layouts cannot be converted to symlinks into
`/opt/irekey/current/`.

## Pros and Cons of the Options

### Release sets as the unit of delivery

* Good, because compatibility is checked once, for the whole combination,
  with the dependency graph.
* Good, because one symlink switch changes every component at once, so
  no state mixes components from two release sets.
* Good, because it follows the manifest pattern of tools such as Google's
  `repo` and `vcstool`, whose manifests pin several repositories at fixed
  revisions.
* Neutral, because every version lists all components, including those
  that did not change; `irekey release new --from <version>` starts from
  the previous version.

### Per-component delivery

* Good, because one component can be updated without touching others.
* Bad, because the combinations across 200 devices grow without bound,
  and most of them are never tested together.
* Bad, because rolling back one component can create a combination that
  never existed, and a change that spans components cannot be switched
  in one step.

### Container-image-only delivery

* Good, because images are content-addressed, and previous images stay
  in the local image store until they are removed.
* Bad, because some components run as systemd services, not containers.
* Bad, because replacing several images is not one atomic step, so a
  failure midway leaves a mix.
* Bad, because rendered configuration, compose files and unit files
  still have to be delivered and rolled back with the images.
* Neutral, because images stay part of a release set: they are referenced
  by digest, loaded during transfer, and kept for the current and
  previous release sets.

## More Information

The specification's customization model defines the release-set file,
and its device-side layout defines the release directories and the
`current` and `previous` symlinks. A `repo` manifest gives each project
a `revision` that may be a branch, a git tag or a commit hash
(https://gerrit.googlesource.com/git-repo/+/HEAD/docs/manifest-format.md),
and `vcs export --exact` stores the exact revision of each repository
(https://github.com/dirk-thomas/vcstool). rename(2): "If newpath
already exists, it will be atomically replaced"; both rename(2) and
link(2) fail with `EXDEV` when the paths "are not on the same mounted
filesystem" (https://man7.org/linux/man-pages/man2/rename.2.html,
https://man7.org/linux/man-pages/man2/link.2.html). All checked
2026-10-25.

Related records: ADR 0005 (bases, layers and patch stacks), ADR 0010
(persistent guard timer).
