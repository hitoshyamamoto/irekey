#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if a text file contains a non-ASCII letter: the repository is English
# only. Allowed: docs/spec/masterprompt.md and README.md (they explain the
# Japanese terms behind the name) and UTF-8 test fixtures (testdata files
# with utf8 in their name). Punctuation and symbols are not letters; a
# combining accent on an ASCII letter and invalid UTF-8 are reported too.
#
# Usage: scripts/check-language.sh [root]   (default root: the repository root)
# The rules live in scripts/guard (go run ./scripts/guard -h).
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "${1:-$repo}" && pwd)
cd "$repo"
exec go run ./scripts/guard language -root "$root"
