# Changelog

All notable changes to irekey are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/). Each version uses the sections
Added, Changed, Deprecated, Removed, Fixed and Security, as needed.

## [Unreleased]

### Added

- The project specification (`docs/spec/masterprompt.md`), the MIT license
  and the repository bootstrap files.
- Architecture decision records in `docs/adr/`: the template, an index,
  ADR 0001 (record architecture decisions), ADR 0020 (project name) and
  ADR 0021 (descriptive invariant names).

### Changed

- The project is named irekey. The specification used the working name
  irekeys; every name derived from it (module path, binaries, device
  paths, schema identifiers, signature namespace, image names) now uses
  irekey. See ADR 0020.
- Safety invariants are referred to by descriptive names (for example
  `DataSeparation`) in code, tests and public documentation, instead of
  by their numeric IDs. See ADR 0021.

## Ideas on record

Suggestions outside the current scope, kept here so they are not lost.
Each one needs a real field case before it is considered.
