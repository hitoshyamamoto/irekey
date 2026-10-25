// SPDX-License-Identifier: MIT

// Package transfer moves bundles to a device's staging area by content
// address: only missing blobs are sent, interrupted transfers resume and
// bandwidth is limited. It also estimates the size of a transfer. A transfer
// changes nothing that runs.
//
// Not implemented yet; see ROADMAP.md.
package transfer
