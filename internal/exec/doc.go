// SPDX-License-Identifier: MIT

// Package exec is the core of the executor, irekey-exec: the activation state
// machine that stops, switches, starts and checks a staged release on a
// device, and confirms or rolls it back, using the journal, guard, adapter and
// health packages below it.
//
// It runs on the device inside the executor and never listens on a port,
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md).
//
// Not implemented yet; see ROADMAP.md.
package exec
