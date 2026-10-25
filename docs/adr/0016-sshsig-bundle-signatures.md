---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Sign bundles in the OpenSSH signature format (SSHSIG)

## Context and Problem Statement

A bundle carries `manifest.json`, which lists every file with its digest
and binds the bundle to one device ID and host-key fingerprint, the
missing blobs, and `manifest.json.sig`. The executor applies a bundle
only if the signature verifies against a key in the device's
trusted-signers file and every file matches its digest (SignedOnly).

The developer who delivers signs on the operator machine. The verifier is
`irekey-exec` on a device without internet access, whose clock may be
wrong, and which must stay a small static binary that never opens a
network connection. Developers already hold SSH keys to reach devices,
often in an SSH agent or on a hardware security key.

Which signature format should bundles use, and which implementation?

## Decision Drivers

* Verification works fully offline, inside the small executor binary.
* Developers sign with the SSH keys they already have, through an SSH
  agent or a hardware security key; no second kind of key.
* The format is published and can be checked by hand with standard
  tools; a bundle signature is valid for nothing else (domain
  separation).
* Trusted signers can change without a visit to the site.
* The signature code protects every device: small, reviewed, maintained.

## Considered Options

* SSHSIG with the hiddeco/sshsig library
* SSHSIG implemented in the repository on top of x/crypto/ssh
* minisign or signify keys
* Sigstore keyless signing with cosign
* OpenPGP signatures with GPG

## Decision Outcome

Chosen option:
"SSHSIG implemented in the repository on top of x/crypto/ssh", because
the format meets every driver, and owning its short framing code on top
of `golang.org/x/crypto/ssh`, which irekey already uses for its
transport, keeps a dependency with little recent activity out of the
path that protects every device.

* `manifest.json.sig` is an armored SSHSIG signature over the exact bytes
  of `manifest.json`, namespace `irekey-bundle@v1`, hash SHA-512, made
  through the SSH agent (which also serves hardware security keys) or a
  key file, and verified with x/crypto/ssh's public-key verification.
* `/etc/opt/irekey/trusted-signers` uses the allowed_signers format of
  `ssh-keygen`, each key restricted to the namespace `irekey-bundle@v1`.
  The executor accepts only the subset of the format that irekey writes
  and refuses any other option instead of ignoring it. A bad or untrusted
  signature is `BND-001`.
* The first state-changing operation on a device writes the file from the
  `signers` list in `irekey.yaml` at an approved commit
  (ApprovedStateOnly). Afterwards it changes only through the critical
  extension `trusted_signers`, which carries the complete new list and is
  applied only when the bundle is signed by a key in the current list; an
  executor that does not know the extension refuses the bundle
  (`BND-003`).
* Residual risk, recorded in the assurance case: an operator machine
  compromised before the first transfer to a device can add its own key
  to that device's trusted-signers file.
* When x/crypto/ssh provides SSHSIG (the accepted Go proposal
  golang/go#68197), irekey moves to it and removes its own code.

This record is proposed: the format is the specification's recommended
default, and the in-repo implementation is the alternative it names for
a library that is not maintained. It is confirmed or revised once bundle
signing and verification are implemented.

### Consequences

* Good, because developers sign with the keys they already use, a
  hardware security key keeps the private key off the laptop, and anyone
  can check a bundle with `ssh-keygen -Y verify -n irekey-bundle@v1`,
  given the signer's identity and an allowed_signers file.
* Good, because the executor gains no dependency beyond x/crypto/ssh.
* Bad, because irekey owns security-relevant parsing code, so
  interoperability tests and fuzzing are mandatory.
* Bad, because trust on a device starts at its first transfer, with the
  residual risk above.

### Confirmation

Interoperability tests in both directions: signatures made by irekey
verify with `ssh-keygen -Y verify`, and signatures made with
`ssh-keygen -Y sign` verify in irekey, for each supported key type.
SignedOnly tests refuse a changed manifest byte, a wrong namespace, an
untrusted key, an unknown trusted-signers option and an unknown critical
extension. The signature and trusted-signers parsers are fuzzed.

Evidence that would change the decision: x/crypto/ssh ships SSHSIG
(switch to it); hiddeco/sshsig becomes actively maintained and the
in-repo code proves harder to keep correct than expected (switch to the
library); or the open question about a second signature level, a CI key
attesting variant builds, is answered in a way SSHSIG cannot serve.

## Pros and Cons of the Options

### SSHSIG with the hiddeco/sshsig library

* Good, because it implements the format and is Apache-2.0.
* Bad, because, as observed on 2026-10-25, it shows little recent
  activity (last commit in June 2025, Dependabot pull requests not
  merged), and it would sit in the executor's verification path.

### SSHSIG implemented in the repository on top of x/crypto/ssh

* Good, because the format is short and published, the cryptography
  stays in x/crypto/ssh, and the upstream implementation can replace it.
* Bad, because the project writes and tests the framing code itself.

### minisign or signify keys

* Good, because both are small, well-understood Ed25519 formats.
* Bad, because every developer would need a second kind of key: both
  sign with their own secret-key files, and neither documents signing
  through the SSH agent or with a hardware security key.

### Sigstore keyless signing with cosign

* Good, because no long-lived signing key exists; irekey uses it for its
  own releases (ADR 0018), where signing happens online in CI.
* Bad, because signing needs an online identity provider, and offline
  verification needs a trust root kept current on devices that never
  reach the internet, in a large verifier.

### OpenPGP signatures with GPG

* Good, because it is a long-established standard.
* Bad, because developers would need OpenPGP keys in addition to their
  SSH keys, x/crypto's OpenPGP package is deprecated and unmaintained,
  and the maintained fork it points to would bring a full OpenPGP
  implementation into the executor.

## More Information

Facts checked on 2026-10-25 in the primary sources:

* SSHSIG, published by OpenSSH:
  https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.sshsig.
  Its supported hash algorithms are `sha256` and `sha512`, and the
  signed data starts with the six-byte preamble `SSHSIG`. ssh-keygen(1):
  for `-Y sign`, `hashalg` accepts `sha256` and `sha512`, default
  `sha512`; `-Y verify` takes the message on standard input, the
  namespace with `-n`, the signature with `-s`, the signer's identity
  with `-I` and the allowed signers file with `-f`; the
  allowed_signers format and its `namespaces=` option are under
  "ALLOWED SIGNERS": https://man.openbsd.org/ssh-keygen.1#ALLOWED_SIGNERS
* ssh-agent(1): by default, ssh-agent refuses signature requests for
  FIDO keys whose application string does not start with `ssh:` when
  the data to be signed does not appear to be an ssh(1) user
  authentication request or an ssh-keygen(1) signature; SSHSIG is the
  format of ssh-keygen(1) signatures:
  https://man.openbsd.org/ssh-agent.1. ssh-add(1) loads FIDO
  authenticator-hosted and PKCS#11 keys into the agent:
  https://man.openbsd.org/ssh-add.1. Signing with such keys through the
  agent is to be confirmed by the interoperability tests.
* `github.com/hiddeco/sshsig`: v0.2.0 (tag 2025-04-12), Apache-2.0, last
  commit 2025-06-13, Dependabot pull requests unmerged:
  https://github.com/hiddeco/sshsig
* `golang.org/x/crypto/ssh` has no SSHSIG implementation; the proposal to
  add one, golang/go#68197, was accepted on 2025-08-15 and is not yet
  implemented: https://github.com/golang/go/issues/68197
* x/crypto's `openpgp` package at v0.58.0: "Deprecated: this package is
  unsafe by design, and has numerous known security issues. It is not
  maintained, and should not be used."; it points to
  `github.com/ProtonMail/go-crypto/openpgp` as a maintained fork:
  https://github.com/golang/crypto/blob/v0.58.0/openpgp/read.go
* minisign "uses the highly secure Ed25519 public-key signature system"
  and documents its own key file formats:
  https://jedisct1.github.io/minisign/; signify:
  https://man.openbsd.org/signify.1, whose source uses Ed25519
  (`usr.bin/signify/signify.c` in OpenBSD); Sigstore:
  https://docs.sigstore.dev/

The specification's "Bundle format v0" section defines the manifest and
the critical-extension rule; its "Security" section lists the signing
control and the residual risk.
