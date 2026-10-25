# Roadmap

This roadmap covers October 2026 to October 2027. It states intent, not
promises: it helps users and contributors see where irekey is going and
where it is not going. Every feature still has to pass the criteria in
[GOVERNANCE.md](GOVERNANCE.md), starting with a real field case.

There is no release yet. The themes below follow the order of the work
plan in the specification
([docs/spec/masterprompt.md](docs/spec/masterprompt.md)); each one starts
when the previous one works and passes CI.

## Now: repository foundation

The base that every later change builds on. Already in place:

- the Go module and its package layout, with architecture tests for the
  dependency rule between packages;
- guards that keep the repository clean: license headers, no disabled
  host-key checking, no remote commands built by string formatting, no
  real-world data, English only, allowed dependency licenses, and the
  executor's dependencies and size;
- CI: lint, tests with an 80% coverage floor, fuzzing, vulnerability and
  license checks, CodeQL and OpenSSF Scorecard, all required before a
  merge to `main` except Scorecard;
- the first commands, `irekey version` and `irekey-exec capabilities`;
- the error code registry (`internal/errs/codes.yaml`), with its
  generated constants and [docs/errors.md](docs/errors.md);
- the community and governance documents, and the threat model;
- the architecture decision records for the decisions the specification
  has already taken, including the error code registry (ADR 0019), the
  open questions and the glossary.

Still to come in this theme:

- a release workflow with reproducible builds, checksums, SBOMs and build
  provenance;
- redaction of secrets in logs and output;
- `irekey doctor`, which checks the operator machine.

## Next

- **The simulated device lab.** `irekey lab up` starts simulated devices
  and the services around them in containers, so that every workflow can
  be tested in CI and on a laptop without hardware.
- **A read-only census of devices.** `irekey init` creates the deployment
  repository; `irekey adopt` registers a device and pins its SSH host key;
  `irekey snapshot`, `irekey status` and drift reports show what each
  device really runs. Nothing on a device is changed.
- **The customization model.** Configuration layers, patch stacks and
  variants on top of one base per component; the graph of how they relate;
  and extraction and triage, which recover each customer's adjustments
  from the devices for a developer to review and record.

## Later

The rest of the work plan, in this order:

- **Editing and rebasing adjustments.** `irekey edit`, `irekey save` and
  `irekey rebase`, with every change reviewed in a merge request and
  checked by verification gates (schemas, patches that apply, builds, and
  a simulation in the lab) before it can reach a device.
- **Safe single-device delivery.** Signed bundles, the device-side
  executor, its journal, the guard that rolls back an activation nobody
  confirmed, and rollback of the whole release set.
- **Rollouts across many devices.** Groups and waves, a canary first,
  failure budgets, concurrency limits, and a pending queue for devices
  that were unreachable.
- **The read-only web interface**, served locally by `irekey ui`.
- **Hardening and the first release, 0.1.0.** The threat model and the
  assurance case complete, the OpenSSF Best Practices "passing" badge, and
  the quick start verified on clean Linux, macOS and Windows machines.
- **Packages** for Homebrew, Scoop, winget, `.deb` and `.rpm`, after the
  first release.

Anything beyond this list comes in only with a real field case. Ideas are
recorded in the "Ideas on record" section of [CHANGELOG.md](CHANGELOG.md)
until someone brings one.

## Not planned

These are out of scope for the first versions:

- managing the VPN that reaches the devices (it stays the operator's
  responsibility);
- Windows devices;
- transports other than SSH;
- delta or chunked transfer;
- beta channels (only the stable channel is used);
- deploy actions from the web interface, which stays read-only;
- graph visualization;
- OS package, kernel and firmware updates;
- field technicians delivering updates;
- device holds;
- HTTPS pull for devices that are always online.

The design leaves extension points for some of them, such as other
transports and other device operating systems, so a real field case can
bring one back as a proposal.
