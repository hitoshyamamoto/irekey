#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if non-test Go code disables SSH host-key verification: a reference
# to the x/crypto/ssh helper that ignores host keys, or a host-key callback
# that can only return nil (logging or dropping a check's result does not
# count as verifying). irekey only talks to a device that presents its
# pinned host key.
#
# Usage: scripts/check-hostkey.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard hostkey -root "$root"
