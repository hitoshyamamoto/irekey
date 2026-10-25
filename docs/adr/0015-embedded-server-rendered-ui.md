---
status: proposed
date: 2026-10-25
decision-makers: Hitoshi Yamamoto (@hitoshyamamoto)
---

# Embed a server-rendered web UI without a Node toolchain

## Context and Problem Statement

`irekey ui` opens a local, read-only web interface next to the CLI: it
shows customers, sites and devices, release sets, the pending queue,
recent deliveries, drift and patch-stack diffs, and a live bar with the
rollout in progress. It has no logic of its own: its handlers call the
same service layer (`internal/app`) as the CLI, and its JSON uses the
same types as `--json`. It shows customer data through a listener on the
operator machine, inside the single static binary that anyone must be
able to rebuild from a git tag to the published checksums.

How should the UI be built, packaged and served?

## Decision Drivers

* A release build needs nothing but the Go toolchain, so it stays
  reproducible and its supply chain stays small enough to review.
* The UI works offline and requests nothing from any other server.
* Other machines cannot reach the listener, and neither other local
  users nor websites open in the same browser can read its pages.
* Content read from devices is untrusted and must be escaped.
* One maintainer can keep it working, including keyboard use, WCAG 2.2
  AA contrast and widths down to 360 px.

## Considered Options

* Server-rendered html/template with vendored htmx
* Single-page application built with a Node toolchain
* Server-rendered html/template without JavaScript
* templ components with htmx

## Decision Outcome

Chosen option: "Server-rendered html/template with vendored htmx",
because it needs only the Go standard library and one small vendored
script, escapes every value by context, and still allows live updates
and partial page changes.

* Templates, one stylesheet and the htmx file are embedded with `embed`.
  htmx is pinned to one version and kept with its license file; a test
  checks the file's digest. No CDN, no bundler, no external asset.
* htmx is licensed under 0BSD, which is not on the allowed license list
  of ADR 0003. Before the file is vendored, an ADR must add 0BSD to that
  list; if it does not, another option has to be chosen.
* The server binds only to `127.0.0.1` and `::1`, on a random port by
  default; a random per-session token in the URL is required on every
  request.
* Every response carries a strict Content-Security-Policy: scripts,
  styles and connections from the page's own origin only, no inline
  script, no framing. There is no state-changing endpoint; the rollout
  bar is fed by server-sent events from the same event stream as
  `--json`.
* The organization follows Spotify's layout (a sidebar with Home, Search
  and Fleet; the rollout bar where Spotify shows what is playing; release
  sets shown like albums) with irekey's own visual identity: no Spotify
  colors, logo, typography or assets. Dark and light themes are CSS
  custom properties in one file; status always has color, icon and word.

This record is proposed: the design is the specification's recommended
default, and it is confirmed or revised once the read-only UI is
implemented.

### Consequences

* Good, because a release build needs no Node toolchain and no npm
  dependency tree to pin and audit.
* Good, because the UI cannot disagree with the CLI, and its attack
  surface stays small: loopback only, a token, a strict policy, no
  writes.
* Bad, because rich views in the browser (graph visualization, very
  large diffs) are harder; graph visualization is later work and may
  need another vendored script, decided then.
* Bad, because Dependabot does not track a vendored file; htmx is
  updated by hand, with the digest test as the check.
* Bad, because htmx's license, 0BSD, needs its own ADR note adding it to
  the allowed list before htmx can be vendored.

### Confirmation

Tests: the server listens on loopback addresses only; every response
carries the policy; no page references an external URL; a request
without the token is refused; the htmx digest matches. The dependency
rule test shows that `internal/ui` reaches devices only through
`internal/app`. An HTML and accessibility linter, chosen when the UI is
built, runs in CI.

Evidence that would change the decision: a required view cannot be made
usable with server rendering and htmx; the accessibility rules cannot be
met this way; or htmx stops being maintained.

## Pros and Cons of the Options

### Server-rendered html/template with vendored htmx

* Good, because `html/template` escapes values by context (HTML,
  attributes, URLs, scripts, styles), which matters for device output.
* Good, because htmx is one file, used through HTML attributes.
* Bad, because htmx is licensed under 0BSD (Zero-Clause BSD), which is
  not on the allowed license list of ADR 0003; adopting it requires an
  ADR that adds 0BSD to that list, or choosing another option. The 0BSD
  text grants permission to use, copy, modify and distribute the
  software "for any purpose with or without fee" and sets no condition.

### Single-page application built with a Node toolchain

* Good, because it allows the richest interaction.
* Bad, because the release build would need Node, npm and a large,
  fast-moving dependency tree: a second supply chain next to Go modules.
* Bad, because rendering logic would live in JavaScript beside the Go
  service layer: two code bases for one maintainer.

### Server-rendered html/template without JavaScript

* Good, because it is the smallest possible surface.
* Bad, because the rollout bar could not update live.

### templ components with htmx

* Good, because templates are checked at compile time.
* Bad, because it adds a dependency and a code generation step, with
  generated code to review, for a UI of a few views.

## More Information

* `html/template`: https://pkg.go.dev/html/template; `embed`:
  https://pkg.go.dev/embed
* htmx: https://htmx.org/. Checked 2026-10-25: its `LICENSE` file is
  the Zero-Clause BSD license (SPDX `0BSD`) on the default branch and at
  v2.0.11 and v4.0.0
  (https://github.com/bigskysoftware/htmx/blob/v2.0.11/LICENSE,
  https://spdx.org/licenses/0BSD.html); the npm package `htmx.org`
  declares `0BSD` for 2.0.11 and `BSD-0-Clause` for 4.0.0. The version
  is chosen when the file is vendored. In htmx 2.0.11,
  `htmx.config.includeIndicatorStyles` (inline indicator styles) and
  `htmx.config.allowEval` default to `true`; under the strict policy
  above both are expected to be set to `false` (to be verified when the
  UI is built): https://htmx.org/docs/
* templ: https://github.com/a-h/templ
* Content Security Policy Level 3: https://www.w3.org/TR/CSP3/;
  server-sent events:
  https://html.spec.whatwg.org/multipage/server-sent-events.html;
  WCAG 2.2: https://www.w3.org/TR/WCAG22/

The specification's "Web UI" section defines the views and the visual
and accessibility rules. Spotify's name and visual identity belong to
Spotify; only the organization of its interface is borrowed.
