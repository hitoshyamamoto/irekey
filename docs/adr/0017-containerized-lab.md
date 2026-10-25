---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Simulate the fleet in containers; keep GPL simulators in their own images

## Context and Problem Statement

irekey's hardest promises concern failures: a VPN that drops during a
transfer or an activation, a reboot in the confirmation window, an
executor killed at any journal step, a full disk, a changed host key, two
operators on one device. In the field these happen by chance. The lab is
a simulated fleet where each can be provoked on purpose, on a laptop and
in CI, so every case becomes an automated test. Hardware problems (serial
ports, drivers, real power loss) stay the bench device's job.

The lab also needs a forge, a vault and data sources for freshness
checks, including an IEC 60870-5-104 simulator. The chosen one, the
Python package `c104` built on lib60870-C, is GPL-3.0; irekey is MIT.

How should the lab be built, distributed and started, and how is
copyleft simulator software kept apart from irekey?

## Decision Drivers

* Lab devices behave like the reference device: Debian, systemd as PID 1
  (guard timers, transient units, boot), an OpenSSH server, and Docker
  Compose for some components.
* It runs on one Linux machine and in hosted CI, scales to 200 devices,
  and is ready with 20 devices in under 5 minutes.
* Faults are deterministic: latency, loss, bandwidth limits and cuts on
  the network; kills and reboots on the device. Images are pinned by
  digest, with provenance; drift is seeded.
* GPL-licensed code is never linked into, embedded in or shipped inside
  the irekey binaries, and the lab stays optional.

## Considered Options

* Containers orchestrated with Docker Compose
* Virtual machines
* A local Kubernetes cluster
* In-process fakes without containers

## Decision Outcome

Chosen option: "Containers orchestrated with Docker Compose", because it
is the only option light enough for 200 devices on one machine and in CI
that still runs the real sshd, systemd and filesystem operations.

| Role | Lab container |
|---|---|
| devices | `device-systemd`: Debian, systemd as PID 1 and sshd, scaled N times |
| devices with Docker | `device-compose`: the same plus Docker-in-Docker; few, because it needs a privileged container |
| forge | Gitea by default; GitLab CE as an optional profile (`--profile gitlab`), because its documentation asks for at least 8 GB of memory |
| vault | Vaultwarden (AGPL-3.0), a server compatible with the official Bitwarden clients, the CLI among them |
| field instruments | Modbus TCP simulator (pymodbus) |
| telecontrol | IEC 60870-5-104 simulator (`c104` on lib60870-C, GPL-3.0) |
| operator machines | two `irekey` containers, for two concurrent developers |
| bad networks | Toxiproxy and/or `tc netem` |

* Image sources live in `lab/images/`; a workflow builds them for several
  architectures, publishes them as `ghcr.io/hitoshyamamoto/irekey-lab-*`
  and attests their provenance.
* The irekey binary embeds only the Compose file and the scenario
  definitions, which reference the images by digest, pinned at release
  time. `irekey lab up` needs Docker or Podman on the operator machine;
  Podman support and the settings systemd needs in a container (cgroups,
  a tmpfs on `/run`) are still to be verified on both engines.
* The IEC 104 simulator runs only in its own image, as its own process,
  reached only over the network. Publishing the image distributes
  GPL-3.0 software, so the image carries its license notice and gives
  access to the corresponding source of the version it contains, as
  GPL-3.0 asks for conveyed object code (clause 6). A guard checks that
  no GPL module appears in `go.mod`.
* Vaultwarden is AGPL-3.0 and is kept apart the same way: it runs only
  in its own container, as its own process, reached only over the
  network, and is never linked into, embedded in or shipped inside the
  irekey binaries. If the lab publishes its own Vaultwarden image, that
  image carries the license notice and gives access to the corresponding
  source in the same way.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once the lab foundation is
implemented and the fault scenarios run in it.

### Consequences

* Good, because every failure that matters can be scripted and repeated
  on every pull request that touches device-side code, with the same
  images everywhere.
* Good, because the license boundary is physical: separate image,
  separate process, network only.
* Bad, because containers share the host kernel: drivers, hardware and
  real power loss remain the bench's job, and a reboot is approximated
  by restarting a container.
* Bad, because systemd in containers depends on the container engine and
  the host's cgroup setup, and the lab needs Docker or Podman, disk and
  memory, and images to maintain.

### Confirmation

`irekey lab up --devices 20` on a clean Linux machine with Docker is
ready in under 5 minutes; every device accepts SSH; the divergence
generator gives the same result for the same seed; every published image
verifies with `gh attestation verify`; the license guard runs in CI.

Evidence that would change the decision: systemd containers prove
unreliable on hosted CI runners or under Podman (a few virtual machines
could then take over the reboot scenarios); 200 devices do not fit on one
machine; or an IEC 104 simulator under an allowed license appears.

## Pros and Cons of the Options

### Containers orchestrated with Docker Compose

* Good, because containers start in seconds, a device costs little
  memory, and Compose is already one of irekey's runtimes.
* Bad, because systemd as PID 1 needs special container settings.

### Virtual machines

* Good, because each one has its own kernel and boots for real.
* Bad, because 200 of them do not fit on one machine, and boot times
  would make every scenario slow.

### A local Kubernetes cluster

* Good, because it scales and has a rich API.
* Bad, because it adds a layer no field device has, while the devices
  would still be containers running systemd as PID 1.

### In-process fakes without containers

* Good, because tests would be fast and need no container engine.
* Bad, because they would not exercise the real sshd, systemd, sudo and
  rename semantics, which are what the lab exists to prove.

## More Information

Facts checked on 2026-10-25 in the primary sources:

* Docker Compose: https://docs.docker.com/compose/; Podman:
  https://podman.io/; Gitea: https://about.gitea.com/
* Docker-in-Docker: the documentation of the official `docker` image says
  "`--privileged` is required for Docker-in-Docker to function properly":
  https://hub.docker.com/_/docker
* GitLab installation requirements: "For a single-node installation, 16
  GB is the baseline", and GitLab "can run with at least 8 GB of memory"
  in memory-constrained environments:
  https://docs.gitlab.com/install/requirements/
* Vaultwarden, licensed AGPL-3.0 (`LICENSE.txt`), is "compatible with
  official Bitwarden clients": https://github.com/dani-garcia/vaultwarden;
  the Bitwarden download page lists the CLI among them:
  https://bitwarden.com/download/
* pymodbus: https://github.com/pymodbus-dev/pymodbus. `c104` 2.2.1 is
  GPL-3.0 and is built on lib60870-C v2:
  https://pypi.org/project/c104/. lib60870 is GPL-3.0 (`COPYING`), and
  its vendor also offers a commercial license:
  https://github.com/mz-automation/lib60870
* Toxiproxy, a TCP proxy for "simulating network conditions", with
  toxics such as `latency`, `bandwidth`, `down`, `timeout` and
  `packet_loss`: https://github.com/Shopify/toxiproxy; `tc-netem(8)`,
  with `delay`, `loss` and `rate`:
  https://man7.org/linux/man-pages/man8/tc-netem.8.html
* GPL-3.0, clause 6 "Conveying Non-Source Forms":
  https://www.gnu.org/licenses/gpl-3.0.html; AGPL-3.0:
  https://www.gnu.org/licenses/agpl-3.0.html

The specification's "Lab" section defines the roles, the five fictional
components and the scenarios; its "License compliance" section defines
the GPL rule.
