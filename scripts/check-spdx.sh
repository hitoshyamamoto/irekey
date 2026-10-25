#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if a Go file does not start with the line
# "// SPDX-License-Identifier: MIT", or a shell script does not have it
# (as "# SPDX-License-Identifier: MIT") on line 2, after the shebang.
#
# Usage: scripts/check-spdx.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard spdx -root "$root"
