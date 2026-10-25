// SPDX-License-Identifier: MIT

package schema

import "slices"

// supported holds the schema identifiers this build reads.
// docs/schema-versions.txt mirrors it; after a change, run
// go test ./internal/schema -update and review the diff.
var supported = []string{}

// Supported returns the schema: identifiers of the deployment repository and
// component files this build can read, each of the form irekey/<kind>/v<N>,
// sorted. The result is never nil and belongs to the
// caller.
func Supported() []string {
	ids := make([]string, len(supported))
	copy(ids, supported)
	slices.Sort(ids)
	return ids
}
