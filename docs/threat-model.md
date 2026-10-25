# Threat model

This document describes what irekey protects, what threatens it, where the
trust boundaries are, and which controls counter each threat. It is a
self-assessment by the maintainer, not an independent audit. What users can
rely on is stated in [SECURITY.md](../SECURITY.md).

Most of irekey is designed but not implemented yet. The Status column of
every control says which applies:

- **in place**: the control exists in this repository or its settings
  today;
- **designed**: the control is specified but not implemented yet, so it
  protects nothing yet.

This document is updated in the same pull request as every
security-relevant change: when a control is implemented (its status
changes, and the test or CI job that proves it is named), when a threat or
a trust boundary changes, and when a residual risk is found or removed.

## Assets

| Asset | Why it matters |
|---|---|
| Customer devices and the processes they control | A bad update can stop a customer's service, and an unreachable device needs a long trip to the site. |
| The path back to each device: the network, its SSH server and the credentials | Losing it turns a failed update into a site visit. |
| Device credentials for hundreds of sites | Whoever holds them can change any device. |
| The integrity of what reaches devices | Code and configuration on a device must be exactly what was reviewed. |
| The deployment repository | It holds customer data: customers, sites, device addresses, configuration, snapshots and history. It is private, never public. |
| The integrity of the irekey release binaries | The operator binary is designed to embed the executor, which runs as root on every device. |

## Threat sources

| Source | What it can do |
|---|---|
| The network path and the customer VPN | Intercept, drop or alter traffic; impersonate a device with a spoofed address; cut the connection in the middle of an update. |
| A compromised operator machine | It holds the VPN access, the SSH keys and the vault session, so it can push arbitrary code and configuration. |
| A careless or malicious insider | Make unreviewed changes; edit devices directly over SSH. |
| A tampered bundle | A bundle altered in transit or in the operator machine's cache, to run other code on a device. |
| A malicious or broken device | Return hostile output to the operator machine: huge output, terminal escape sequences, malformed JSON. |
| A compromised dependency or CI pipeline | Ship a modified irekey release. |
| Physical access to a device | Out of scope, beyond detecting the changes it leaves as drift. |

## Trust boundaries

These are the boundaries of the design. Today irekey cannot connect to a
device, so only the supply chain boundary is crossed by real code; the
repository guards marked "in place" below keep code that would weaken the
other boundaries from being merged.

1. **The operator machine.** irekey, the working copy of the deployment
   repository, the local state, the SSH keys or agent and the unlocked
   vault session live here. It is the control plane, and it is trusted as
   long as it is not compromised.
2. **Operator machine to device.** SSH over the customer VPN. The network
   is untrusted: the device is authenticated by its pinned host key, and
   everything it returns is untrusted input.
3. **The device.** The executor runs there as root, through the SSH
   account's sudo. It trusts only bundles that are signed by a key in the
   device's trusted-signers file and bound to that device.
4. **The forge.** It hosts the deployment repository and enforces
   merge-request approval through branch protection. irekey treats the
   protected branch as the approved state.
5. **The vault.** A local encrypted file, or a Bitwarden account through
   the `bw` command line. Secrets leave it only into memory on the operator
   machine and into files on the device.
6. **The supply chain.** From this repository, through GitHub Actions, to
   the release artifacts, including the Go modules and the actions used.

## Controls

### The network path and the customer VPN

| Control | Status |
|---|---|
| Every connection checks the device's pinned SSH host key. Trust on first use happens only during adoption, after a human confirms the fingerprint, or with a fingerprint obtained out of band. | designed |
| A CI guard rejects Go code outside tests that disables host-key verification. | in place |
| State-changing operations also require the device's MAC address set to match. | designed |
| Each bundle is bound to one device ID and host-key fingerprint, and the executor re-checks the binding. | designed |
| Transfers are verified blob by blob against SHA-256 digests, and resume after a cut. | designed |
| Device-side work runs detached from the SSH session and is journaled, so a cut connection never leaves a device half-updated; the guard rolls back an activation that is not confirmed. | designed |

### A compromised operator machine

| Control | Status |
|---|---|
| Field devices receive only release sets and configuration from a clean commit reachable from the deployment repository's protected branch; the forge enforces approval. | designed |
| Devices apply only bundles signed by a key in their trusted-signers file. The file is written at the first transfer, from the signer list of an approved commit, and is changed afterwards only by a bundle signed by a currently trusted key. | designed |
| Personal SSH keys in an agent or a hardware token are preferred; the vault is locked again after each use; secrets stay in memory and are redacted from all output. | designed |
| Only the executor runs under sudo; every other remote command runs unprivileged. | designed |

### A careless or malicious insider

| Control | Status |
|---|---|
| Every change to the deployment repository goes through a merge request with mandatory approval. | designed |
| Every connection compares the device with its expected state. Untriaged drift blocks updates, and only a recorded human decision resolves it. | designed |
| No flag disables a safety check; the few interactive overrides are recorded in history. | designed |
| Every delivery is recorded in history: who, what, when, the checks passed, and the result per device. | designed |
| In this repository, a CI job fails every pull request with a commit that has no Developer Certificate of Origin sign-off. | in place |
| In this repository, a ruleset on `main` accepts changes only through squash-merged pull requests whose required CI checks (`lint`, `test`, `build`, `dco`, `codeql`, `licenses`, `guards`) pass on an up-to-date branch, with no bypass, the maintainer included. It requires no approving review, because there is a single maintainer. | in place |

### A tampered bundle

| Control | Status |
|---|---|
| The manifest is signed in the OpenSSH signature format (namespace `irekey-bundle@v1`), and every blob is checked against its SHA-256 digest before use. | designed |
| Unknown bundle formats and unknown critical extensions are refused. | designed |
| Unsafe entries are refused: absolute paths, `..`, symlinks that leave the component, device files, setuid and setgid bits. | designed |
| Limits on the number of files, the bundle size and the path length. | designed |
| The manifest and archive parsers are fuzzed. | designed |

### A malicious or broken device

| Control | Status |
|---|---|
| Every read from a device is size-bounded and parsed strictly, from machine-readable output produced with `LC_ALL=C`, never from localized text. | designed |
| Terminal output is sanitized: no control characters except newline and tab. | designed |
| Remote commands are built from an argument vector and quoted by one tested function. | designed |
| A CI guard rejects remote commands assembled by string formatting outside that function. | in place |
| The parsers of device output are fuzzed. | designed |

### A compromised dependency or CI pipeline

| Control | Status |
|---|---|
| Every action in the workflows is pinned to a full commit SHA, and a repository setting refuses to run an action referenced by a tag or a branch. | in place |
| The default workflow token is read-only (a repository setting, and `permissions` in every workflow); a job that needs more declares each permission with a comment; checkouts do not keep credentials. | in place |
| Dependabot proposes Go module and action updates weekly, after a one-week cooldown; Dependabot alerts and security updates are enabled. An optional workflow can arm auto-merge for Dependabot's pull requests once every required check passes; it is disabled today (no GitHub App is configured), and when enabled it merges dependency updates without a human review. | in place (auto-merge: disabled) |
| `govulncheck` checks the code and its dependencies in CI. | in place |
| CodeQL (security-extended queries), OpenSSF Scorecard and zizmor (analysis of the workflows) run in CI. | in place |
| A guard requires every Go module to be listed with an allowed license, so GPL code never becomes a dependency; `go-licenses` checks the licenses in CI. | in place |
| The executor depends on no operator-side package and on nothing outside the standard library and this module, and its stripped binary is limited to 12 MiB; an architecture test checks the dependencies, and a guard checks the dependencies and the size. | in place |
| The executor contains no network code. | in place: true of the current code; no check enforces it yet |
| GitHub secret scanning with push protection is enabled. | in place |
| A repository ruleset prevents release tags (`v*`) from being deleted, moved or updated. | in place |
| Release tags are signed with the maintainer's SSH key, whose public half is in `.github/allowed_signers`. | designed: the public key is in place, but no release tag exists yet |
| Releases are built reproducibly and published with `SHA256SUMS`, SBOMs and Sigstore build provenance attestations. | designed: the release workflow does not exist yet |
| Native Go fuzzing in CI: briefly on every change, longer every week. | in place for the parsers of the repository guards; designed for irekey's own parsers |

### Physical access to a device

| Control | Status |
|---|---|
| Out of scope. Changes made on the device show up as drift at the next connection, and block updates until they are triaged. | designed |

## Residual risks

- **A machine compromised before the first transfer.** The trusted-signers
  file is written on a device by the first transfer to it. An operator
  machine that is compromised at that moment can add its own signing key
  to that file, and bundles signed with that key are accepted later.
- **A compromised operator machine in general.** It holds the VPN access,
  the SSH keys, the vault session and the sudo rights of the device
  account. Approval and signing limit what it can deliver through irekey,
  but it can act on devices directly over SSH, and irekey can only report
  the result as drift.
- **Trust on first use.** If a device is impersonated at the moment of
  adoption and the operator confirms the fingerprint without checking it,
  the wrong host key is pinned. A fingerprint obtained out of band avoids
  this.
- **Root on the device.** The executor runs as root through sudo, so a
  defect in it can affect the whole device. A restricted sudo mode is an
  open question, because it would leave a permanent file on the device.
- **The guard after a reboot.** That an enabled systemd timer brings the
  guard back after a reboot is inferred from the systemd documentation. A
  lab scenario and a bench test with real power loss must confirm it
  before the first release.
- **Approval depends on the forge.** irekey relies on the forge's branch
  protection for approval; when it cannot confirm that protection through
  the forge's API, it can only warn.
- **Single maintainer.** There is no independent review of changes; the
  bus factor is 1.
- **Design only.** Until a designed control is implemented and tested, it
  protects nothing.
