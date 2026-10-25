---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Guard unconfirmed activations with a persistent systemd timer

## Context and Problem Statement

After an activation switches a device to a new release set, the new
release is kept only when the operator opens a new SSH connection and
confirms it (PathBack). Without confirmation within the window, or
after a reboot or power loss before it, the device must roll back by
itself: it is reachable only through the customer's VPN, and a device
left broken costs a trip to the site. Outside an activation window no
irekey process, timer or listener may exist on the device
(NothingPermanentRunning), and the mechanism must depend neither on
the SSH session (ConnectionIndependent) nor on the wall clock.

Which mechanism on the device rolls back an unconfirmed activation,
including after a reboot?

## Decision Drivers

* The guard survives a reboot or power loss during the window, and
  disappears completely when the window ends.
* It works without the SSH session or the laptop and is armed before
  the point of no return.
* It uses only systemd, already on the reference device, and monotonic
  time; it is idempotent.

## Considered Options

* Persistent timer and service in /etc/systemd/system
* Transient timer started with systemd-run
* Sleep-based watchdog process
* cron @reboot entry

## Decision Outcome

Chosen option: "Persistent timer and service in /etc/systemd/system",
decided by the maintainer, because unit files on disk are expected to
survive a reboot, yet exist only during the window.

* At `arm-guard`, the executor writes `irekey-guard-<update-id>.timer`
  and `.service` to `/etc/systemd/system/` (as root, ADR 0011), enables
  and starts the timer, and records the boot ID
  (`/proc/sys/kernel/random/boot_id`).
* The timer sets `OnActiveSec=` to the window (`max_downtime` + soak +
  confirmation window, the last 15 minutes by default), `OnBootSec=60s`
  and `AccuracySec=1s` (the one-minute default would stretch the
  window). The `Type=oneshot` service runs `irekey-exec guard --update
  <update-id>`.
* A timer activated after its `OnBootSec=` point elapses at once, so the
  guard runs right after arming; in that run (same boot ID, activation
  unit still active) it does nothing.
* Otherwise the guard checks the `pending-confirmation` marker and the
  journal. It rolls back when the boot ID changed and the update is
  unconfirmed, or when the `OnActiveSec=` window elapsed without
  confirmation; if the update is confirmed, it disarms itself.
* `disarm-guard` (after commit, abort or rollback) disables the timer
  and removes both unit files. The detached activation keeps a transient
  unit (`systemd-run --unit=irekey-activate-<update-id> --collect`),
  because the guard, not that unit, provides reboot safety.

### Consequences

* Good, because a reboot or power loss during the window is expected to
  end in rollback without anyone reaching the device.
* Good, because a power loss between confirmation and `disarm-guard` is
  harmless: at the next boot the guard finds the update confirmed and
  removes its unit files.
* Bad, because the guard must recognize its own firing right after
  arming.
* Bad, because monotonic timers pause while a device is suspended, so
  suspended time extends the window; reference devices run
  continuously, and the limitation is documented.

### Confirmation

* That an enabled timer starts again at boot, so the guard runs after a
  reboot, is an inference from the documentation. Before the first
  release it must be proven by the lab scenario where the device reboots
  during the confirmation window and by a bench test with real power
  loss.
* In containers the boot ID may come from the host kernel and stay the
  same when a lab device restarts (to be verified with the lab images);
  the lab scenario must account for this, and the bench test remains
  the proof of the changed-boot-ID path. In containers, systemd also maps
  `OnBootSec=` to `OnStartupSec=` for the system manager, so in the lab
  the boot timer counts from the start of the container's systemd.
* Tests of `guard` cover the firing right after arming, an elapsed
  window, a changed boot ID and a confirmed update; after every terminal
  state no `irekey-guard-*` unit file is left.

## Pros and Cons of the Options

### Persistent timer and service in /etc/systemd/system

* Good, because the unit files survive a reboot, and monotonic timers do
  not depend on the wall clock.
* Bad, because the files must be written and removed reliably.

### Transient timer started with systemd-run

`systemd-run --on-active=...` creates a transient timer and service.

* Good, because nothing is written to `/etc` and systemd cleans up.
* Bad, because transient units live under `/run` and are released when
  the system is rebooted, so a reboot during the window would remove
  the guard and keep an unconfirmed release: PathBack would fail in
  exactly the case the guard exists for.

### Sleep-based watchdog process

* Good, because it needs nothing from systemd beyond starting a process.
* Bad, because a sleeping process does not survive a reboot or power
  loss, and can be killed, for example by the out-of-memory killer.

### cron @reboot entry

* Good, because it could work on devices without systemd.
* Bad, because `@reboot` covers only reboots; the window still needs a
  sleeping process, and a cron daemon becomes a device requirement.
* Neutral, because with a sleep-based watchdog it remains the open
  fallback for devices without systemd, outside the first versions.

## More Information

* systemd.timer(5), systemd 262:
  https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html.
  `OnActiveSec=` is relative to the timer's activation, `OnBootSec=` to
  boot; "If a timer configured with OnBootSec= or OnStartupSec= is
  already in the past when the timer unit is activated, it will
  immediately elapse and the configured unit is started"; monotonic
  timers pause during suspend; `AccuracySec=` defaults to 1min. For
  `OnBootSec=`: "In containers, for the system manager instance, this is
  mapped to OnStartupSec=, making both equivalent."
* systemd-run(1) (`--unit=`, `--collect`, `--on-active=`):
  https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html.
* org.freedesktop.systemd1(5): `StartTransientUnit()` creates "a
  transient unit which will be released as soon as it is not running or
  referenced anymore or the system is rebooted":
  https://www.freedesktop.org/software/systemd/man/latest/org.freedesktop.systemd1.html.
  systemd.unit(5) lists `/run/systemd/transient` as the "Dynamic
  configuration for transient units":
  https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
* Linux kernel documentation, `/proc/sys/kernel/random/boot_id`: "a UUID
  generated the first time this is retrieved, and unvarying after that":
  https://docs.kernel.org/admin-guide/sysctl/kernel.html#random
* All checked 2026-10-25. Specification sections "Guard
  (confirm-or-revert)", "Activation state machine", "Executor" and
  "Fixtures and scenarios"; the cron fallback stays an open question.
