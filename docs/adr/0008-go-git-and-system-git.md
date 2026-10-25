---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Use go-git for simple git operations and system git for patch operations

## Context and Problem Statement

irekey uses git on the operator machine in two ways. The deployment
repository and the cached component clones need everyday operations:
init, add, commit, clone, fetch, push, and reading objects, refs, git
tags and history. Patch stacks need operations that only a complete git
provides: `git am --3way` onto a clean checkout of a base in a temporary
worktree, `git format-patch` to regenerate a stack, `rerere` in editing
workspaces, `git range-diff` for merge request summaries, and moving
stacks onto a new base. irekey ships as one static binary built without
cgo, and the first device must be adopted within a minute of downloading
it.

Which git implementation does irekey use for which operations?

## Decision Drivers

* Adoption, snapshots and every read-only command must work on a machine
  without system git.
* Patch operations must behave exactly like git, because developers
  review the resulting patches and `range-diff` output with their own
  git.
* No cgo: the operator binary cross-compiles for Linux, macOS and Windows
  on amd64 and arm64.

## Considered Options

* go-git for simple operations, system git for patch operations
* go-git for everything
* System git for everything
* libgit2 through cgo bindings

## Decision Outcome

Chosen option: "go-git for simple operations, system git for patch
operations", because it is the only option that keeps adoption free of
external programs while giving patch operations the behavior of real git.

* `github.com/go-git/go-git/v5` handles `init`, `add`, `commit`, `clone`,
  `fetch`, `push` and all read access.
* System `git`, started from an argument vector with `LC_ALL=C`, handles
  `am`, `apply`, `format-patch`, `rebase`, `rerere`, `range-diff` and
  `worktree add`. It is called only from the patch stack package.
* `irekey doctor` checks that system git is present and recent enough; a
  command that needs a patch operation fails with `LOC-001` otherwise.
* The minimum git version is set from the commands and options actually
  used when patch operations are implemented. Every planned command
  exists in git 2.19 and later, so the specification's provisional
  minimum of 2.38 is replaced by that measured value.

This is the recommended default; it is confirmed or revised when patch
operations are implemented. It would change if a stable go-git release
supported `am`, `apply`, `rebase` and linked worktrees well enough to
pass the patch stack tests (removing the system git dependency), or if
go-git's `fetch` or `push` proved unreliable against a supported forge
(moving those operations to system git too).

### Consequences

* Good, because adoption and inspection need nothing but the irekey
  binary, and patch stacks come from the same program developers review
  them with.
* Bad, because two git implementations touch the same repositories;
  tests must cover go-git reading repositories in which system git
  created linked worktrees or `rerere` records.
* Bad, because system git reads the user's configuration (for example
  `format.*` settings); irekey must pass the options that affect patch
  output explicitly, or the output differs between machines.

### Confirmation

* An integration test adopts a lab device with no `git` on `PATH`.
* Tests of `doctor` cover a missing git and one older than the minimum.
* Code review keeps system git calls inside the patch stack package.

## Pros and Cons of the Options

### go-git for simple operations, system git for patch operations

* Good, because each operation uses the implementation that supports it.
* Neutral, because system git is needed only by developers who maintain
  patch stacks.

### go-git for everything

go-git is a git implementation in pure Go (Apache-2.0).

* Good, because irekey would have no external runtime dependency.
* Bad, because go-git v5.19.3 lists `apply`, `am`, `cherry-pick`,
  `rebase`, `revert`, `format-patch` and `worktree` as not supported and
  does not mention `range-diff` or `rerere`; irekey would have to
  reimplement the core of patch stacks.
* Neutral, because the v6 line has partial `cherry-pick` and `worktree
  add` on its main branch, but exists only as a pre-release.

### System git for everything

* Good, because one implementation would give full git behavior.
* Bad, because adoption would require git on the operator machine.
* Bad, because every operation would parse subprocess output, also on
  Windows, where installations and line-ending settings vary.

### libgit2 through cgo bindings

* Good, because libgit2 implements more operations natively than go-git,
  for example applying diffs and rebasing.
* Bad, because it requires cgo, which the static-binary requirement
  excludes.

## More Information

* go-git v5.19.3 (2026-10-04, Apache-2.0, actively maintained; v6 only
  as v6.0.0-beta.1). Compatibility table:
  https://github.com/go-git/go-git/blob/v5.19.3/COMPATIBILITY.md. This
  re-checks the earlier finding recorded in the specification.
* git release notes:
  https://github.com/git/git/tree/master/Documentation/RelNotes.
  The 2.19.0 notes introduce `git range-diff` and the 2.20.0 notes
  `format-patch --range-diff`. `worktree` exists since 2.5.0: its manual
  page first appears at v2.5.0
  (https://github.com/git/git/blob/v2.5.0/Documentation/git-worktree.txt),
  and the 2.5.0 notes describe it, without naming it, as an experimental
  replacement for `git-new-workdir`. The other commands are far older.
* git-format-patch(1), "CONFIGURATION": `format.*` variables set extra
  headers, the subject prefix and file suffix, numbering, `To:` and
  `Cc:`, attachments, sign-off and the output directory:
  https://git-scm.com/docs/git-format-patch#_configuration
* libgit2 v1.9.7 headers `git2/apply.h` ("Apply patches to the working
  directory or index") and `git2/rebase.h`:
  https://github.com/libgit2/libgit2/tree/v1.9.7/include/git2
* All checked 2026-10-25. Specification sections "Architectural
  decisions", "Patch stacks" and "Editing and maintaining adjustments".
