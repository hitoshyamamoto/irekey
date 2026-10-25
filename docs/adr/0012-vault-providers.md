---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Offer a local age-encrypted vault and a Bitwarden CLI vault provider

## Context and Problem Statement

irekey needs secrets for every device: SSH keys or passwords, sudo
passwords, and the values that configuration layers reference as
`secret://<item-reference>`, resolved at transfer time and written on
the device with mode `0600`. Device files hold only references, such as
`credentials: { ref: "secret://bitwarden/acme-north-01" }`. No plaintext
secret may reach the deployment repository, logs, CLI output or the
local cache (DataSeparation). `irekey init` sets up the vault within
the first-run share of the one-minute adoption budget. Today there is
one developer; a second may join.

Where does irekey keep secrets, and which providers does the first
version ship?

## Decision Drivers

* Plaintext secrets exist only in memory; no custom cryptography.
* A developer without a password manager gets a vault with no setup
  beyond a passphrase; a team with one can keep using it.
* A path to sharing when a second developer appears.
* irekey contacts no server except the devices, the configured forge and
  the configured vault.

## Considered Options

* Local age file and Bitwarden CLI
* Local age file only
* Encrypted secrets in the deployment repository
* A secrets server such as HashiCorp Vault

## Decision Outcome

Chosen option: "Local age file and Bitwarden CLI", because it serves
both the developer without a password manager and the team with one,
without custom cryptography. Both are providers behind the vault
extension point.

* `local`: one file encrypted with `filippo.io/age` to a scrypt
  passphrase recipient (`age.NewScryptRecipient`, read with
  `age.NewScryptIdentity`) in the user's configuration directory.
* `bitwarden`: drives the `bw` CLI against Bitwarden or a self-hosted
  Vaultwarden: detect `bw`, check `bw status`, unlock with the session
  key kept in memory only, and `bw lock` when done.
* For both: secrets are plaintext only in irekey's memory; the vault is
  locked again after each operation that unlocked it; every writer
  passes through redaction. A locked or unavailable vault is `LOC-003`,
  a missing item `ID-005`; `doctor` checks vault access.
* When a second distinct author appears in the deployment repository's
  history, irekey suggests `irekey vault migrate` to a shared vault.

This is the recommended default; it is confirmed or revised when `init`
and the providers are implemented. The `bw` commands, exit statuses and
JSON output must be verified then. The Bitwarden CLI documentation
describes two ways to pass the session key, the `BW_SESSION` environment
variable and the `--session` option; irekey uses the environment, so the
key never shows in the process list, and a lab test checks that no
argument vector carries it. The decision would change if `bw` output is
not stable enough to parse, if `bw` calls are too slow for the adoption
time budget, or if developers need another password manager, which would
become another provider.

### Consequences

* Good, because a new developer has an encrypted vault after one
  passphrase prompt, with nothing to install.
* Good, because teams with Bitwarden or Vaultwarden keep sharing and
  revocation where they already work; the lab's Vaultwarden container
  exercises this provider.
* Bad, because a scrypt recipient must be the only recipient of an age
  file, so the local vault cannot also be encrypted to a second
  developer's key, and age does not recommend scrypt for automated
  systems; sharing or unattended use needs another provider.
* Bad, because the `bitwarden` provider depends on an external program
  and its output format, and locking after every operation asks for the
  passphrase or master password again.
* Neutral, because "only in memory" means irekey never writes a secret
  to disk; Go does not guarantee that every copy in memory is erased,
  and the threat model says so.

### Confirmation

* DataSeparation tests write known secret values through every path
  and search logs, `--json` output, local state and the deployment
  repository for them; another test checks that every command that
  unlocked the vault locks it again.
* Local provider tests: round trip, wrong passphrase, and a file on disk
  that only ever holds ciphertext.
* Lab tests of the `bitwarden` provider against Vaultwarden: status,
  unlock, item lookup, lock, and no session key in any argument vector.

## Pros and Cons of the Options

### Local age file and Bitwarden CLI

* Good, because both kinds of user are served from the first version.
* Bad, because two providers must be maintained and tested from the
  start.

### Local age file only

* Good, because one provider and no external program.
* Bad, because a team's password manager could not be used, and sharing
  would wait for a later provider.

### Encrypted secrets in the deployment repository

Secrets encrypted to each developer's public key, committed next to the
device files.

* Good, because secrets would be versioned and reviewed with the rest.
* Bad, because every clone, CI runner and past commit would carry
  encrypted credentials for all devices; removing a developer would mean
  rotating every secret, and rotating one would need a merge request.

### A secrets server such as HashiCorp Vault

* Good, because it offers central policies, audit and sharing.
* Bad, because it means running and securing a server, more than the
  first versions need; it stays a candidate for a later provider.

## More Information

* `filippo.io/age` v1.3.2 (BSD-3-Clause): https://pkg.go.dev/filippo.io/age.
  Its documentation says a scrypt recipient "must be the only recipient
  for the file" and is "not recommended for automated systems". Checked
  2026-10-25.
* Bitwarden CLI: https://bitwarden.com/help/cli/, checked 2026-10-25.
  `bw unlock` returns a session key, used through the `BW_SESSION`
  environment variable or the global option `--session <session>`;
  `bw unlock` can also read the master password with `--passwordenv` or
  `--passwordfile`; `bw status` returns JSON with a `status` field;
  `bw lock` invalidates the session key; `bw config server <url>`
  connects the CLI to a self-hosted server. Exit statuses and the JSON
  output of item lookups are still to be verified.
* Vaultwarden describes itself as "compatible with official Bitwarden
  clients" (https://github.com/dani-garcia/vaultwarden), and the
  Bitwarden download page lists the CLI among those clients
  (https://bitwarden.com/download/); that the CLI works against it is
  to be confirmed by the lab tests.
* Specification sections "First run", "Configuration layers", "Scope of
  the first versions" and "Controls". The SSH transport that uses these
  credentials is ADR 0009; sudo passwords are ADR 0011.
