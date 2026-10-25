---
status: accepted
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Record architecture decisions as MADR files in docs/adr

## Context and Problem Statement

irekey is designed up front in a written specification
(`docs/spec/masterprompt.md`) and is maintained by one person. Many design
points are chosen before any code exists, and some of them will be
revisited. When a choice is questioned later, by a contributor, a reviewer
or the maintainer, the reasons and the alternatives that were rejected must
be easy to find. Commit messages and pull request threads record what
changed, but not in a form that can be listed, searched and superseded.

How should the project record its architecture decisions?

## Decision Drivers

* A decision and its reasons must be readable without access to anyone's
  memory or private notes (the project has a bus factor of one).
* Records must live next to the code and change through the same reviewed
  pull requests, so they never drift from what was actually decided.
* The format must be light enough that writing a record is not a reason to
  skip it.
* A record that is replaced must stay readable, with a pointer to what
  replaced it.

## Considered Options

* MADR files in docs/adr
* Nygard's original format
* Decisions only in issues and pull requests

## Decision Outcome

Chosen option: "MADR files in docs/adr", because it keeps decisions in
the repository, under review, in a structure that forces the alternatives
and consequences to be written down, while every section beyond the
mandatory ones is optional.

Conventions:

* One file per decision, named `NNNN-title-with-dashes.md`, numbered in
  sequence. `0000-template.md` is the template.
* The status is one of `proposed`, `accepted`, `rejected`, `deprecated`
  or `superseded by ADR-NNNN`. A record is never deleted; a replaced
  decision gets a new record and the old one is marked as superseded.
* When a decision changes the specification, the specification and the
  record change in the same pull request.
* `docs/adr/README.md` lists every record with its status.

### Consequences

* Good, because the reasons behind each decision are reviewable and stay
  with the code.
* Good, because rejected alternatives are written down, so the same
  discussion does not have to start from zero.
* Bad, because every significant decision costs a short document; this is
  accepted as the price of a reviewable design.

### Confirmation

Pull request review: a change that takes or reverses a design decision
without a record, or without updating the specification, is not merged.

## Pros and Cons of the Options

### MADR files in docs/adr

MADR (Markdown Architectural Decision Records) is a template with
mandatory and optional sections.

* Good, because the template asks for drivers, options and consequences,
  which are the parts most often missing later.
* Good, because it is plain Markdown with optional YAML front matter; no
  tool is needed to read or write it.
* Neutral, because MADR suggests `docs/decisions/`; this project uses
  `docs/adr/`, the directory its specification defines.

### Nygard's original format

Michael Nygard's original form of an architecture decision record, without
a fixed template.

* Good, because it is the original, minimal form (context, decision,
  status, consequences).
* Bad, because without a template the considered options are easily left
  out.

### Decisions only in issues and pull requests

No separate records; decisions live only in issue and pull request
discussions.

* Good, because it costs nothing extra.
* Bad, because discussions are scattered, hard to list, and cannot be
  marked as superseded.

## More Information

* MADR: https://adr.github.io/madr/ (template version 4.0.0).
* Michael Nygard, "Documenting Architecture Decisions" (2011):
  https://www.cognitect.com/blog/2011/11/15/documenting-architecture-decisions
