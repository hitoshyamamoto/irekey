// SPDX-License-Identifier: MIT

// Package snapshot builds snapshots, the read-only records of what a device
// runs: file manifests with digests and facts, with exclusions such as data
// paths. It also computes drift, the difference between a device's expected
// state and its snapshot.
//
// Not implemented yet; see ROADMAP.md.
package snapshot
