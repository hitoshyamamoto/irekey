---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Write repository files in YAML and validate them with JSON Schema

## Context and Problem Statement

Developers edit the deployment repository by hand (device files, layers,
release sets, groups, rollout plans) and review it in merge requests;
irekey writes device files at adoption and triage records after triage.
Each component repository adds `.irekey/component.yaml` and a schema for
the component's configuration keys. Every file carries
`schema: irekey/<kind>/v<N>`, so a newer irekey can read and migrate
older files and an older irekey refuses newer ones.

These files decide what reaches a device. A misspelled key that is
silently ignored can drop a health check or a data path, and nothing
shows it until a field update goes wrong.

Which format should hand-edited files use, how are they validated, and
with which libraries?

## Decision Drivers

* People write and review the files: they need comments (why a
  customer's value differs) and diffs that read well.
* Every error is found before delivery, by the CLI and by `irekey ci`,
  with file, line and key in the message; an unknown key is an error.
* The formats are a versioned contract; their definitions should be
  publishable for editors and other tools, and component configuration
  schemas should use the same mechanism.
* Libraries are pure Go (`CGO_ENABLED=0`), maintained, and under an
  allowed license.

## Considered Options

* YAML validated with JSON Schema
* YAML decoded strictly into Go types
* JSON with JSON Schema
* TOML
* CUE

## Decision Outcome

Chosen option: "YAML validated with JSON Schema", because it is the only
option that combines comments and readable diffs with a standard,
publishable schema language that also serves component configuration.

* Every hand-edited file kind has a JSON Schema, embedded in the binary
  (`internal/schema`) and published as generated copies in `schemas/`;
  `docs/schema-versions.txt` lists them and is kept current by a test.
* A file is validated against the schema named by its `schema` key
  (failure: `REPO-001`, with file, line and key path), then decoded into
  its Go type by a decoder that rejects unknown fields
  (`KnownFields(true)`), which catches a schema and a Go type that have
  drifted apart.
* Objects with fixed keys allow no additional properties; maps with free
  keys, such as device labels, constrain their values instead.
* Libraries: `go.yaml.in/yaml/v3`, the YAML organization's maintained
  continuation of the archived `gopkg.in/yaml.v3` (v4 is a release
  candidate and is not used before its release), and
  `github.com/santhosh-tekuri/jsonschema/v6`.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once the deployment-repository
loader and its first schemas (device, customer, site, component) are
implemented.

### Consequences

* Good, because errors are reported before any connection, at a location
  a developer can act on, and editors can check files with the
  published schemas while they are written.
* Good, because one validator serves irekey's file kinds and every
  component's configuration schema.
* Bad, because each file kind is described twice, as schema and Go type;
  strict decoding and tests over the example files keep them in line.
* Bad, because the v3 API is frozen with security fixes only; moving to
  v4 later is a reviewed dependency change.

### Confirmation

Tests: every file in `examples/deployment-repo` validates; a misspelled
key in each file kind fails with `REPO-001`; the YAML loaders are fuzzed.
Review confirms that irekey's code never imports `gopkg.in/yaml.v3`.

Evidence that would change the decision: errors cannot be mapped back to
YAML lines usefully; a file kind needs a rule JSON Schema cannot express
(cross-file checks belong to the graph); loading misses the scale target
(planning for 500 devices in under 2 s on a laptop); or the v3 line
stops receiving security fixes.

## Pros and Cons of the Options

### YAML validated with JSON Schema

* Good, because YAML allows comments and is familiar from Compose files
  and CI pipelines, and JSON Schema is a standard with outside tooling.
* Bad, because YAML has pitfalls (indentation, values that look like
  another type); typed schemas turn them into errors, not guesses.

### YAML decoded strictly into Go types

* Good, because each file kind is described once.
* Bad, because there is no published schema, value rules (slugs, ranges,
  enumerations) become hand-written Go, and component configuration
  still needs a schema language.

### JSON with JSON Schema

* Good, because JSON has one simple data model in the standard library.
* Bad, because it has no comments, and hand-edited diffs are noisy.

### TOML

* Good, because TOML allows comments and has simple typing.
* Bad, because deep nesting (components, health checks, waves) is
  awkward, and it still needs a separate schema language.

### CUE

* Good, because types, constraints and defaults live in one language.
* Bad, because every developer editing the deployment repository would
  have to learn it, and CUE's unification rejects conflicting concrete
  values while irekey's layers are ordered overrides.

## More Information

Facts checked on 2026-10-25 in the primary sources:

* `go.yaml.in/yaml/v3` v3.0.5 (2026-07-26): its license file puts the
  files ported from libyaml under MIT and all remaining files under
  Apache-2.0, and the repository has a `NOTICE` file
  (https://github.com/yaml/go-yaml/blob/v3.0.5/LICENSE). The v3.0.5
  README says the YAML organization took over maintenance after
  `go-yaml/yaml` was labeled unmaintained. The README on the main branch
  describes v3 as "Stable, frozen API; security fixes only" and says
  v4.0.0 "is not released yet"; the newest v4 version is
  v4.0.0-rc.6 (2026-06-17): https://github.com/yaml/go-yaml
* `gopkg.in/yaml.v3` is archived; its README reads "THIS PROJECT IS
  UNMAINTAINED": https://github.com/go-yaml/yaml
* `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3 (2026-08-06),
  Apache-2.0, actively maintained:
  https://github.com/santhosh-tekuri/jsonschema
* CUE, "Unification": "For concrete data, unification of basic types
  requires that all values specified for a field must be equal":
  https://cuelang.org/docs/tour/basics/unification/

The specification's "Deployment repository format" and "Compatibility
contract" sections define the files, their versioning and the promise.
