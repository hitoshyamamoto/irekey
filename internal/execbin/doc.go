// SPDX-License-Identifier: MIT

// Package execbin embeds the device-side executor, irekey-exec, built for
// Linux on amd64, arm64 and armv7. Each executor is stored gzip-compressed,
// next to a manifest that records, per platform, the SHA-256 digest and the
// size of the executor and of its compressed file. The package loads the
// manifest and verifies the executors against it. The operator command line
// does not use it yet; it will, to report the embedded executors and to
// deliver them to devices.
//
// The files are generated and not tracked by git: go generate
// ./internal/execbin builds them from cmd/irekey-exec into bin/ (see
// bin/README.md). A tree where they were not generated still builds and
// tests; Embedded then reports that no executor is embedded, as in a
// development build.
//
// The digest that identifies an executor is the SHA-256 of the uncompressed
// binary, the value a release's SHA256SUMS will list for the asset
// irekey-exec-linux-<arch>. The digest of the compressed file is recorded as
// well, so that the file can be checked before it is decompressed.
package execbin
