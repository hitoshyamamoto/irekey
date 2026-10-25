#!/bin/sh
# SPDX-License-Identifier: MIT
# Self-test of the guard scripts. Runs the guard unit tests; then runs each
# check-*.sh wrapper on a small tree that must pass and on one that must
# fail with findings; then checks that the executor size limit can fail.
# Values the guards reject are assembled from parts at run time, so that
# this file itself passes every guard.
#
# Usage: scripts/test-guards.sh
set -eu

cd "$(dirname "$0")/.."

status=0

echo "== go test ./scripts/guard/..."
if ! go test ./scripts/guard/...; then
	echo "error: the guard unit tests failed" >&2
	status=1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

# put FILE FORMAT [ARG...]: writes printf output to FILE, creating its
# directory.
put() {
	file=$1
	shift
	mkdir -p "$(dirname "$file")"
	# The format comes from this script; it carries octal escapes for
	# non-ASCII bytes.
	# shellcheck disable=SC2059
	printf "$@" >"$file"
}

spdx_go='// SPDX-License-Identifier: MIT\n\npackage p\n\n'

# spdx
t=$tmp/spdx
put "$t/good/main.go" "$spdx_go"'func main() {}\n'
put "$t/good/run.sh" '#!/bin/sh\n# SPDX-License-Identifier: MIT\necho ok\n'
put "$t/bad/main.go" 'package main\n'
put "$t/bad/run.sh" '#!/bin/sh\necho no license line\n'

# hostkey
t=$tmp/hostkey
put "$t/good/client.go" "$spdx_go"'func check(host string, remote net.Addr, key ssh.PublicKey) error {\n\treturn compare(key)\n}\n'
put "$t/bad/client.go" "$spdx_go"'var cfg = ssh.ClientConfig{HostKeyCallback: ssh.%s%s()}\n' Insecure IgnoreHostKey

# shell
t=$tmp/shell
put "$t/good/session.go" "$spdx_go"'func f() { _ = sess.Run(quote.Join(argv)) }\n'
put "$t/bad/session.go" "$spdx_go"'import "fmt"\n\nfunc f(dir string) { _ = sess.Run(fmt.Sprintf("ls %%s", dir)) }\n'

# realdata
t=$tmp/realdata
put "$t/good/notes.txt" 'host 192.0.2.10, mac 02:00:00:00:00:01, go 1.27.3, at 18:05:11\n'
put "$t/good/docs/guide.md" 'See https://example.com and https://github.com/x.\n'
put "$t/good/scripts/guard/allowed-domains.txt" 'github.com # hosting\n'
put "$t/bad/notes.txt" 'dns %s.%s, mac %s:%s\n' 8.8 4.4 00:1a:2b 3c:4d:5e
put "$t/bad/docs/guide.md" 'See https://%s.%s\n' globex com

# language
t=$tmp/language
put "$t/good/notes.md" 'irekey \342\200\224 inspect \342\206\222 deliver\n'
put "$t/good/README.md" 'abc\303\251\n'
put "$t/bad/notes.md" 'abc\303\251\n'

# gomod
t=$tmp/gomod
gomod='module example.com/m\n\ngo 1.27\n\nrequire example.com/dep v1.0.0\n'
put "$t/good/go.mod" "$gomod"
put "$t/good/scripts/guard/allowed-modules.txt" 'example.com/dep MIT # fixture dependency\n'
put "$t/bad/go.mod" "$gomod"'require example.com/other v1.0.0\nreplace example.com/sim => ./lab/sim\n'
put "$t/bad/scripts/guard/allowed-modules.txt" 'example.com/dep %s # copyleft\n' GPL-3.0-only

# check_guard NAME: the good tree must pass, and the bad tree must fail
# with findings (not with a usage or internal error).
check_guard() {
	good=passes
	bad=passes
	"scripts/check-$1.sh" "$tmp/$1/good" >"$tmp/$1.good.out" 2>&1 || good=fails
	"scripts/check-$1.sh" "$tmp/$1/bad" >"$tmp/$1.bad.out" 2>&1 || bad=fails
	if [ "$bad" = fails ] && ! grep -q "^error: $1 guard: " "$tmp/$1.bad.out"; then
		bad="fails without findings"
	fi
	if [ "$good" = passes ] && [ "$bad" = fails ]; then
		printf 'ok    %-10s good tree passes, bad tree fails\n' "$1"
		return
	fi
	printf 'FAIL  %-10s good tree %s, bad tree %s\n' "$1" "$good" "$bad"
	echo "error: check-$1.sh gave an unexpected result; its output:" >&2
	cat "$tmp/$1.good.out" "$tmp/$1.bad.out" >&2
	status=1
}

echo "== guard wrappers"
for name in spdx hostkey shell realdata language gomod; do
	check_guard "$name"
done

echo "== executor size limit"
if [ ! -d cmd/irekey-exec ]; then
	printf 'skip  %-10s cmd/irekey-exec does not exist yet\n' exec-deps
elif IREKEY_EXEC_MAX_BYTES=1 scripts/check-exec-deps.sh >"$tmp/exec.out" 2>&1; then
	printf 'FAIL  %-10s passes with a 1-byte limit\n' exec-deps
	cat "$tmp/exec.out" >&2
	status=1
elif grep -q 'above the limit' "$tmp/exec.out"; then
	printf 'ok    %-10s fails with a 1-byte limit\n' exec-deps
else
	printf 'FAIL  %-10s fails, but not on the size limit\n' exec-deps
	cat "$tmp/exec.out" >&2
	status=1
fi

exit "$status"
