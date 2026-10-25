// SPDX-License-Identifier: MIT

// Package adapter defines the runtime adapters that stop, start and inspect
// components on a device, with implementations for systemd and Docker
// Compose.
//
// It runs on the device inside the executor and never listens on a port,
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md).
//
// Not implemented yet; see ROADMAP.md.
package adapter
