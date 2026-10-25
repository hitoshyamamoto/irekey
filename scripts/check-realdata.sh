#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if a file contains real-world network data: an IPv4 or IPv6 address
# outside the documentation and loopback ranges or a globally administered
# MAC address (any text file), or a host name, under any top-level domain
# of the IANA root zone or an intranet suffix such as .local or .corp, other
# than the reserved example names and scripts/guard/allowed-domains.txt
# (docs, examples, lab, schemas, test data, root Markdown files and GitHub
# templates).
#
# Usage: scripts/check-realdata.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard realdata -root "$root"
