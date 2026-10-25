// SPDX-License-Identifier: MIT

package redact

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// FuzzWriter writes arbitrary input, with arbitrary secrets separated by
// "|", to a Writer in chunks chosen from a seed. The output must equal
// Bytes on the whole input, which must equal the reference, and no
// registered pattern may appear in it, except those that overlap Marker
// (see checkStream).
func FuzzWriter(f *testing.F) {
	f.Add("s3cr3t|s3cr3t-t0ken|t0ken-x9", []byte("pass=s3cr3t key=s3cr3t-t0ken-x9 s3cr3"), uint64(1))
	f.Add("hunter2|2fa-backup-77319", []byte("x hunter2fa-backup-77319 hunter2fa"), uint64(2))
	f.Add("1212|abab", []byte("1212121 ababab]x[REDACTED]"), uint64(3))
	f.Add(`q"<>|`+"\xff\x00\xfe\xfd", []byte(`"q\"<>" `+"\xff\x00\xfe\xfd"), uint64(4))
	f.Add("", []byte("no secrets at all"), uint64(5))
	f.Fuzz(func(t *testing.T, secrets string, input []byte, seed uint64) {
		r := New()
		n := 0
		for s := range strings.SplitSeq(secrets, "|") {
			if n == 8 {
				break
			}
			r.Add(s[:min(len(s), 16)])
			n++
		}
		rng := rand.New(rand.NewPCG(seed, seed))
		var sizes []int
		for left := len(input); left > 0; {
			size := rng.IntN(min(left, 16) + 1)
			sizes = append(sizes, size)
			left -= size
		}
		if msg := checkStream(t, r, input, sizes); msg != "" {
			t.Fatalf("secrets %q, input %q: %s", secrets, input, msg)
		}
	})
}

// FuzzSanitize checks that the output of Sanitize is valid UTF-8, contains
// only newline, tab and graphic characters, does not change when sanitized
// again, and equals the input when the input was clean already.
func FuzzSanitize(f *testing.F) {
	for _, tt := range sanitizeTests {
		f.Add(tt.in)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkSanitized(t, s, Sanitize(s))
	})
}
