---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Implement SSH in process with golang.org/x/crypto/ssh

## Context and Problem Statement

Every interaction with a device goes over SSH, from a developer's laptop
running Linux, macOS or Windows, through the customer's VPN. irekey must
refuse a device that does not present its pinned host key
(PinnedIdentity), authenticate with credentials that may come from the
vault, including passwords, run many commands and long data streams over
one connection, notice a lost connection within seconds, and work
through jump hosts. Failures must be classified from structured signals,
so the operator learns whether the device was left unchanged.

Should irekey implement SSH inside its own process, or drive an external
SSH program?

## Decision Drivers

* Host-key pinning is enforced by irekey; no user setting may weaken it.
* Vault passwords work without helper programs or a terminal, and
  Windows laptops need no installed `ssh` program.
* One multiplexed connection per device per operation, with keepalives;
  failures map to error codes without parsing message text.
* A static binary without cgo, whose connections can be reproduced by
  hand.

## Considered Options

* In-process SSH with x/crypto/ssh
* Shelling out to OpenSSH
* libssh bindings through cgo

## Decision Outcome

Chosen option: "In-process SSH with x/crypto/ssh", because it is the only
option that enforces pinning in irekey's own code, takes vault-supplied
passwords directly and needs no external program on any operator
platform.

* `golang.org/x/crypto/ssh` provides the connection; its `knownhosts`
  package checks the device against the deployment repository's
  generated `known_hosts`, and `ssh/agent` talks to the user's SSH agent.
  `ssh.InsecureIgnoreHostKey()` is never used outside tests.
* Authentication order: SSH agent, identity files, then a key or a
  password (password or keyboard-interactive) from the vault.
* `github.com/kevinburke/ssh_config` reads `HostName`, `Port`, `User`,
  `ProxyJump` and `IdentityFile` from `~/.ssh/config`; nothing else is
  applied. On Windows, `github.com/Microsoft/go-winio` reaches the
  OpenSSH agent's named pipe.
* One connection per device per operation, one session per command,
  `keepalive@openssh.com` every 15 seconds; three missed replies mean
  the connection is lost.
* `-v` prints an equivalent `ssh` command line (with `-p`, `-J` and `-o
  UserKnownHostsFile=` set to the generated `known_hosts`), so any
  connection can be reproduced with OpenSSH.

This is the recommended default; it is confirmed or revised when the SSH
transport is implemented and tested against the lab devices. It would
change if devices run SSH servers that x/crypto/ssh cannot negotiate
with, or if developers rely on `~/.ssh/config` features outside the
subset. Transport is an extension point, so an OpenSSH-based transport
could then be added behind the same interface.

### Consequences

* Good, because pinning, authentication order and timeouts are tested
  code, and failures arrive as Go errors that map directly to `NET-*`
  and `ID-*` codes.
* Bad, because only a subset of `~/.ssh/config` is honored. The
  library's documentation disagrees with itself about `Match`: its README
  says `Match` is unsupported, its changelog says most criteria are
  implemented since 1.5, and the v1.6.0 parser treats `Match all` and
  `Match host` like `Host` blocks and refuses to parse a file with any
  other criterion, `exec` included. A host configured through `Match`
  blocks can therefore resolve differently than with `ssh`, or the file
  fails to parse; irekey must say so instead of connecting silently with
  other settings (to be verified with test files when the transport is
  implemented).
* Bad, because algorithms and security fixes follow x/crypto, shipped in
  irekey releases, not operating system updates.
* Bad, because ssh_config's license file holds its MIT text plus
  go-toml's MIT notice, which GitHub reports as NOASSERTION; the license
  check must be confirmed to classify it as MIT, or an explained
  exception is needed.

### Confirmation

* The guard script that fails on `InsecureIgnoreHostKey` or an
  unconditional host-key callback outside tests.
* Integration tests against real `sshd` in the lab images: the lab
  scenario where a device's host key changes is refused with `ID-001`;
  vault passwords; a jump host; a cut connection found by keepalives.
* A test runs the command line printed by `-v` with OpenSSH against a
  lab device and reaches the same host with the same pinned key.
* Keys in a hardware token, used through the SSH agent, must be verified
  to authenticate.

## Pros and Cons of the Options

### In-process SSH with x/crypto/ssh

* Good, because the host-key callback is irekey code, and one pure-Go
  process multiplexes connections to many devices on every platform.
* Bad, because irekey re-implements the OpenSSH behavior it needs:
  configuration lookup, jump chains, finding the SSH agent.

### Shelling out to OpenSSH

* Good, because the full `~/.ssh/config`, `ControlMaster` and every
  OpenSSH algorithm would be available, updated with the system.
* Bad, because vault passwords would need `SSH_ASKPASS` helpers or a
  pseudo-terminal, passing the secret through another program.
* Bad, because `ssh` "exits with the exit status of the remote command or
  with 255 if an error occurred", so a host-key mismatch, a failed login
  and an unreachable host all end with 255 and could only be told apart
  by parsing its messages.
* Bad, because pinning would depend on options that interact with the
  user's configuration, and Windows laptops on an OpenSSH installation.

### libssh bindings through cgo

* Good, because libssh is a mature C implementation.
* Bad, because it requires cgo, which the static-binary requirement
  excludes, and its LGPL license is outside the allowed licenses.

## More Information

* `golang.org/x/crypto` v0.58.0 (BSD-3-Clause), packages `ssh`,
  `ssh/knownhosts`, `ssh/agent`: https://pkg.go.dev/golang.org/x/crypto/ssh.
  It has no SSHSIG implementation yet (proposal
  https://github.com/golang/go/issues/68197, accepted 2025-08-15, not
  implemented); bundle signatures are ADR 0016.
* `github.com/kevinburke/ssh_config` v1.6.0 (API `Get`, `GetStrict`,
  `Decode`, `UserSettings`): https://github.com/kevinburke/ssh_config.
  Its README reads "Notably, the `Match` directive is currently
  unsupported"; its `CHANGELOG.md` lists Match support under version
  1.5; at v1.6.0, `parseMatch` in `parser.go` accepts only the `all` and
  `host` criteria and raises a parse error for the others.
  `github.com/Microsoft/go-winio` v0.6.4 (MIT):
  https://github.com/microsoft/go-winio.
* OpenSSH manual pages: ssh(1), "EXIT STATUS":
  https://man.openbsd.org/ssh.1; ssh-add(1) loads FIDO
  authenticator-hosted and PKCS#11 keys into the agent:
  https://man.openbsd.org/ssh-add.1
* libssh is licensed under the LGPL (version 2.1 in its `COPYING` file):
  https://www.libssh.org/features/ and
  https://gitlab.com/libssh/libssh-mirror/-/blob/master/COPYING
* All checked 2026-10-25.
* Credentials: ADR 0012; privilege on the device: ADR 0011.
  Specification sections "Transport" and "Architectural decisions".
