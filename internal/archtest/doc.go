// SPDX-License-Identifier: MIT

// Package archtest holds the architecture tests that enforce the package
// dependency rule: which packages may import which, what the executor may
// depend on, and which packages must stay free of I/O. The tests read the
// import graph with go list; the package has no code of its own.
package archtest
