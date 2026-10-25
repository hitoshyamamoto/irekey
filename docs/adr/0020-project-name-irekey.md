---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Name the project irekey

## Context and Problem Statement

The specification was written with the working name "irekeys" and listed
the name as decided: repository and Go module
`github.com/hitoshyamamoto/irekeys`, binaries `irekeys` and `irekeys-exec`,
and every name derived from it (device paths, schema identifiers, the
signature namespace, image names, systemd unit names). The public GitHub
repository, however, was created as `hitoshyamamoto/irekey`. A project
whose repository, module path and binary names disagree is confusing to
install and to search for, so one name has to win before anything is
published.

Which name does the project use?

## Decision Drivers

* The repository, the Go module path and the binary names should be the
  same word.
* Nothing has been published yet: the repository has no release and no
  git tag, and the Go module proxy lists no version of the module. A
  rename now costs only edits to the specification.
* The meaning of the name, explained in the specification's "Name and
  metaphor" section, must still hold.

## Considered Options

* Use irekey everywhere
* Rename the repository to irekeys
* Repository irekey, module and binaries irekeys

## Decision Outcome

Chosen option: "Use irekey everywhere", decided by the maintainer, because
it matches the repository that already exists and keeps one word for the
repository, the module and the binaries. The singular "key" keeps the
meaning of the name; the specification's "Name and metaphor" section was
amended accordingly.

Every name derived from the project name changes accordingly:

| Item | Before | After |
|---|---|---|
| Repository and Go module | `github.com/hitoshyamamoto/irekeys` | `github.com/hitoshyamamoto/irekey` |
| Binaries and commands | `irekeys`, `irekeys-exec`, `irekeys <command>` | `irekey`, `irekey-exec`, `irekey <command>` |
| Source directories of the binaries | `cmd/irekeys`, `cmd/irekeys-exec` | `cmd/irekey`, `cmd/irekey-exec` |
| Device directories | `/opt/irekeys`, `/var/lib/irekeys`, `/etc/opt/irekeys` | `/opt/irekey`, `/var/lib/irekey`, `/etc/opt/irekey` |
| Operator cache directory | `~/.cache/irekeys/` | `~/.cache/irekey/` |
| Deployment repository root file | `irekeys.yaml` | `irekey.yaml` |
| Deployment repository CI template | `.github/workflows/irekeys.yml` | `.github/workflows/irekey.yml` |
| Component manifest directory | `.irekeys/` | `.irekey/` |
| Schema identifiers | `irekeys/<kind>/v<N>` | `irekey/<kind>/v<N>` |
| Bundle signature namespace | `irekeys-bundle@v1` | `irekey-bundle@v1` |
| systemd units on the device | `irekeys-guard-<update-id>`, `irekeys-activate-<update-id>` | `irekey-guard-<update-id>`, `irekey-activate-<update-id>` |
| Optional sudoers file | `/etc/sudoers.d/irekeys` | `/etc/sudoers.d/irekey` |
| Lab images | `ghcr.io/hitoshyamamoto/irekeys-lab-*` | `ghcr.io/hitoshyamamoto/irekey-lab-*` |
| Release provenance asset | `irekeys-X.Y.Z-provenance.sigstore.json` | `irekey-X.Y.Z-provenance.sigstore.json` |
| Release tag message | `irekeys X.Y.Z` | `irekey X.Y.Z` |

### Consequences

* Good, because the repository, the module path and the binaries agree.
* Good, because the change happens before any release, so no user,
  device or deployment repository has to migrate.
* Bad, because earlier drafts and notes that say "irekeys" are now
  outdated; the specification records the rename at its top.

### Confirmation

Searching the repository for "irekeys" finds it only in this record, in
the amendment note at the top of the specification, and in the changelog
entry that announces the rename.

## Pros and Cons of the Options

### Use irekey everywhere

* Good, because it matches the repository that exists.
* Good, because the singular "key" still carries the meaning of the name.
* Neutral, because the specification has to be edited in many places;
  the edit is mechanical and reviewable as one diff.

### Rename the repository to irekeys

* Good, because the specification stays unchanged.
* Bad, because the maintainer prefers the name the repository already
  has; the plural form existed only in the working draft.

### Repository irekey, module and binaries irekeys

* Bad, because the module path would not match the repository, and users
  would install a binary whose name differs from the project's.

## More Information

Every occurrence of the name in the specification was replaced, and its
"Name and metaphor" section was amended, in the same pull request as this
record.
