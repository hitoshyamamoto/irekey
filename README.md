# irekey

[![CI](https://github.com/hitoshyamamoto/irekey/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hitoshyamamoto/irekey/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hitoshyamamoto/irekey)](go.mod)
[![License](https://img.shields.io/github/license/hitoshyamamoto/irekey)](LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/hitoshyamamoto/irekey/badge)](https://scorecard.dev/viewer/?uri=github.com/hitoshyamamoto/irekey)
<!--
Two badges are added later: the Release badge after the first release, and
the OpenSSF Best Practices badge after the project is registered there.
-->

irekey manages the same software running on many offline embedded Linux
computers, where every customer needs small local differences, and
delivers changes over SSH with automatic rollback.

**Status: early development.** There is no release. Two commands work
today: `irekey version`, which prints the version, commit, Go version and
platform, and `irekey-exec capabilities`, which prints one JSON line that
describes the device-side executor. Every other behavior described in
this README is the design in the [specification](docs/spec/masterprompt.md)
and is not implemented yet, unless a section says that it works today.
[ROADMAP.md](ROADMAP.md) shows the order of work.

## The name

`irekey` blends two ideas:

- **irekae** (入換): in Japanese railway operations, *shunting* — moving
  rolling stock between the tracks of a yard to regroup and form trains.
  Written 入れ替え, the same reading also means "swap" or "replacement",
  which is what an update does to a device.
- **key**: nothing is switched without the right key. The device's pinned
  SSH host key proves the target, and a trusted signing key authorizes the
  bundle.

*No switch without the right key.*

*Every device is a train. irekey is the yard where you inspect, switch and
dispatch them.*

| Railway | irekey |
|---|---|
| train | device |
| cars | the components on the device |
| formation (編成, *hensei*) | release set |
| yard tracks | groups and waves |
| inspection (検車, *kensha*) | snapshot and verification |
| shunting (入換, *irekae*) | transfer, activation and atomic switch |
| turntable (転車台, *tenshadai*) | rollback |
| timetable (ダイヤ, *daiya*) | rollout plan |

The metaphor ends here. Commands, flags, file formats, error codes and the
rest of the documentation use plain terms.

## What irekey is for

Picture a team that maintains one product made of several components: a
few services, a web frontend, a data analysis job and an infrastructure
repository. The product runs on hundreds of embedded Linux computers, one
or more at each customer site. Each device:

- is reachable only through the customer's VPN, from a developer's laptop,
  over SSH, and has no internet access;
- runs the same base product with small per-customer differences:
  configuration values, enabled modules, screen layouts, and sometimes
  small code changes;
- has been changed by hand over SSH for years, so nobody knows exactly
  what runs where;
- costs a long, expensive trip to the site if an update leaves it
  unreachable.

irekey replaces hundreds of hand-edited copies with one base code,
explicit and reviewed per-customer adjustments, and controlled delivery:

1. Register every device and record what it really runs (a snapshot of
   file hashes), without changing it.
2. Recover each customer's adjustments from the devices and keep them as
   reviewable configuration layers and patches in a private deployment
   repository, never as a branch per customer.
3. Change them with a normal git workflow: branch, merge request,
   approval, merge.
4. Update many devices at once from reusable groups: see for each device
   what changes and how many bytes will be transferred, validate the change
   (including a run on simulated devices) before anything is applied,
   update one canary device first, and roll back automatically when a
   health check fails or the operator cannot reconnect to confirm.

Delivery with rollback exists in other tools. What irekey adds is
extracting, maintaining and re-applying each customer's adjustments on top
of a base that keeps moving, across several repositories, for devices that
are reachable only now and then.

## Planned quick start

> **Not working yet.** This is the target experience for the first
> release. None of these commands exists today, and there is no release to
> download.

The goal is under one minute from download to the first adopted device,
with no configuration file. On a Linux x86-64 machine with SSH access to a
device:

```sh
curl -LO https://github.com/hitoshyamamoto/irekey/releases/latest/download/irekey-linux-amd64
curl -LO https://github.com/hitoshyamamoto/irekey/releases/latest/download/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
chmod +x irekey-linux-amd64 && mv irekey-linux-amd64 irekey
./irekey init
./irekey adopt 192.0.2.15 --customer acme --site north
```

- `irekey init` creates the private deployment repository and sets up a
  vault for device credentials: an existing Bitwarden account through the
  `bw` command line, or a new local encrypted file.
- `irekey adopt` connects over SSH, shows the device's host-key
  fingerprint, hostname and MAC addresses and asks you to confirm them.
  It then pins the host key, discovers the operating system, architecture,
  runtimes and components with read-only commands, takes a read-only
  snapshot, and commits the new device file in the deployment repository.
  Adoption never uses sudo; at most it uses a temporary directory in the
  SSH account's home, removed before it disconnects.

Each release will also carry a build provenance attestation, checked with
the GitHub CLI:
`gh attestation verify irekey-linux-amd64 --repo hitoshyamamoto/irekey`.

### Simulated device lab (planned)

`irekey lab up` will start a complete set of simulated devices in
containers (Debian with systemd and an SSH server), together with a git
server, a vault and simulated field instruments. Every workflow can then be
tried, and every failure provoked on purpose, without hardware. The lab
will need Docker on the operator machine, and it is optional and separate
from the irekey binary.

## Building from source

This works today. You need git and Go 1.21 or later. The
`toolchain go1.27.3` line in `go.mod` selects go1.27.3: with the default
`GOTOOLCHAIN=auto`, an older go command downloads that toolchain and
builds with it.

```sh
git clone https://github.com/hitoshyamamoto/irekey
cd irekey
go build ./cmd/irekey
./irekey version
```

`irekey version` prints the version, the commit, the Go version and the
platform on one line. A build from a git checkout gets a pseudo-version
from Go, so the line has this form:

```
irekey v0.0.0-<commit time>-<commit> (commit <first 12 hex digits>, go1.27.3, <os>/<arch>)
```

The device-side executor builds the same way. irekey is designed to upload
it to devices when needed; run by hand, it only reports what it supports:

```sh
go build ./cmd/irekey-exec
./irekey-exec capabilities
```

Run the tests with `go test ./...`. [CONTRIBUTING.md](CONTRIBUTING.md)
lists every check that CI runs.

## What irekey is not

- **Not a VPN manager.** Reaching the customer network is up to you;
  irekey starts from an SSH path to the device.
- **Not a daemon on the device.** No package to install and no open port:
  the executor is uploaded when needed, and outside an update nothing of
  irekey runs on the device.
- **Not an operating system updater.** It delivers your components and
  their configuration, not OS packages, kernels or firmware.
- **Not a pull-based updater** for always-online devices. The developer's
  machine pushes over SSH; the first versions have no other transport.
- **Not a tool for field technicians.** Its users are developers, and
  every change goes through a reviewed merge request before it can reach a
  field device.
- **Not a web console.** The command line is the product; the planned web
  interface is read-only.
- **Not an automatic fixer of drift.** irekey finds what differs on a
  device and suggests what to do with it; a human decides.
- **Not for Windows devices** in the first versions. The operator machine
  can run Linux, macOS or Windows.

Delta transfer, release channels other than `stable`, device holds and
graph visualization are not part of the first versions (see
[ROADMAP.md](ROADMAP.md)).

## Sponsors

Sponsorship is welcome through
[GitHub Sponsors](https://github.com/sponsors/hitoshyamamoto). The only
acknowledgement is the sponsor's logo or name and a link in this section,
if the sponsor wants it. No goods or services are expected or promised in
return: no support, no priority for bugs or features, and no influence on
the roadmap or on reviews.

No sponsors are listed yet.

## Documentation

| Document | What it covers |
|---|---|
| [Specification](docs/spec/masterprompt.md) | what irekey is, its safety invariants, the design and the engineering standard |
| [Architecture decision records](docs/adr/README.md) | each decision, the options considered and the reasons |
| [Open questions](docs/open-questions.md) | the assumptions of the design and the questions that are still open |
| [Glossary](docs/glossary.md) | the canonical vocabulary |
| [CONTRIBUTING.md](CONTRIBUTING.md) | development setup, the checks and the rules for changes |
| [GOVERNANCE.md](GOVERNANCE.md) | how features are accepted and decisions are made |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | the Contributor Covenant 2.1, which applies in every project space |
| [SECURITY.md](SECURITY.md) | how to report a vulnerability, and the security model |
| [Threat model](docs/threat-model.md) | assets, threats, trust boundaries, and the controls with their status |
| [STABILITY.md](STABILITY.md) | the compatibility contract |
| [ROADMAP.md](ROADMAP.md) | the order of work |
| [CHANGELOG.md](CHANGELOG.md) | every notable change |

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) first:
every change comes with tests and is written in English, and every commit
is signed off under the Developer Certificate of Origin
(`git commit -s`), which CI checks. Report security issues privately, as
described in [SECURITY.md](SECURITY.md), never in a public issue.

## License

MIT. See [LICENSE](LICENSE).
