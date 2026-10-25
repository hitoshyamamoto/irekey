// SPDX-License-Identifier: MIT

// Package state keeps the local operational state of the operator machine in
// SQLite: jobs, progress, the pending queue and the snapshot and transfer
// caches, with their migrations. Anything another developer needs lives in
// the deployment repository instead.
//
// Not implemented yet; see ROADMAP.md.
package state
