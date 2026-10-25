// SPDX-License-Identifier: MIT

// Package health runs the health checks declared in a component manifest and
// judges them against the baseline taken before the update, so that a check
// that was already failing is reported instead of causing a rollback.
//
// It runs on the device inside the executor and never listens on a port,
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md).
//
// Not implemented yet; see ROADMAP.md.
package health
