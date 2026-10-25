# CLAUDE.md

Guidance for Claude Code when working in this repository. The single source
of truth is the specification, `docs/spec/masterprompt.md`; this file only
summarizes the rules that apply to every change and points into it.

## What irekey is

irekey manages the same software running on many offline embedded Linux
computers, where every customer needs small local differences, and delivers
changes over SSH with automatic rollback. Two binaries: `irekey` (operator
machine) and `irekey-exec` (device-side executor). Read spec sections 1-7
before changing behavior.

## Working rules (spec 0.3)

1. Work in the order of spec section 23; one coherent, reviewable change per
   pull request, with tests.
2. Decided items (spec 0.2) are not changed without asking the maintainer.
   Ask before: changing a compatibility contract (spec 22), adding a
   network listener, adding anything that runs permanently on a device, or
   adding a dependency with a non-permissive license.
3. Every safety invariant (spec 7) has at least one test that fails when it
   is broken. Invariants are referred to by their descriptive names, never
   by their IDs, outside the spec (ADR 0021): tests are `Test<Name>_<case>`
   (for example `TestDataSeparation_WriterRedactsAcrossChunkBoundaries`),
   comments read `// Safety (DataSeparation): ...`.
4. Never invent APIs, flags, config keys, systemd directives or library
   behavior. Check with `go doc`, the module source or official docs. When
   something cannot be checked, write `// Verify: <what> (<external source>)`
   and list it in the pull request under "Not verified".
5. Dependencies are a decision: standard library first. A new module needs a
   justification (purpose, alternatives, license, maintenance status) in the
   commit message and the PR, an entry in `scripts/guard/allowed-modules.txt`,
   and must pass the license check.
6. English only in code, identifiers, comments, docs, CLI output, logs, test
   names, fixtures, commits and pull requests. Japanese railway terms are
   allowed only in the spec and README.md (`scripts/check-language.sh`).
7. No real-world data: IPs only from 192.0.2.0/24, 198.51.100.0/24,
   203.0.113.0/24, 2001:db8::/32 and loopback; MACs only locally
   administered (02:00:00:xx:xx:xx); domains only example.com, .net and
   .org, the reserved `.example`, `.test` and `.invalid` names, and the tool
   and hosting domains listed in `scripts/guard/allowed-domains.txt`; names
   only acme, globex, initech. No employer, product or customer names
   (`scripts/check-realdata.sh`). Test fixtures that must contain bad data
   build it at run time, so the repository stays clean under its guards.
8. Use the vocabulary of spec section 5 exactly. Forbidden synonyms: "fleet
   repository", "client" (for customer), "equipment", "plant", "agent" (for
   the executor), "deploy key", "backend" (for a provider), "tag" (for
   anything but a git tag).
9. When a decision changes, update the spec and add an ADR in `docs/adr/`
   in the same pull request.

## Commits and pull requests

- Conventional Commits for commit titles and pull request titles (pull
  requests are squash-merged; the title becomes the commit title and the
  description becomes the commit body). First word lowercase; proper nouns
  and acronyms keep their case; at most 72 characters.
- Sign off every commit (`git commit -s`). The pull request description ends
  with the author's `Signed-off-by:` line as its last paragraph.
- Do not add AI attribution: no `Co-Authored-By` trailer for an assistant
  and no "Generated with" line.
- Never write the specification's internal labels in commits, pull requests
  or public files: no milestone labels, invariant IDs, goal, requirement,
  scenario, gate or question IDs, and no "Section N.N" references. Describe
  the change in plain words. ADR numbers and error codes (`ENG-002`) are
  project identifiers and are fine. The two exceptions are the
  specification itself and this file, which points into the
  specification's sections so they can be found.
- Write in depth and explicitly: why, what (every file), decisions with the
  alternatives considered, dependencies, how it was tested. State only
  results that were actually observed; anything not verified goes under
  "Not verified" with the reason. Follow `.github/PULL_REQUEST_TEMPLATE.md`.

## Commands

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
go test ./internal/schema -update                # rewrite docs/schema-versions.txt, then review the diff
go generate ./internal/errs                      # after editing internal/errs/codes.yaml
go generate ./internal/execbin                   # build the executors to embed (gitignored; optional for development)
```

`-race` needs cgo and a C compiler. CI always runs it; if the local machine
has no C compiler, run the tests without `-race` and say so in the PR.
Packages with golden files define the same `-update` flag; run it for the
package that changed and review the diff before committing.

## Guards (scripts/)

`check-spdx.sh` (SPDX header on every .go and .sh file),
`check-hostkey.sh` (no `InsecureIgnoreHostKey`, no unconditional host-key
callback), `check-shell.sh` (no remote command built by string formatting;
the only exception will be the shared quoting function, planned as
`internal/transport/ssh/quote.go`), `check-realdata.sh`,
`check-language.sh`, `check-gomod.sh` (every module listed with an allowed
license; no GPL), `check-exec-deps.sh` (executor dependencies and size),
`check-errors.sh` (error codes: registered, used through their generated
constants, reserved until produced, active ones used by code and referred
to by a test; generated files fresh; ADR 0019).
`scripts/test-guards.sh` runs every guard against a tree that must pass and
a tree that must fail; `check-exec-deps.sh` is checked with a forced
size-limit failure.

## Layout and dependency rule (spec 8.3, 8.4)

- `cmd/irekey` -> `internal/cli` -> `internal/app` -> domain packages;
  `internal/ui` -> `internal/app`; nothing else imports `internal/cli`.
- Pure packages (no I/O): model, layers, variant, graph, schema, errs,
  redact. They import no `os`, `net` (except `net/netip`), `os/exec` or
  `syscall`.
- `cmd/irekey-exec` never imports operator-side packages, never listens on a
  port and opens no connection to other hosts (whether local health checks
  may reach the device's own services on loopback is an open question).
  Target size < 8 MiB stripped (linux/amd64); CI fails above 12 MiB.
- `internal/archtest` enforces the import rules with `go list`;
  `scripts/check-exec-deps.sh` enforces the executor's dependencies and
  size. That the executor never listens or connects is checked in review.

## Pointers into the spec

| Topic | Section |
|---|---|
| Invariants and their names | 7 |
| Architecture and layout | 8 |
| Device layout, executor, guard, journal | 11 |
| CLI conventions and exit codes | 15 |
| Error model and categories | 16, Appendix B |
| Security controls and documents | 17 |
| Testing strategy | 20 |
| Engineering standard, CI, release | 21 |
| Compatibility contract | 22 |
| Order of work and acceptance | 23 |
| Definition of Done | Appendix D |
