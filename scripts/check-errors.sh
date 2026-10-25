#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if the error-code registry (internal/errs/codes.yaml) is invalid or
# its generated files (internal/errs/zz_generated_codes.go, docs/errors.md)
# are stale; if non-test Go code outside internal/errs writes an error code
# as a string literal (or a constant concatenation of literals) instead of
# its generated constant, uses a code constant that is not registered or a
# code that is still reserved, or imports internal/errs with a dot; if an
# active code is not used by non-test code outside internal/errs, or no test
# outside internal/errs refers to its constant; or if a Markdown file (except
# the specification, docs/errors.md and test data) mentions a code with a
# registered prefix that is not registered.
#
# Usage: scripts/check-errors.sh [root]   (default root: the repository root)
# The rules live in internal/errs/internal/gen
# (go run ./internal/errs/internal/gen/errsgen -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./internal/errs/internal/gen/errsgen -check "$root"
