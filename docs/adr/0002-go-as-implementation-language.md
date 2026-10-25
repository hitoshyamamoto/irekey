---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Implement irekey in Go

## Context and Problem Statement

irekey consists of two programs. The operator CLI, `irekey`, runs on a
developer's machine: Linux, macOS or Windows, on amd64 or arm64. The
device-side executor, `irekey-exec`, is uploaded on demand to embedded
Linux devices (amd64 first, arm64 and armv7 too) that have no internet
access and on which nothing may be installed beyond irekey's own files.
The specification asks for a single binary that needs no configuration
for the first device, and for less than one minute from download to the
first adopted device.

Which language should irekey be written in?

## Decision Drivers

* One self-contained binary per platform, with no interpreter, shared
  library or package to install on the operator machine or the device.
* Cross-compilation from one build machine to six operator targets
  (linux, darwin and windows, each on amd64 and arm64) and three executor
  targets (linux on amd64, arm64 and armv7).
* A small executor: the target is under 8 MiB stripped for linux/amd64.
* A mature in-process SSH implementation with full control of host-key
  verification.
* Reproducible release builds: anyone can rebuild a release from its git
  tag and obtain the published `SHA256SUMS`.
* Maintainable by one person, with tooling the maintainer already uses.

## Considered Options

* Go
* Rust
* Python with PyInstaller
* Shell scripts and Ansible

## Decision Outcome

Chosen option: "Go", decided by the maintainer, because it meets every
driver with the standard toolchain alone: static binaries without cgo,
cross-compilation through environment variables, an SSH implementation
maintained by the Go project, and the build and release patterns of the
maintainer's other Go project, modtop.

Build rules that follow from this decision:

* Every binary is built with `CGO_ENABLED=0`; the code uses neither cgo
  nor `unsafe`.
* Release builds use `-trimpath` and fixed `-ldflags`, so the output does
  not depend on the paths of the build machine.
* The executors for `linux/amd64`, `linux/arm64` and `linux/arm` with
  `GOARM=7` are built first and embedded into the operator CLI with the
  `embed` package, together with their digests.

### Consequences

* Good, because the operator downloads one file, checks it with
  `sha256sum` and runs it; nothing else has to be installed.
* Good, because the executor does not depend on the shared libraries or
  interpreter versions of each device.
* Good, because `golang.org/x/crypto/ssh` (BSD-3-Clause), maintained by
  the Go project, provides the in-process SSH that host-key pinning and
  vault-supplied credentials need (ADR 0009).
* Bad, because libraries that wrap C code, such as the SQLite C library
  or libgit2, cannot be used; irekey needs a pure-Go SQLite driver
  (ADR 0013) and calls the system `git` program for patch operations
  (ADR 0008).
* Bad, because the Go runtime is linked into every binary, which sets a
  floor on the executor's size; a CI check has to watch it.

### Confirmation

The specification's CI design confirms this decision: the build job
cross-compiles every target with `CGO_ENABLED=0` and fails when the
executor exceeds its size limit, and a nightly job rebuilds a released
git tag and compares the result with the published `SHA256SUMS`. Code
review rejects cgo and `unsafe`.

## Pros and Cons of the Options

### Go

* Good, because `CGO_ENABLED=0` gives static binaries, and `GOOS`,
  `GOARCH` and `GOARM` cross-compile to every target from one machine.
* Good, because the standard library covers most needs (`crypto/sha256`,
  `archive/tar`, `encoding/json`, `html/template`, `log/slog`, `embed`),
  which keeps the dependency list short.
* Good, because modtop already releases static binaries for linux amd64,
  arm64 and armv7 built with `CGO_ENABLED=0` and `-trimpath`, so its lint
  configuration and workflows can be reused with small changes.

### Rust

* Good, because it produces fast native binaries without a garbage
  collector, and its type system rules out classes of memory and
  concurrency bugs at compile time.
* Bad, because cross-compiling to nine targets needs the standard library
  of each target and, for many pairs of host and target, a linker for
  that target; on Linux, the targets that link the C runtime statically
  by default are the separate musl targets.
* Bad, because the standard library has no SSH: the `ssh2` crate, hosted
  in the rust-lang organization, binds the C library libssh2, which needs
  a C toolchain for every target, and other SSH implementations, such as
  `russh`, are third-party crates.
* Bad, because the maintainer's tooling and release patterns are for Go;
  a second toolchain is a cost for a project with one maintainer.

### Python with PyInstaller

* Good, because Python is quick to write and widely known.
* Bad, because PyInstaller's documentation asks to install it and bundle
  the application separately on each platform, so every target needs its
  own build environment.
* Bad, because a one-file bundle unpacks itself into a temporary
  directory at every start, an extra write on the device at each run.
* Bad, because every bundle carries a Python interpreter, which makes the
  executor's size budget hard to meet.

### Shell scripts and Ansible

* Good, because Ansible already pushes over SSH with nothing resident on
  the managed host, which is the delivery model irekey needs (ADR 0004).
* Bad, because Ansible's control node does not run natively on Windows
  (only under the Windows Subsystem for Linux), an operator platform.
* Bad, because most Ansible modules need Python on the managed host, and
  the operator would install Python and Ansible before the first
  adoption, which rules out a single binary.
* Bad, because the device-side safety logic (journal, detached
  activation, guard, recovery) would still have to be written, in shell,
  where quoting and error handling are hard to test.

## More Information

Facts checked on 2026-10-25 in the primary sources:

* modtop: https://github.com/hitoshyamamoto/modtop; its
  `.github/workflows/release.yml` builds linux amd64, arm64 and armv7
  with `CGO_ENABLED=0` and `-trimpath`.
* `golang.org/x/crypto/ssh`: https://pkg.go.dev/golang.org/x/crypto/ssh;
  its license file is BSD-3-Clause. The `golang.org/x/...` repositories
  "are part of the Go Project but outside the main Go tree":
  https://go.dev/wiki/X-Repositories
* Rust: `rustup target add` "only installs the Rust standard library for
  a given target", and for some host and target combinations "there are
  typically other tools necessary to cross-compile, particularly a
  linker":
  https://rust-lang.github.io/rustup/cross-compilation.html. The musl
  targets link the C runtime statically by default:
  https://doc.rust-lang.org/reference/linkage.html#static-and-dynamic-c-runtimes.
  `ssh2`, "Rust bindings to libssh2": https://github.com/rust-lang/ssh2-rs
* PyInstaller: "you must install PyInstaller on each platform and bundle
  your app separately on each":
  https://pyinstaller.org/en/stable/usage.html. A one-file program
  creates a temporary `_MEIxxxxxx` folder when started and writes the
  support files into it, and the bundle includes the Python interpreter:
  https://pyinstaller.org/en/stable/operating-mode.html
* Ansible installation guide: "Windows without WSL is not natively
  supported as a control node", and the managed node "requires Python to
  run Ansible-generated Python code", with exceptions such as network
  modules:
  https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html
