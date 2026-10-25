// SPDX-License-Identifier: MIT

package gen

import (
	"slices"
	"testing"
)

func TestResolvedStates(t *testing.T) {
	want := map[int][]string{
		0: nil, 1: {"unchanged"}, 6: {"unchanged"}, 7: {"previous-release"}, 8: {"needs-intervention"},
		9: nil, 10: {"unknown"}, 12: {"unknown"}, 15: {"unknown", "unchanged"}, 130: {"unchanged", "unknown"},
		16: nil,
	}
	for exit, states := range want {
		if got := ResolvedStates(exit); !slices.Equal(got, states) {
			t.Errorf("ResolvedStates(%d) = %q, want %q", exit, got, states)
		}
	}
}

func TestPessimismCoversTheErrorStates(t *testing.T) {
	// Every device state except new-release, which no failure leaves a
	// device in, has a place in the order.
	for _, d := range DeviceStates {
		if got := slices.Contains(Pessimism, d.Name); got != (d.Name != "new-release") {
			t.Errorf("Pessimism contains %s: %v", d.Name, got)
		}
	}
}
