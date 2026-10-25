#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if go.mod requires a module (direct or indirect), or replaces one
# with a module, that is not listed in scripts/guard/allowed-modules.txt
# with an allowed license (MIT, BSD, Apache-2.0, ISC, MPL-2.0), or replaces
# a module with a path inside lab/. go.mod is read with go mod edit -json.
# This is how GPL code, such as the lab simulators, never becomes a Go
# dependency.
#
# Usage: scripts/check-gomod.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard gomod -root "$root"
