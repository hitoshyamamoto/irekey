// SPDX-License-Identifier: MIT

// Package ui serves the web UI started by irekey ui: HTTP handlers, templates
// and static assets embedded in the binary, listening on 127.0.0.1 only. It is
// read-only and has no logic of its own: every handler calls package app and
// never the transport.
//
// Not implemented yet; see ROADMAP.md.
package ui
