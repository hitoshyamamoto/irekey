# Contributing to irekey

Thank you for helping. irekey is in early development: there is no release
yet, and the specification in
[docs/spec/masterprompt.md](docs/spec/masterprompt.md) describes what is
being built. Read [GOVERNANCE.md](GOVERNANCE.md) before proposing a
feature: proposals need a real field case and must fit the project's verb
(inspect, adjust, deliver). For anything larger than a small fix, open an
issue first, so the scope is agreed before the code is written.

## Development setup

- Go. The `toolchain go1.27.3` line in `go.mod` makes any Go 1.21 or later
  download and use go1.27.3 automatically, unless `GOTOOLCHAIN` is set to
  `local`. Inside the repository, `go version` then prints go1.27.3.
- git.
- A C compiler, such as gcc or clang, only for `go test -race`: the race
  detector needs cgo. Everything else builds with `CGO_ENABLED=0`.
- [golangci-lint](https://golangci-lint.run/) v2.14.0, the version CI uses.
- Optional: `shellcheck`. When it is on `PATH`, actionlint also checks the
  shell scripts inside workflow `run:` blocks, as it does in CI.

Try the two binaries:

```sh
go run ./cmd/irekey version
go run ./cmd/irekey-exec capabilities
```

## Before opening a pull request

Run the same checks as CI, from the repository root:

```sh
gofmt -l .                      # must print nothing
go vet ./...
go test -race -coverpkg=./cmd/...,./internal/... -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1          # product code total must stay >= 80%
golangci-lint run ./...                          # v2.14.0, config in .golangci.yml
scripts/test-guards.sh                           # self-test of the guards
for s in scripts/check-*.sh; do sh "$s" || echo "FAIL $s"; done
scripts/fuzz.sh 30s                              # every native fuzz target
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12   # after editing workflows
```

If the machine has no C compiler, run the tests without `-race` and say
so in the pull request; CI always runs them with it. The coverage floor
measures the product code, the packages under `cmd/` and `internal/`: CI
fails when their total statement coverage is below 80%. The guard
program in `scripts/guard` is tested in the same run, but its statements
are not counted, so it cannot hold up the product's coverage.

CI also checks the licenses of every dependency, for each operating
system that irekey is built for. To run the same check locally, install
go-licenses (it lands in `$(go env GOPATH)/bin`, which must be on `PATH`):

```sh
go install github.com/google/go-licenses/v2@v2.0.1
for os in linux darwin windows; do
  GOROOT=$(go env GOROOT) GOOS=$os go-licenses check ./... \
    --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,MPL-2.0
done
```

go-licenses must see the same GOROOT as the toolchain that `go.mod`
selects. It tells standard library packages apart by the GOROOT it was
built with. When another Go built it, for example an older Go when it was
installed from outside the repository, it reports every standard library
package as having no module info and fails. Run inside the repository,
`go env GOROOT` prints the GOROOT of go1.27.3, and passing it explicitly
makes the check work whichever Go built go-licenses; that is why CI
passes `GOROOT=$(go env GOROOT)`.

The remaining CI jobs need no local step. The `build` job cross-compiles
the executor for Linux (amd64, arm64, armv7) and the command line for
Linux, macOS and Windows (amd64, arm64) with `CGO_ENABLED=0`, then runs
`irekey version` and `irekey-exec capabilities` with an empty home
directory. The `dco` job checks the sign-offs (see below), and CodeQL
analyzes the code. The `integration` and `e2e-smoke` jobs are
placeholders that pass until those tests exist, zizmor analyzes the
workflows when a pull request changes them, and OpenSSF Scorecard runs
after every merge.

A pull request can be merged only when the required checks `lint`,
`test`, `build`, `dco`, `codeql`, `licenses` and `guards` pass on a
branch that is up to date with `main`; nobody can bypass them, the
maintainer included (see [GOVERNANCE.md](GOVERNANCE.md#decision-making)).

## Golden files

Some tests compare output with golden files; today that is
`docs/schema-versions.txt`, checked by the tests of `internal/schema`.
After an intended change, rewrite the golden files with the `-update`
flag of the package's tests, then review the diff before committing:

```sh
go test ./internal/schema -update
git diff
```

Pass the package path. The flag exists only in packages that have golden
files, and `go test ./... -update` fails in every other package that has
tests. A change of `--json` output always updates its golden files and is
listed in the changelog.

## Coding style

Go code follows [Effective Go](https://go.dev/doc/effective_go) and the
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments). It must
be formatted with `gofmt` and pass `golangci-lint` with the repository's
[.golangci.yml](.golangci.yml); CI enforces both. In addition:

- Errors are wrapped with `%w`. Typed errors carry an error code (such as
  `ENG-002`) from the registry in `internal/errs`, which is designed but
  not implemented yet. A failure message says what failed, its error
  code, the state the device was left in, and the next command to run.
  No `panic` outside `main`.
- Anything that does I/O takes a `context.Context` as its first
  parameter, and every network or process call has a timeout.
- Logging uses `log/slog`: structured, leveled and redacted. Human output
  is not a log.
- No `unsafe` and no cgo.
- Every `.go` file starts with `// SPDX-License-Identifier: MIT`, and
  every shell script has `# SPDX-License-Identifier: MIT` on its second
  line, after the shebang (`scripts/check-spdx.sh`).
- Never invent an API, flag, configuration key or library behavior. Check
  it with `go doc`, the module source or the official documentation. When
  that is not possible, write `// Verify: <what> (<where>)` next to the
  code and list it in the pull request under "Not verified".

Exceptions to a linter are rare and explained in a comment next to them.

## Developer Certificate of Origin

Every commit must be signed off, certifying that you wrote it or otherwise
have the right to submit it under the project's license, as stated in the
[Developer Certificate of Origin 1.1](https://developercertificate.org/).
Add the sign-off with `git commit -s`; it appends a line such as:

```
Signed-off-by: Your Name <you@example.com>
```

The `dco` check in CI rejects a pull request when one of its commits has
no sign-off. Pull requests are squash-merged and the description becomes
the body of the squashed commit, so the pull request description also
ends with your own `Signed-off-by:` line as its last paragraph.

## Rules

- **Tests are required.** Every change comes with tests in the same pull
  request, and a bug fix comes with a regression test that fails without
  the fix. Every safety invariant gets tests named after it,
  `Test<Name>_<behavior>` (for example
  `TestDataSeparation_WriterRedactsAcrossChunkBoundaries`), written with
  the code that relies on it; a change may strengthen those tests, never
  weaken them. Tests run without hardware and without internet access.
- **English only.** Code, identifiers, comments, commit messages, CLI
  output, logs, documentation, test names and fixtures are in English,
  written with ASCII letters (`scripts/check-language.sh`). The only
  exceptions are the Japanese terms behind the project's name, in the
  README and the specification, and test fixtures for UTF-8 handling
  (files under a `testdata` directory with `utf8` in their name).
- **No real-world data.** No real customer, site, employer or product
  names, IP or MAC addresses, host keys or credentials.
  `scripts/check-realdata.sh` rejects IP and MAC addresses and domains
  outside the allowed ranges and lists; names, host keys and credentials
  are checked in review. Use the documentation ranges
  `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24` and `2001:db8::/32`,
  loopback addresses, locally administered MAC addresses
  (`02:00:00:xx:xx:xx`), the domain `example.com`, and the fictional names
  `acme`, `globex` and `initech`. In documentation, examples, lab files,
  schemas and test data, any other domain must belong to a tool, module,
  standard or hosting service listed in
  `scripts/guard/allowed-domains.txt`. A test that needs bad data builds
  it at run time, so the repository itself stays clean under its guards.
- **Dependencies are a decision.** Prefer the standard library. A new
  module needs a justification paragraph in the commit message and in the
  pull request: why it is needed, the alternatives considered, its
  license and its maintenance status. It also needs an entry in
  `scripts/guard/allowed-modules.txt`, and must pass the license checks.
  Allowed licenses are MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC
  and MPL-2.0; MPL-2.0 is file-level copyleft and needs a note in an
  architecture decision record. Any other license needs a decision record
  first.
- **Commits and pull requests.** Commit titles and pull request titles
  follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat(adopt): ...`, `fix(exec): ...`, `docs: ...`), with the first word
  in lowercase and at most 72 characters. The pull request title becomes
  the commit title. Write the description explicitly and in detail,
  following the pull request template: what changed and why, by file;
  each decision with the alternatives considered; dependencies; how it
  was tested, with the real results of the commands you ran; what was not
  verified, and why; and what is out of scope. State only results you
  actually observed.
- **CI actions** are pinned to a full commit SHA with a version comment
  (`uses: actions/checkout@<sha> # v7.0.1`). The repository enforces SHA
  pinning: a workflow that references an action by a tag or a branch does
  not run. Run actionlint after editing a workflow.
- **Changelog.** Every user-visible change adds an entry under
  `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), in the section that fits
  (Added, Changed, Deprecated, Removed, Fixed, Security).
- **Decisions.** A change to the design, or to anything the specification
  records as decided, needs an architecture decision record in
  [docs/adr/](docs/adr/) and the matching update of the specification, both
  in the same pull request. Discuss it in an issue first.

## Releasing (maintainers)

There is no release yet. The release workflow is added in a later change,
and this section will then describe the exact steps. Releases will follow
a curated changelog and maintainer-signed tags: each version tag will be
created with `git tag -s`, and anyone will be able to verify it with
`git tag -v` against the SSH key listed in
[.github/allowed_signers](.github/allowed_signers).

## Reproducing a release

Release builds are meant to be reproducible: anyone will be able to
rebuild a release from its signed tag and get the published checksums.
The exact commands depend on the release workflow, which is added in a
later change; this section will list them then.

## Security issues

Do not open a public issue. Report vulnerabilities privately, as described
in [SECURITY.md](SECURITY.md).
