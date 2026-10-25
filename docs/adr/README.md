# Architecture decision records

Each numbered file in this directory records one architecture decision of
irekey: the context, the options that were considered, the choice, and its
consequences. The format is MADR; see
[ADR 0001](0001-record-architecture-decisions.md) for the conventions and
[0000-template.md](0000-template.md) for the template.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions as MADR files in docs/adr | accepted |
| [0002](0002-go-as-implementation-language.md) | Implement irekey in Go | accepted |
| [0003](0003-mit-license.md) | License irekey under the MIT license | accepted |
| [0004](0004-laptop-driven-push-over-ssh.md) | Push from the operator machine over SSH, with nothing permanent running on devices | accepted |
| [0005](0005-one-base-layers-and-patch-stacks.md) | Keep one base per component and express customer differences as layers and patch stacks | proposed |
| [0006](0006-release-sets-as-unit-of-delivery.md) | Deliver and roll back release sets as one unit | proposed |
| [0007](0007-separate-private-deployment-repository.md) | Keep customer data in a separate private deployment repository, with approval enforced by the forge | proposed |
| [0008](0008-go-git-and-system-git.md) | Use go-git for simple git operations and system git for patch operations | proposed |
| [0009](0009-in-process-ssh.md) | Implement SSH in process with golang.org/x/crypto/ssh | proposed |
| [0010](0010-persistent-guard-timer.md) | Guard unconfirmed activations with a persistent systemd timer | accepted |
| [0011](0011-privilege-through-sudo.md) | Gain privilege through the SSH account's sudo; defer restricted sudo | proposed |
| [0012](0012-vault-providers.md) | Offer a local age-encrypted vault and a Bitwarden CLI vault provider | proposed |
| [0013](0013-sqlite-for-local-state.md) | Keep local operational state in SQLite with a pure-Go driver | proposed |
| [0014](0014-yaml-and-json-schema.md) | Write repository files in YAML and validate them with JSON Schema | proposed |
| [0015](0015-embedded-server-rendered-ui.md) | Embed a server-rendered web UI without a Node toolchain | proposed |
| [0016](0016-sshsig-bundle-signatures.md) | Sign bundles in the OpenSSH signature format (SSHSIG) | proposed |
| [0017](0017-containerized-lab.md) | Simulate the fleet in containers; keep GPL simulators in their own images | proposed |
| [0018](0018-release-process.md) | Release with a curated changelog and maintainer-signed tags | accepted |
| [0019](0019-stable-error-code-registry.md) | Register stable error codes in one file and generate code and documentation from it | proposed |
| [0020](0020-project-name-irekey.md) | Name the project irekey | accepted |
| [0021](0021-descriptive-invariant-names.md) | Refer to safety invariants by descriptive names outside the specification | accepted |
| [0022](0022-repository-guards-and-ci-checks.md) | Implement the repository guards and CI checks with these refinements | accepted |
