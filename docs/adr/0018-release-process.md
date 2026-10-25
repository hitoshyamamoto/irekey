---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Release with a curated changelog and maintainer-signed tags

## Context and Problem Statement

An irekey release runs on operator machines that hold VPN access, SSH
keys and a vault session, and it contains the executor that runs as root
on every managed device. A modified release would reach all of them, so
a compromised dependency or CI system is part of the threat model. Users
must be able to tell who decided a release, check that a binary was
built by the project's workflow from the tagged source, and rebuild it
to the same checksums. The maintainer already releases another Go
project, modtop, with a curated changelog, a maintainer-signed SSH tag
and a release workflow that runs on the pushed tag.

How are irekey versions released, and what does each release prove?

## Decision Drivers

* Only the maintainer can start a release, with a git tag signed by the
  maintainer's key.
* Anyone can rebuild a release from its tag and obtain the published
  `SHA256SUMS`, and can verify its provenance.
* No long-lived secret in the release path: the workflow token and
  keyless signing only.
* Release notes are written for operators deciding whether to update
  their devices, not generated from commit subjects.
* One maintainer: one process for all of the maintainer's Go projects.

## Considered Options

* Curated changelog and maintainer-signed tags
* release-please
* GoReleaser for build and publication
* Releases built on the maintainer's machine

## Decision Outcome

Chosen option: "Curated changelog and maintainer-signed tags", decided by
the maintainer, because it keeps the release decision in a signature only
the maintainer can make, and the pattern is already proven in modtop.

1. In `CHANGELOG.md` (Keep a Changelog 1.1.0), rename `[Unreleased]` to
   `## [X.Y.Z] - YYYY-MM-DD` and merge that change.
2. On the merge commit: `git tag -s vX.Y.Z -m "irekey X.Y.Z"`, then
   `git push origin vX.Y.Z`. Tags are signed with the SSH key listed in
   `.github/allowed_signers` (`gpg.format=ssh`) and checked with
   `git tag -v vX.Y.Z`.
3. The release workflow, started by the pushed tag, with write
   permissions only in its release job:
   * runs the tests, and extracts the version's changelog section into
     `$RUNNER_TEMP/notes.md`, outside the working tree so the build stays
     clean, failing if the section is missing;
   * builds `irekey-exec` for Linux (amd64, arm64, arm/v7), embeds their
     digests, then builds `irekey` for Linux, macOS and Windows (amd64,
     arm64), all with `CGO_ENABLED=0`, `-trimpath` and the version set
     through `-ldflags`;
   * writes `SHA256SUMS` and one CycloneDX SBOM per binary
     (`cyclonedx-gomod`), and attests build provenance for binaries and
     SBOMs with `actions/attest-build-provenance` (Sigstore keyless,
     with the workflow's identity), verifiable with
     `gh attestation verify`;
   * publishes with `gh release create --verify-tag`, so the release
     never creates a tag of its own, attaching the binaries,
     `SHA256SUMS`, the SBOMs, `THIRD_PARTY_NOTICES` and the provenance
     bundle as `irekey-X.Y.Z-provenance.sigstore.json`, a suffix the
     OpenSSF Scorecard Signed-Releases check recognizes as a signature
     (its highest score is given for an `*.intoto.jsonl` provenance
     file).
4. `CONTRIBUTING.md` explains how to reproduce a release; a nightly job
   rebuilds a released tag and compares the checksums.
5. Before 1.0, versions are `0.MINOR.PATCH`; a `0.x` release may break a
   contract, with an entry under "Changed" or "Removed".

The release workflow itself is added in a later change; this record
fixes the process it implements. Packages (Homebrew, Scoop or winget,
`.deb` or `.rpm`) are later work.

### Consequences

* Good, because each release shows who decided it (signed tag), which
  workflow built it from which commit (provenance), what it contains
  (SBOM) and what was published (checksums), and anyone can rebuild it.
* Good, because no stored secret exists that could leak from the release
  path, and irekey and modtop share one process.
* Bad, because the changelog and the version number are manual work (a
  missing changelog section stops the release, as intended), and losing
  the maintainer's signing key means a new key in
  `.github/allowed_signers`, under the continuity rules of
  `GOVERNANCE.md`.

### Confirmation

A dry run of the workflow (a `v0.0.0-rc.1` tag in a fork, or a manual
run) produces every asset, every SBOM and the provenance bundle;
`gh attestation verify` passes on each binary; a second build reproduces
`SHA256SUMS`. Afterwards, the nightly reproducibility job and the
Scorecard Signed-Releases check cover every release.

## Pros and Cons of the Options

### Curated changelog and maintainer-signed tags

* Good, because the signed tag is the release decision, checkable by
  anyone with `git tag -v`, and the notes are written for operators.
* Bad, because the maintainer performs two manual steps per release.

### release-please

A bot that opens release pull requests from Conventional Commits and,
when one is merged, creates the tag and the release.

* Good, because the project already writes Conventional Commits.
* Bad, because it tags the commit and creates the release itself, with
  the token it runs with rather than the maintainer's signing key, which
  conflicts with maintainer-signed tags, and its changelog is generated
  from commit messages instead of curated.

### GoReleaser for build and publication

* Good, because one configuration covers checksums, SBOMs, signing and
  the package formats planned for later; worth reconsidering then.
* Neutral, because it builds from an existing tag, so it is compatible
  with signed tags.
* Bad, because the two-step build (executors, their digests, then the
  CLI) and the reproducibility settings would move from a short script
  proven in modtop into a tool configuration to learn and pin.

### Releases built on the maintainer's machine

* Good, because it needs no workflow.
* Bad, because there is no provenance from a known build environment,
  and the upload needs a token on the laptop.

## More Information

* modtop: `.github/workflows/release.yml`, and the "Releasing
  (maintainers)" and "Reproducing a release" sections of its
  `CONTRIBUTING.md`. Its v0.1.0 was built with an untracked notes file in
  the working tree and needs a workaround to be reproduced; writing the
  notes to `$RUNNER_TEMP` avoids that.
* Keep a Changelog 1.1.0: https://keepachangelog.com/en/1.1.0/; Semantic
  Versioning: https://semver.org/spec/v2.0.0.html
* Signed tags: https://git-scm.com/docs/git-tag; `gpg.format` and
  `gpg.ssh.allowedSignersFile`: https://git-scm.com/docs/git-config
* cyclonedx-gomod: https://github.com/CycloneDX/cyclonedx-gomod;
  https://cli.github.com/manual/gh_attestation_verify;
  `gh release create --verify-tag`: "Abort in case the git tag doesn't
  already exist in the remote repository":
  https://cli.github.com/manual/gh_release_create
* `actions/attest-build-provenance` signs the attestation "using a
  short-lived Sigstore-issued signing certificate", verifiable with
  `gh attestation verify`. As of version 4 it "is simply a wrapper on
  top of `actions/attest`", and its README recommends `actions/attest`
  for new implementations, a point to weigh when the workflow is
  written:
  https://github.com/actions/attest-build-provenance,
  https://github.com/actions/attest
* release-please: when a release pull request is merged it "Tags the
  commit with the version number" and "Creates a GitHub Release based on
  the tag", from Conventional Commit messages:
  https://github.com/googleapis/release-please
* GoReleaser: checksums, SBOMs, signing, nFPM `.deb` and `.rpm`
  packages, Homebrew casks, Scoop and winget manifests are documented
  under https://goreleaser.com/customization/; it "will use the latest
  Git tag of your repository":
  https://goreleaser.com/getting-started/quick-start/
* OpenSSF Scorecard checks, "Signed-Releases" (`*.sigstore.json` counts
  as a signature, `*.intoto.jsonl` as SLSA provenance):
  https://github.com/ossf/scorecard/blob/main/docs/checks.md
* All checked 2026-10-25.

The specification's "Release process" and "CI workflows" sections define
the workflow in detail.
