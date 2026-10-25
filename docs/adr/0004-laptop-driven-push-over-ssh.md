---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Push from the operator machine over SSH, with nothing permanent running on devices

## Context and Problem Statement

The devices irekey manages have no internet access. Each one is reached
only through its customer's VPN, which a developer opens from a laptop,
and then over SSH. Today a device runs its OpenSSH server and the
product's components, nothing else. A device that becomes unreachable
after an update costs a long trip to its site.

Where does the logic that decides and drives an update run, and what may
irekey leave running or installed on a device?

## Decision Drivers

* Devices cannot reach any server on their own; they can only be driven
  from the operator machine while a connection is open.
* Anything installed or running permanently on more than 200 devices must
  itself be updated, monitored and secured. The specification decides: no
  daemon, no package and no open port on devices.
* A dropped VPN must never leave a device broken or half-updated.
* SSH and sudo already exist on every device and are what developers use.

## Considered Options

* Push from the operator machine over SSH
* Resident device agent that pulls
* Ansible push
* HTTPS pull

## Decision Outcome

Chosen option: "Push from the operator machine over SSH", decided by the
maintainer, because it is the only option that works with devices that
cannot reach a server, and it leaves nothing running on a device between
updates.

The operator machine is the control plane: `irekey` holds the deployment
repository, the vault session, the local operational state and the
executor binaries, decides each step and drives it over SSH. On devices:

* The executor, `irekey-exec`, is uploaded on demand. `irekey-exec
  install-self` re-checks its SHA-256 digest as root and copies it to
  `/opt/irekey/bin/<digest>/`; the operator machine verifies the
  installed digest before every use. At most two executors are kept.
* The executor never listens on a port and never initiates a network
  connection. It runs only when irekey invokes it over SSH or an armed
  guard starts it, and it is long-running only during an activation.
* Outside an activation window, no irekey process, timer or listener
  exists on the device (NothingPermanentRunning). The only persistent
  artifacts are the files under `/opt/irekey`, `/var/lib/irekey` and
  `/etc/opt/irekey`, plus two documented exceptions: the symlinks that
  replace legacy component paths, and the container images loaded into
  the local Docker image store (those of the current and previous
  release sets).
* During an activation window, the confirmation guard is the only unit
  written to disk: a persistent systemd timer and service pair that rolls
  back an activation the operator does not confirm over a new SSH
  connection, and that is removed when the window ends (ADR 0010). The
  activation itself runs detached from the SSH session, as a transient
  unit that does not outlive it (ConnectionIndependent).
* Adoption is read-only and uses no sudo; its only write is a temporary
  directory under the SSH account's home, removed in the same connection.

### Consequences

* Good, because between updates a device runs what it runs today; irekey
  adds no daemon to maintain and no open port.
* Good, because the decision logic, the credentials and the approved
  state stay on the operator machine, and every connection is checked
  against the device's pinned host key (PinnedIdentity).
* Good, because a new executor reaches a device with the next operation,
  without a separate upgrade campaign.
* Bad, because a device is observed and changed only while an operator
  is connected; drift is seen at the next connection, and work for an
  unreachable device waits in its pending queue.
* Bad, because the operator machine becomes a sensitive asset: it holds
  VPN access, keys and the vault session, and the threat model treats its
  compromise as a source of attack.
* Bad, because state-changing work needs root, which the first versions
  obtain through the SSH account's existing sudo (ADR 0011).

### Confirmation

Tests named after NothingPermanentRunning are designed to list, in the
lab, a device's processes, systemd units, timers, listening sockets and
files outside the three directories and the two exceptions after every
terminal state of an update, and to fail when any of them belongs to
irekey. The specification's working rules require the maintainer's
approval before adding a network listener or anything that runs
permanently on a device.

## Pros and Cons of the Options

### Push from the operator machine over SSH

* Good, because it needs only what devices already have: an OpenSSH
  server, a shell and sudo.
* Good, because it works through a VPN that only the operator opens.
* Bad, because the operator machine must be connected for every step,
  including confirmation.

### Resident device agent that pulls

A daemon on every device polls a server, applies updates and reports
back, as Mender does in its managed mode.

* Good, because devices update without an operator and report status
  continuously.
* Bad, because the devices have no server they can reach.
* Bad, because the daemon runs permanently on every device and must be
  updated, monitored and secured itself.
* Bad, because Mender's robust operating system updates, which roll back
  through the bootloader, need a redundant (A/B) root filesystem and a
  board integration that the existing devices were not built for; its
  application updates install on an existing system and update the
  active root filesystem instead.

### Ansible push

* Good, because it has the same model: push over SSH, nothing resident.
* Bad, because offline bundles, staging, a device-side journal and
  rollback are not built in; all device-side safety would still have to
  be written.
* Bad, because most modules need Python on the device, and the control
  node does not run natively on Windows, an operator platform.

### HTTPS pull

Devices fetch bundles from an HTTPS endpoint on a schedule.

* Good, because it suits always-online devices without an operator.
* Bad, because the devices in scope have no endpoint they can reach, and
  a scheduled fetch needs a timer or daemon on every device.
* Neutral, because transport stays an extension point; HTTPS pull for
  always-online devices is later work and would need its own record.

## More Information

* Mender documentation, checked 2026-10-25. "In managed mode, the Mender
  Client runs as a daemon and will regularly poll the server"; operating
  system updates use a dual redundant (A/B) scheme in which the
  bootloader rolls back to the previous partition ("Client modes of
  operation" and "Robust Operating System updates"):
  https://docs.mender.io/overview/introduction. Operating system updates
  need "a board integration", application updates "update components in
  the active root filesystem":
  https://docs.mender.io/client-installation/overview
* Ansible installation guide (control node not natively supported on
  Windows without WSL; Python on the managed node):
  https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html
* Related records: ADR 0009 (in-process SSH), ADR 0010 (persistent guard
  timer), ADR 0011 (privilege through sudo).
