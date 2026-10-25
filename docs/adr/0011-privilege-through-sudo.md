---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Gain privilege through the SSH account's sudo; defer restricted sudo

## Context and Problem Statement

Transfer and activation write to `/opt/irekey`, `/var/lib/irekey`,
`/etc/opt/irekey` and `/etc/systemd/system`, and stop and start
services, which needs root. irekey reaches devices through an existing
SSH account that normally has `sudo`, and nothing is installed on a
device in advance (ADR 0004). Some accounts need a sudo password, which
then comes from the vault. The executor's standard input also carries
data: the blob stream and secret values. Adoption never uses sudo.

How does irekey obtain root on a device, and how does a sudo password
reach `sudo` safely?

## Decision Drivers

* Nothing is installed or reconfigured on a device only to grant irekey
  privilege (NothingPermanentRunning).
* A sudo password never appears on a command line, where the process
  list would show it, nor in logs or output (DataSeparation).
* The privileged surface is one verified binary.
* Passwordless sudo and sudo with a password both work; read-only
  operations never need privilege.

## Considered Options

* The SSH account's existing sudo
* Restricted sudo through /etc/sudoers.d/irekey
* Root login over SSH

## Decision Outcome

Chosen option: "The SSH account's existing sudo", because it works on
devices as they are, much like Ansible's `become`, and keeps everything
privileged inside one binary whose digest irekey verifies.

* Privileged steps run as `sudo /opt/irekey/bin/<digest>/irekey-exec
  <subcommand>`. The only other command under sudo is the installation,
  `sudo <tmp>/irekey-exec install-self --sha256 <digest>`, which
  re-checks the digest as root. Every other remote command runs
  unprivileged.
* Passwordless sudo is used as it is; otherwise the vault's password is
  sent on standard input to `sudo -S -p ''`.
* When standard input also carries data, it is a sequence of
  length-prefixed frames (4-byte big-endian length, 1-byte frame type,
  payload; types: sudo password, secret values, tar stream, end). The
  sudo password is always the first frame, consumed by a thin `sudo -S`
  reader before the executor starts.
* `doctor <device>` reports the sudo mode; no sudo is `PRV-001`, a
  rejected password `PRV-002`. The restricted mode is deferred.

This is the recommended default; it is confirmed or revised when
transfer and activation are implemented. The exact handoff must be
verified in the lab and recorded here. sudo(8) says only that `-S` reads
the password from standard input; sudo's source reads it one byte at a
time up to a newline or carriage return. Whether `sudo -S` reads nothing
when its policy or cached credentials need no password is still to be
verified, and after a wrong password sudo asks again, up to
`passwd_tries` times (3 by default). The reader must neither leave a
password frame for the executor nor consume data meant for it. The
decision would change if the handoff cannot be made reliable, if devices
commonly configure sudo in a way this mode cannot use (for example
requiring a terminal), or if the restricted mode is accepted and becomes
the recommended setup.

### Consequences

* Good, because nothing is added to a device to grant privilege, and the
  privileged code is one binary the laptop verifies before every use.
* Bad, because the account's sudo usually allows any command; running
  only the executor under sudo is a rule of irekey, not enforced by the
  device. A compromised laptop that holds the password has full root on
  the device; the threat model and assurance case record this.
* Bad, because the vault holds sudo passwords for many devices, which
  raises its value to an attacker (ADR 0012), and the framing is a
  protocol between laptop and executor that must be kept compatible.

### Confirmation

* A test of the remote command builder asserts that `irekey-exec` is the
  only program run under sudo and that adoption never uses sudo.
* DataSeparation tests check that the sudo password never appears in
  an argument vector, in logs, in `--json` output or in error messages.
* Lab integration tests cover passwordless sudo, sudo with a password, a
  wrong password and an account without sudo, each with a tar stream
  after the password frame, including cached credentials, with which
  `sudo` runs "without requiring authentication" (sudo(8)). The frame
  parser is fuzzed.

## Pros and Cons of the Options

### The SSH account's existing sudo

* Good, because it needs no change to the device and works with and
  without a sudo password.
* Bad, because irekey cannot narrow what the account may do as root.

### Restricted sudo through /etc/sudoers.d/irekey

An optional `irekey bootstrap --restrict-sudo <device>` would install a
sudoers file that lets the account run only a fixed, root-owned executor
path, validated with `visudo -cf` before installation.

* Good, because the device itself would enforce that only the executor
  runs as root.
* Bad, because the sudoers file is a permanent artifact outside irekey's
  directories; it conflicts with the goal that nothing is permanently
  installed on devices, so it stays an open question that needs its own
  ADR.
* Bad, because executors live under content-addressed paths that change
  with every version, while a sudoers rule needs a fixed path.

### Root login over SSH

* Good, because there would be no sudo handling at all.
* Bad, because OpenSSH's default (`PermitRootLogin prohibit-password`)
  refuses password login as root, and changing it means reconfiguring
  `sshd` on every device.
* Bad, because every remote command, read-only ones included, would run
  as root.

## More Information

* sudo(8): `-S`: "Write the prompt to the standard error and read the
  password from the standard input instead of using the terminal
  device"; `-p` sets the prompt:
  https://www.sudo.ws/docs/man/sudo.man/. In sudo v1.9.17p2, `getln()` in
  `src/tgetpass.c` reads one byte at a time and stops at `\n` or `\r`:
  https://github.com/sudo-project/sudo/blob/v1.9.17p2/src/tgetpass.c
  (the sudo version on the devices is to be checked). sudoers(5):
  `passwd_tries` defaults to 3, `requiretty` is off by default:
  https://www.sudo.ws/docs/man/sudoers.man/. visudo(8): `-c` checks,
  `-f` names the file: https://www.sudo.ws/docs/man/visudo.man/
* sshd_config(5): the default of `PermitRootLogin` is
  `prohibit-password`, with which "password and keyboard-interactive
  authentication are disabled for root":
  https://man.openbsd.org/sshd_config.5
* All checked 2026-10-25. The guard (ADR 0010) needs this privilege; the
  transport that carries the frames is ADR 0009.
* Specification sections "Privilege", "Executor" and "Controls". Its
  open questions include the restricted sudo bootstrap and the choice
  between per-person device accounts and one maintenance account, which
  decides whose sudo credentials the vault holds.
