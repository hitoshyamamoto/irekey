// SPDX-License-Identifier: MIT

// Package journal reads and writes the executor's journal on the device: one
// record before and one after every step, each flushed to disk, so that
// recovery after a crash or a power loss is deterministic.
//
// It runs on the device inside the executor and never listens on a port,
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md).
//
// Not implemented yet; see ROADMAP.md.
package journal
