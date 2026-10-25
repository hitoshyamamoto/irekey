// SPDX-License-Identifier: MIT

package redact

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"sync"
)

// Marker replaces every secret in redacted text.
const Marker = "[REDACTED]"

// MinSecretLen is the length, in bytes, of the shortest secret that Add
// accepts. A secret of one to three bytes would replace unrelated text
// everywhere (every "a", every "id", every "the") and make the output
// useless, while protecting next to nothing.
const MinSecretLen = 4

// A Redactor replaces registered secrets with [Marker]. It is safe for
// concurrent use. The zero value is a Redactor with no secrets; a Redactor
// must not be copied after first use.
type Redactor struct {
	mu   sync.RWMutex
	seen map[string]struct{} // every registered pattern, for deduplication
	pats *patterns           // replaced, never modified, by Add
}

// New returns a Redactor that knows the given secrets. Secrets that Add
// rejects, because they are shorter than [MinSecretLen], are ignored; call
// Add to learn whether a secret was accepted.
func New(secrets ...string) *Redactor {
	r := &Redactor{}
	for _, s := range secrets {
		r.Add(s)
	}
	return r
}

// Add registers a secret and reports whether it is redacted from now on: it
// returns false, and ignores the secret, when the secret is shorter than
// [MinSecretLen] bytes, and true otherwise, also when the secret is already
// registered. From then on, every occurrence of the secret, and of each of
// its encoded forms, is replaced by [Marker] by String, Bytes, every Writer
// and every handler of r. An encoded form shorter than MinSecretLen is not
// registered.
//
// Add matches the secret exactly as given and does not normalize it: a
// secret read from a file usually ends with a newline that the text it
// appears in does not have, and must be trimmed by the caller.
//
// Text that was printed before Add is not recalled, and neither is an
// occurrence whose first bytes a Writer had already written. Call Add as
// soon as a secret is obtained, before it can reach any output.
func (r *Redactor) Add(secret string) bool {
	if len(secret) < MinSecretLen {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = map[string]struct{}{}
	}
	var added []string
	for _, f := range forms(secret) {
		if _, ok := r.seen[f]; !ok && len(f) >= MinSecretLen {
			r.seen[f] = struct{}{}
			added = append(added, f)
		}
	}
	if len(added) > 0 {
		r.pats = r.pats.with(added)
	}
	return true
}

// String returns s with every secret replaced by [Marker].
func (r *Redactor) String(s string) string {
	out, _ := r.redactString(s)
	return out
}

// Bytes returns a copy of b with every secret replaced by [Marker].
func (r *Redactor) Bytes(b []byte) []byte {
	out, _, _, _ := r.load().redact(nil, b, scanState{}, false)
	return out
}

// redactString is String that also reports whether a secret was found. The
// result alone cannot tell: a secret may be equal to Marker.
func (r *Redactor) redactString(s string) (string, bool) {
	p := r.load()
	if p.count == 0 {
		return s, false
	}
	out, _, _, matched := p.redact(nil, []byte(s), scanState{}, false)
	if !matched {
		return s, false
	}
	return string(out), true
}

// load returns the patterns registered so far. The result is never modified,
// so it can be used without holding the lock.
func (r *Redactor) load() *patterns {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.pats == nil {
		return &patterns{}
	}
	return r.pats
}

// forms returns secret and the encoded forms it takes in output, possibly
// with duplicates.
//
// Safety (DataSeparation): a secret written by encoding/json, by a log/slog
// handler, with %q or through Sanitize appears escaped, not verbatim. Each
// form is computed with the encoder itself, so it matches byte for byte what
// the encoder writes inside a string.
func forms(secret string) []string {
	out := []string{secret, Sanitize(secret)}
	// encoding/json escapes <, > and & by default (json.Marshal); an
	// Encoder with SetEscapeHTML(false) and the JSON handler of log/slog do
	// not.
	if b, err := json.Marshal(secret); err == nil {
		out = append(out, string(unquote(b)))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(secret); err == nil {
		out = append(out, string(unquote(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))))
	}
	// strconv.Quote is what %q and the text handler of log/slog use.
	out = append(out, string(unquote([]byte(strconv.Quote(secret)))))
	return out
}

// unquote removes the quotation marks around an encoded string.
func unquote(b []byte) []byte {
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		return b[1 : len(b)-1]
	}
	return b
}

// patterns is an immutable set of non-empty patterns to replace.
type patterns struct {
	byFirst [256][]string // patterns by their first byte, longest first
	maxLen  int           // length of the longest pattern
	count   int           // number of patterns
}

// with returns a copy of p, which may be nil, with the given new patterns.
func (p *patterns) with(add []string) *patterns {
	q := &patterns{}
	if p != nil {
		*q = *p // copies the array; the slices are cloned below before changing
	}
	for _, s := range add {
		c := s[0]
		q.byFirst[c] = append(slices.Clip(q.byFirst[c]), s)
		slices.SortStableFunc(q.byFirst[c], func(a, b string) int { return cmp.Compare(len(b), len(a)) })
		q.maxLen = max(q.maxLen, len(s))
		q.count++
	}
	return q
}

// scanState is what redact carries from one chunk of a stream to the next.
type scanState struct {
	inRun bool // the chunk starts inside a covered run
	end   int  // with inRun: where the run ends, relative to the chunk start
}

// redact appends src to dst with every occurrence of a pattern replaced, and
// returns the result, the number of bytes of src consumed, the state at that
// point and whether any pattern was found.
//
// Occurrences are merged: each maximal stretch of bytes covered by
// overlapping occurrences (a covered run) is replaced by one [Marker], so no
// part of any occurrence survives. Occurrences that only touch are separate
// runs, with a Marker each. redact scans every position, also inside a run,
// and extends the run to the end of the longest pattern found there.
//
// With more set, src is not the end of the stream. Then redact stops at the
// first position where the rest of src is a proper prefix of a pattern,
// since the bytes still to come may complete it, and the caller keeps the
// unconsumed rest, together with the returned state, for the next call.
// Inside a run, the Marker has already been written; the held bytes are
// only needed to see how far the run extends.
//
// Safety (DataSeparation): the prefix check comes before matching, so no
// decision is taken at a position where a longer pattern could still
// complete. Every pattern that matches at a decided position in the whole
// stream is then visible in src, and the result does not depend on how the
// stream is cut into chunks.
func (p *patterns) redact(dst, src []byte, st scanState, more bool) (out []byte, consumed int, next scanState, matched bool) {
	inRun, end := st.inRun, st.end
	copied := 0 // src[:copied] is in dst or covered by a Marker
	i := 0
	for ; i < len(src); i++ {
		if inRun && i == end {
			inRun = false
			copied = i
		}
		cands := p.byFirst[src[i]]
		if len(cands) == 0 {
			continue
		}
		rest := src[i:]
		if more && len(rest) < p.maxLen && properPrefix(cands, rest) {
			break
		}
		n := longestAt(cands, rest)
		switch {
		case n == 0:
		case inRun:
			end = max(end, i+n)
		default:
			dst = append(dst, src[copied:i]...)
			dst = append(dst, Marker...)
			inRun, end, matched = true, i+n, true
		}
	}
	if inRun && i == end {
		// The run ends with the input; a pattern ends at most there.
		inRun = false
		copied = i
	}
	if !inRun {
		dst = append(dst, src[copied:i]...)
		return dst, i, scanState{}, matched
	}
	return dst, i, scanState{inRun: true, end: end - i}, matched
}

// properPrefix reports whether rest is a proper prefix of one of cands,
// which are sorted longest first.
func properPrefix(cands []string, rest []byte) bool {
	for _, c := range cands {
		if len(c) <= len(rest) {
			return false
		}
		if c[:len(rest)] == string(rest) {
			return true
		}
	}
	return false
}

// longestAt returns the length of the longest of cands, which are sorted
// longest first, that rest starts with, or 0.
func longestAt(cands []string, rest []byte) int {
	for _, c := range cands {
		if len(c) <= len(rest) && string(rest[:len(c)]) == c {
			return len(c)
		}
	}
	return 0
}
