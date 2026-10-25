#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if non-test Go code assembles a shell command from strings outside
# internal/transport/ssh/quote.go: fmt.Sprintf, fmt.Sprint, strings.Join,
# "+", "+=" or a strings.Builder passed to Run, Start, Output or
# CombinedOutput (directly or through a local variable), or os/exec starting
# a shell with -c (or -lc, -ec, ...) and a computed script. Remote commands
# are built from an argument vector and quoted by one tested function.
#
# Usage: scripts/check-shell.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard shell -root "$root"
