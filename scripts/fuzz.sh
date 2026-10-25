#!/bin/sh
# SPDX-License-Identifier: MIT
# Runs every native Go fuzz target of the module, one at a time, each for
# the given duration. A target is a "func FuzzXxx(" in a *_test.go file
# that git tracks or would track (not ignored). Fails if any run fails;
# go test writes the failing input under the package's testdata/fuzz/.
#
# Usage: scripts/fuzz.sh <duration>   (go test -fuzztime syntax: 30s, 5m)
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <duration>" >&2
	exit 2
fi
duration=$1

cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

if ! git ls-files --cached --others --exclude-standard -- '*_test.go' >"$tmp/files"; then
	echo "error: cannot list the test files with git" >&2
	exit 2
fi

# One "<package dir> <target>" line per fuzz target.
sort -u "$tmp/files" |
	while IFS= read -r file; do
		[ -f "$file" ] || continue
		dir=$(dirname "$file")
		grep -oE '^func Fuzz[A-Za-z0-9_]*\(' "$file" | sed 's/^func //; s/($//' |
			while IFS= read -r name; do
				echo "$dir $name"
			done
	done >"$tmp/targets"

if [ ! -s "$tmp/targets" ]; then
	echo "no fuzz targets found"
	exit 0
fi

status=0
while read -r dir name; do
	echo "== fuzz ./$dir $name ($duration)"
	if ! go test "./$dir" -run '^$' -fuzz "^$name\$" -fuzztime "$duration" </dev/null; then
		echo "error: fuzz target $name in ./$dir failed" >&2
		status=1
	fi
done <"$tmp/targets"

exit "$status"
