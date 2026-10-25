<!--
Pull requests are squash-merged: the PR title becomes the commit title and this
description becomes the commit body. Write the title as a Conventional Commit
(feat, fix, docs, ci, build, chore, refactor, test, perf). Be explicit; state
only results that were actually observed.
-->

## What

### Summary

<!-- What this pull request delivers and why, in one paragraph. -->

### Changes

<!-- By area (code, CI, docs, ...): every file or directory changed, what it is and why. -->

### Decisions

<!-- Each non-obvious choice: the alternatives considered and why they were not taken. Link ADRs. -->

### Dependencies

<!-- "None", or a table: module | version | purpose | alternatives | license | maintenance status. -->

## How it was tested

<!-- The exact commands that were run and their real results. Say explicitly what was not tested, and why. -->

## Not verified

<!-- Every `// Verify:` note added (file:line) and every fact that could not be confirmed, with the reason. -->

## Out of scope

<!-- What was deliberately left out of this pull request. -->

## Checklist

- [ ] `go vet ./...`, `go test -race ./...` and `golangci-lint run ./...` pass
- [ ] Every `scripts/check-*.sh` guard passes
- [ ] Tests cover the change; bug fixes include a regression test; safety tests are untouched or strengthened
- [ ] New error paths use registered, documented and tested error codes
- [ ] `--json` output changes updated the golden files
- [ ] Docs and ADRs updated when behavior or a decision changed
- [ ] No real-world data, English only, SPDX header on new source files
- [ ] New dependencies justified (purpose, alternatives, license, maintenance)
- [ ] `CHANGELOG.md` updated for user-visible changes
- [ ] Every commit is signed off (`git commit -s`)

<!--
End the description with your own Developer Certificate of Origin sign-off as
the last paragraph, so the squashed commit carries it:
Signed-off-by: Your Name <your.address@example.com>
-->

