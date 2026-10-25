# Security policy

## Reporting a vulnerability

Please do not open a public issue. Report it privately through
[GitHub private vulnerability reporting](https://github.com/hitoshyamamoto/irekey/security/advisories/new).

Include a description, the steps to reproduce, the affected commit or
version (`irekey version`, or `irekey-exec capabilities` for the executor)
and, if you have one, a suggested fix. You can expect an acknowledgement
within 5 business days and an initial assessment within 14 days.

## Supported versions

No version has been released yet. Until the first release, reports are
handled against the `main` branch. After it, only the latest release
receives security fixes.

## Scope

irekey is in early development. Today the repository holds two minimal
commands, the CI workflows and the repository guards; the rest is a
design. In scope:

- the `irekey` and `irekey-exec` binaries built from `main`;
- the CI workflows and the repository guards, for example a way to get
  code that disables SSH host-key checking, or real-world data, past the
  guards;
- weaknesses in the design described in the specification and in
  [docs/threat-model.md](docs/threat-model.md). They are welcome before the
  design is implemented; report them the same way.

Once the design is implemented, issues of particular interest will be:

- any way to make irekey run an operation on a device whose SSH host key
  differs from the pinned one;
- any way to make the executor apply a bundle that is unsigned, signed by
  an untrusted key, bound to another device, or altered;
- any way to leave a device half-updated, or to keep an activation that
  was never confirmed;
- a plaintext secret in the deployment repository, in logs, in command
  output or in the local cache;
- anything of irekey left running or listening on a device outside an
  activation window;
- crashes, hangs or terminal escape sequences caused by output from a
  device;
- unsafe entries in a bundle: absolute paths, `..`, symlinks that leave the
  component, device files, setuid or setgid bits.

Out of scope: the security of the customer VPN itself, and physical access
to a device, beyond detecting the changes it leaves as drift.

## Security model: what you can and cannot expect

Most of irekey's security model is designed but not implemented yet. This
section keeps the two apart: what holds today, and what the design commits
to. [docs/threat-model.md](docs/threat-model.md) gives the status of every
control.

### What holds today

You can expect that, on the `main` branch:

- `irekey` has one command besides its help, `version`, which prints
  build information. It opens no network connection and writes no file.
- `irekey-exec` has one subcommand, `capabilities`, which prints one JSON
  line. The executor contains no network code: none of its dependencies,
  as listed by `go list -deps`, is a `net` package. It opens no connection
  and listens on nothing. No CI check enforces this yet; it is a property
  of the current code.
- CI runs the repository guards on every push to `main` and on every pull
  request to it, and a pull request cannot be merged until they pass.
  They reject:
  - Go code outside tests that disables SSH host-key verification (the
    `golang.org/x/crypto/ssh` helper that ignores host keys, or a host-key
    callback whose whole body is `return nil`);
  - Go code outside tests that assembles a shell command by string
    formatting or concatenation, anywhere except
    `internal/transport/ssh/quote.go`, the planned home of the one quoting
    function;
  - IP addresses outside the documentation and loopback ranges and
    globally administered MAC addresses in any text file, and domains
    other than the reserved example names and an allowlist of tool and
    hosting domains in documentation, examples, lab files, schemas, test
    data and the GitHub templates;
  - non-ASCII letters outside the files that explain the project's name
    and the test fixtures for UTF-8 handling;
  - Go modules that are not listed with an allowed license (MIT, BSD,
    Apache-2.0, ISC, MPL-2.0), so GPL code never becomes a dependency;
  - an executor that depends on operator-side packages or on modules
    outside the standard library, or whose stripped binary exceeds
    12 MiB;
  - Go and shell files without an SPDX license header.
- Every GitHub Action in the workflows is pinned to a full commit SHA, and
  a repository setting refuses to run an action referenced by a tag or a
  branch. Workflows run with a read-only token, and a job that needs more
  declares each permission with a comment.
- CI is configured to run `govulncheck`, CodeQL and OpenSSF Scorecard, and
  Dependabot proposes dependency updates every week. An optional workflow
  can arm auto-merge for Dependabot's pull requests, so they merge once
  every required check passes; it is disabled today, because no GitHub
  App is configured for it.
- A ruleset on `main` accepts changes only through squash-merged pull
  requests whose required CI checks pass on an up-to-date branch, with no
  bypass, the maintainer included. Another ruleset prevents release tags
  (`v*`) from being deleted, moved or updated.
- GitHub secret scanning with push protection is enabled for the
  repository.

You cannot expect today:

- any of the device-side protections below: irekey cannot connect to a
  device yet;
- verified downloads: there is no release, so no checksums, provenance
  attestations or signed release tags exist yet;
- independent review of changes: there is a single maintainer, and the
  ruleset on `main` requires no approving review.

### What the design commits to

The following is designed, not implemented yet. Each point is a safety
invariant of the specification, named as in its invariants table; each
will be protected by tests named after it.

- **Pinned identity** (`PinnedIdentity`). No operation runs on a device
  unless it presents the SSH host key pinned at adoption, after a human
  confirmed its fingerprint. State-changing operations also require the
  device's MAC address set to match. Each bundle is bound to one device
  and its host key, and the executor checks the binding. Host-key
  checking cannot be turned off.
- **Signed bundles** (`SignedOnly`). The executor applies only bundles
  whose manifest is signed by a key in the device's trusted-signers file
  and whose every file matches its digest.
- **Data separation** (`DataSeparation`). No plaintext secret in the
  deployment repository, in logs, in command output or in the local cache:
  configuration holds references to the vault, resolved only at transfer
  time. No customer data in this public repository.
- **Path back** (`PathBack`). An activation is kept only after the
  operator opens a new SSH connection and confirms it. A guard, armed on
  the device before the point of no return, rolls back when the
  confirmation does not arrive in time or when the device reboots first.
- **Nothing permanent running** (`NothingPermanentRunning`). Outside an
  activation window, no irekey process, timer or listener exists on the
  device. The executor is uploaded when needed, checked by its digest, and
  never listens on a port or initiates network connections. Whether its
  health checks may reach the device's own services on loopback is an
  open question of the specification.
- **Approved state only** (`ApprovedStateOnly`). Field devices receive
  only release sets and configuration from a clean deployment-repository
  commit that is reachable from the remote protected branch, so every
  change has been reviewed and approved on the forge first.
- **No half-updated device** (`AtomicSwitch`, `ConnectionIndependent`,
  `CrashSafe`). A device switches release sets with one atomic rename.
  Device-side work runs detached from the SSH session and is journaled, so
  a dropped connection or a power loss ends on the previous or the new
  release, never in between.

The design also bounds and sanitizes all output read from devices, binds
the planned web interface to the loopback address only, and sends no
telemetry: irekey contacts only the devices, the configured forge and the
configured vault.

Even with the design implemented, you will not be able to expect that
irekey:

- protects you from a compromised operator machine. That machine holds
  the VPN access, the SSH keys and the vault session. Review and signing
  limit what reaches field devices through irekey, but such a machine can
  still act on devices directly over SSH. In particular, a machine
  compromised before the first transfer to a device can add its own
  signing key to that device's trusted signers;
- secures the network path: the customer VPN is outside irekey;
- prevents changes made with physical access to a device; it can only
  report them as drift;
- works without root on the device: the executor runs through the SSH
  account's sudo.

## How we handle a report

1. Acknowledge the report within 5 business days.
2. Reproduce it and assess the impact within 14 days, keeping the reporter
   informed.
3. Prepare the fix in the private fork of a GitHub security advisory, with
   a regression test.
4. Release the fix and publish the advisory, with a CVE when one applies.
   The changelog lists the fix under "Security". Until the first release,
   the fix is merged to `main` and the advisory names that commit.
5. Credit the reporter in the advisory and the changelog, unless they ask
   to stay anonymous.
