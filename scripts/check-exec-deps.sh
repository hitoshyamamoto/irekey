#!/bin/sh
# SPDX-License-Identifier: MIT
# Fails if the device executor (cmd/irekey-exec) depends on an operator-side
# package of this module or on a package outside the standard library and
# this module (on linux/amd64, linux/arm64 or linux/arm/v7), or if its
# stripped linux/amd64 binary exceeds the size limit.
# The executor runs as root on devices: it carries only device-side code and
# stays small.
#
# Usage: scripts/check-exec-deps.sh
# IREKEY_EXEC_MAX_BYTES overrides the size limit (default 12 MiB).
set -eu

cd "$(dirname "$0")/.."

module=github.com/hitoshyamamoto/irekey
pkg=./cmd/irekey-exec
max=${IREKEY_EXEC_MAX_BYTES:-12582912} # 12 MiB: hard limit
target=8388608                         # 8 MiB: size target, warning above

# Operator-side packages under internal/ (with their subpackages).
operator_side="app cli ui vault forge state deployrepo execbin transport transfer rollout"

# Modules outside the standard library that the executor may use, separated
# by spaces. Empty on purpose: adding one is a decision.
allowed_external=""

status=0

# The executor runs on Linux devices, so its dependencies are listed for
# every release target: build constraints can pull in different packages
# per architecture.
deps=""
for platform in amd64 arm64 arm/7; do
	arch=${platform%/*}
	arm=""
	case $platform in
	*/*) arm=${platform#*/} ;;
	esac
	if ! more=$(env CGO_ENABLED=0 GOOS=linux GOARCH="$arch" ${arm:+GOARM="$arm"} go list -deps "$pkg"); then
		echo "error: cannot list the dependencies of $pkg for linux/$platform" >&2
		exit 1
	fi
	deps="$deps $more"
done
# One package path per line; the list was built with spaces as separators.
deps=$(printf '%s\n' "$deps" | tr ' ' '\n' | sed '/^$/d' | sort -u)

for dep in $deps; do
	for name in $operator_side; do
		case $dep in
		"$module/internal/$name" | "$module/internal/$name/"*)
			echo "error: $pkg depends on operator-side package $dep" >&2
			status=1
			;;
		esac
	done

	# A standard library path has no dot in its first element.
	case ${dep%%/*} in
	*.*) ;;
	*) continue ;;
	esac
	case $dep in
	"$module" | "$module/"*) continue ;;
	esac
	allowed=no
	for mod in $allowed_external; do
		case $dep in
		"$mod" | "$mod/"*) allowed=yes ;;
		esac
	done
	if [ "$allowed" = no ]; then
		echo "error: $pkg depends on $dep, outside the standard library and this module" >&2
		status=1
	fi
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

if CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -ldflags "-s -w" -o "$tmp/irekey-exec" "$pkg"; then
	size=$(wc -c <"$tmp/irekey-exec" | tr -d ' ')
	echo "irekey-exec (linux/amd64, stripped): $size bytes (limit $max, target $target)"
	if [ "$size" -gt "$max" ]; then
		echo "error: irekey-exec is $size bytes, above the limit of $max bytes" >&2
		status=1
	elif [ "$size" -gt "$target" ]; then
		echo "::warning::irekey-exec is $size bytes, above the $target-byte target"
	fi
else
	echo "error: cannot build $pkg" >&2
	status=1
fi

exit "$status"
