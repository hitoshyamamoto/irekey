// SPDX-License-Identifier: MIT

// Package schema holds the JSON Schemas that describe the files irekey reads
// and writes, and validates documents against them. Every deployment
// repository file and every file irekey reads from a component repository
// names its schema in a schema: field, with an identifier of the form
// irekey/<kind>/v<N>; [Supported] lists the identifiers this build reads. It
// performs no I/O.
//
// No schema is defined and validation is not implemented yet; see ROADMAP.md.
package schema
