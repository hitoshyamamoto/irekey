# Embedded executors (generated)

This directory holds the device-side executors that package `execbin`
embeds. Except for this README, its content is generated and ignored by
git, so a fresh clone has only this file here; the package then reports
that no executor is embedded (a development build). The operator command
line does not use the package yet.

To fill it, run from the repository root:

```sh
go generate ./internal/execbin
```

This builds `./cmd/irekey-exec` for linux/amd64, linux/arm64 and
linux/arm/v7 (version `dev`) and writes:

- `irekey-exec-linux-amd64.gz`, `irekey-exec-linux-arm64.gz` and
  `irekey-exec-linux-armv7.gz`: the executors, gzip-compressed with a fixed
  header (no file name, no modification time);
- `manifest.json`: the version and, for each platform, the SHA-256 digest
  and size of the uncompressed executor and of its compressed file.

The executors are built without cgo, with `-trimpath` and a stripped symbol
table, and with a fixed set of go command settings: the caller's `GOFLAGS`,
`GOEXPERIMENT`, `GOFIPS140` and go env file are ignored. The toolchain is
not pinned: a `GOTOOLCHAIN` in the go env file is ignored with the rest of
that file, a `GOTOOLCHAIN` in the environment is honored, and otherwise the
go command selects the toolchain from `go.mod` as usual (it switches to the
`toolchain` line when it is older). The same commit in a clean checkout,
built with the same Go toolchain, gives the same executors (Go records the
commit and whether the tree was modified in each binary); compressing
identical executor bytes gives identical files.

The uncompressed digest is the one that identifies an executor: the value a
release's `SHA256SUMS` will list for `irekey-exec-linux-<arch>`.

The generator fails if the linux/amd64 executor is larger than 12 MiB and
warns above 8 MiB. Its source is `internal/execbin/internal/gen`; run it
with `-h` for its options.
