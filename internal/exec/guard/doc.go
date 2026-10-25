// SPDX-License-Identifier: MIT

// Package guard arms and disarms the guard, the persistent systemd timer and
// service pair that rolls back an activation when confirmation does not
// arrive in time or the device reboots first.
//
// It runs on the device inside the executor and never listens on a port,
// and opens no connection to other hosts; whether local health checks may
// connect to the device's own services on loopback is an open question (see
// docs/open-questions.md).
//
// Not implemented yet; see ROADMAP.md.
package guard
