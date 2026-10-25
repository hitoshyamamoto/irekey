// SPDX-License-Identifier: MIT

package redact

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/quick"
)

// writeChunks writes data to w in chunks of the given sizes, which must add
// up to len(data), and closes w.
func writeChunks(t *testing.T, w *Writer, data []byte, sizes []int) {
	t.Helper()
	for _, n := range sizes {
		got, err := w.Write(data[:n])
		if err != nil || got != n {
			t.Fatalf("Write(%q) = %d, %v, want %d, nil", data[:n], got, err, n)
		}
		data = data[n:]
	}
	if len(data) > 0 {
		t.Fatalf("writeChunks: %d bytes left over", len(data))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ones returns n chunk sizes of one byte.
func ones(n int) []int {
	sizes := make([]int, n)
	for i := range sizes {
		sizes[i] = 1
	}
	return sizes
}

// patternsOf returns every pattern registered in r: the secrets and their
// encoded forms.
func patternsOf(r *Redactor) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for p := range r.seen {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// overlapsMarker reports whether s could appear in redacted output without
// appearing in the input, because Marker itself, or Marker together with
// the text around it, spells it: s occurs inside Marker ("RED"), contains
// Marker, starts with the end of Marker ("]x" in "[REDACTED]x") or ends
// with its start ("x[" in "x[REDACTED]"). Any other pattern that appears in
// the output would lie entirely in text copied from the input, where the
// redactor would have replaced it.
func overlapsMarker(s string) bool {
	if strings.Contains(Marker, s) || strings.Contains(s, Marker) {
		return true
	}
	for i := 1; i < len(s); i++ {
		if strings.HasSuffix(Marker, s[:i]) || strings.HasPrefix(Marker, s[i:]) {
			return true
		}
	}
	return false
}

// leaked returns the registered patterns that appear in out, except those
// that overlap Marker.
func leaked(r *Redactor, out []byte) []string {
	var found []string
	for _, p := range patternsOf(r) {
		if !overlapsMarker(p) && bytes.Contains(out, []byte(p)) {
			found = append(found, p)
		}
	}
	return found
}

func TestOverlapsMarker(t *testing.T) {
	for s, want := range map[string]bool{
		"RED": true, Marker: true, "x" + Marker + "y": true, "]x": true, "x[": true, "D]x": true,
		"x": false, "x]": false, "[x": false, "RxD": false, "": true,
	} {
		if got := overlapsMarker(s); got != want {
			t.Errorf("overlapsMarker(%q) = %v, want %v", s, got, want)
		}
	}
}

// reference redacts in with the given patterns the slow way, as an oracle:
// it finds every occurrence of every pattern, merges the occurrences that
// overlap, and writes one Marker for each merged stretch.
func reference(pats []string, in []byte) []byte {
	ends := make([]int, len(in)) // end of the longest occurrence at i, or 0
	for i := range in {
		for _, p := range pats {
			if bytes.HasPrefix(in[i:], []byte(p)) {
				ends[i] = max(ends[i], i+len(p))
			}
		}
	}
	var out []byte
	for i := 0; i < len(in); {
		if ends[i] == 0 {
			out = append(out, in[i])
			i++
			continue
		}
		end := ends[i]
		for j := i + 1; j < end; j++ {
			end = max(end, ends[j])
		}
		out = append(out, Marker...)
		i = end
	}
	return out
}

func TestDataSeparation_WriterRedactsAcrossChunkBoundaries(t *testing.T) {
	// s3cr3t is a prefix of s3cr3t-t0ken, which t0ken-x9 overlaps; the
	// sample ends with a prefix of both, which is written unchanged when the
	// Writer is closed.
	r := New("s3cr3t", "s3cr3t-t0ken", "t0ken-x9")
	const sample = "user=acme pass=s3cr3t key=s3cr3t-t0ken-x9 tail=s3cr3t-t0k t0ken-x9! s3cr3"
	const want = "user=acme pass=" + Marker + " key=" + Marker + " tail=" + Marker + "-t0k " +
		Marker + "! s3cr3"
	if got := r.String(sample); got != want {
		t.Fatalf("String = %q, want %q", got, want)
	}
	check := func(t *testing.T, sizes []int) {
		t.Helper()
		var buf bytes.Buffer
		writeChunks(t, r.NewWriter(&buf), []byte(sample), sizes)
		if buf.String() != want {
			t.Errorf("chunks %v: wrote %q, want %q", sizes, buf.String(), want)
		}
	}
	n := len(sample)
	for i := 0; i <= n; i++ {
		check(t, []int{i, n - i})
		for j := i; j <= n; j++ {
			check(t, []int{i, j - i, n - j})
		}
	}
	check(t, ones(n))
}

func TestDataSeparation_WriterHoldsPrefixUntilFlush(t *testing.T) {
	var buf bytes.Buffer
	w := New("s3cr3t").NewWriter(&buf)
	if _, err := w.Write([]byte("pass=s3cr")); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "pass=" {
		t.Errorf("before the secret completes, wrote %q, want %q", got, "pass=")
	}
	if _, err := w.Write([]byte("3t and s3")); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "pass="+Marker+" and "; got != want {
		t.Errorf("after the secret completes, wrote %q, want %q", got, want)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "pass="+Marker+" and s3"; got != want {
		t.Errorf("after Flush, wrote %q, want %q", got, want)
	}
	if err := w.Flush(); err != nil {
		t.Errorf("Flush with nothing held: %v", err)
	}
}

func TestDataSeparation_WriterHoldsInsideARun(t *testing.T) {
	// The Marker is written when a run starts; while a longer overlapping
	// secret may still extend the run, only the bytes needed to tell are
	// held, and nothing of the run is written.
	var buf bytes.Buffer
	w := New("hunter2", "2fa-backup-77319").NewWriter(&buf)
	for _, chunk := range []string{"x hunter2", "fa-back", "up-77319 y"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "2fa") || strings.Contains(buf.String(), "back") {
			t.Fatalf("after %q, wrote %q", chunk, buf.String())
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "x "+Marker+" y"; got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestDataSeparation_WriterSeesSecretsAddedLater(t *testing.T) {
	r := New()
	var buf bytes.Buffer
	w := r.NewWriter(&buf)
	if _, err := w.Write([]byte("early k3y9\nsplit k3")); err != nil {
		t.Fatal(err)
	}
	r.Add("k3y9")
	if _, err := w.Write([]byte("y9\nlate k3y9\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// What was written before Add cannot be recalled, and an occurrence
	// whose first bytes were written before Add is not recognized.
	if got, want := buf.String(), "early k3y9\nsplit k3y9\nlate "+Marker+"\n"; got != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

// streamCase is a random input for the streaming property: secrets, an
// input that often contains them, and the chunk sizes it is written in.
type streamCase struct {
	secrets []string
	input   []byte
	sizes   []int
}

// alphabet is small, so that random secrets overlap, share prefixes and
// occur in the input often. It contains bytes of Marker, characters that
// the encoded forms escape, and bytes that are not valid UTF-8 on their
// own.
const alphabet = "ab[]RE\"\\<\n\x00\xc3\xa9\xff"

func randString(rng *rand.Rand, minLen, maxLen int) string {
	b := make([]byte, minLen+rng.Intn(maxLen-minLen+1))
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

// Generate implements quick.Generator. Secrets are made to overlap: one
// may start with the end of another, or overlap itself ("abab"). Some are
// shorter than MinSecretLen, so Add rejects them.
func (streamCase) Generate(rng *rand.Rand, _ int) reflect.Value {
	var c streamCase
	for range 1 + rng.Intn(4) {
		s := randString(rng, 2, 8)
		switch prev := c.secrets; {
		case len(prev) > 0 && rng.Intn(3) == 0:
			p := prev[rng.Intn(len(prev))]
			s = p[rng.Intn(len(p)):] + s
		case rng.Intn(6) == 0:
			s += s[:rng.Intn(len(s))]
		}
		c.secrets = append(c.secrets, s)
	}
	for range rng.Intn(12) {
		s := c.secrets[rng.Intn(len(c.secrets))]
		switch rng.Intn(4) {
		case 0:
			c.input = append(c.input, s...)
		case 1:
			c.input = append(c.input, s[:rng.Intn(len(s))]...)
		case 2:
			c.input = append(c.input, s[rng.Intn(len(s)):]...)
		default:
			c.input = append(c.input, randString(rng, 0, 4)...)
		}
	}
	for left := len(c.input); left > 0; {
		n := rng.Intn(min(left, 8) + 1) // empty writes included
		c.sizes = append(c.sizes, n)
		left -= n
	}
	return reflect.ValueOf(c)
}

// checkStream writes in to a Writer of r in chunks of the given sizes and
// returns a description of the first failed check, or "". The Writer must
// write what Bytes returns, which must equal the reference, and no
// registered pattern may appear in the output. Patterns that overlap Marker
// are left out of the last check: Marker, alone or with its neighbors, can
// spell them without any secret leaking (a secret "]x" appears in
// "[REDACTED]x" when another secret is followed by "x").
func checkStream(t *testing.T, r *Redactor, in []byte, sizes []int) string {
	t.Helper()
	want := r.Bytes(in)
	if ref := reference(patternsOf(r), in); !bytes.Equal(want, ref) {
		return fmt.Sprintf("Bytes returned %q, the reference %q", want, ref)
	}
	var buf bytes.Buffer
	writeChunks(t, r.NewWriter(&buf), in, sizes)
	if !bytes.Equal(buf.Bytes(), want) {
		return fmt.Sprintf("chunks %v: wrote %q, Bytes returned %q", sizes, buf.Bytes(), want)
	}
	if found := leaked(r, want); len(found) > 0 {
		return fmt.Sprintf("output %q contains %q", want, found)
	}
	return ""
}

func TestDataSeparation_StreamingEqualsBatch(t *testing.T) {
	// For every set of secrets, input and chunking.
	prop := func(c streamCase) bool {
		if msg := checkStream(t, New(c.secrets...), c.input, c.sizes); msg != "" {
			t.Logf("secrets %q, input %q: %s", c.secrets, c.input, msg)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Error(err)
	}
}

func TestDataSeparation_WriterIsSafeForConcurrentUse(t *testing.T) {
	const secret = "c0ncurr3nt-s3cret"
	const workers, lines = 8, 200
	r := New(secret)
	var buf bytes.Buffer
	w := r.NewWriter(&buf)
	var wg sync.WaitGroup
	for id := range workers {
		wg.Go(func() {
			for i := range lines {
				line := fmt.Sprintf("worker %d line %d key=%s end\n", id, i, secret)
				if _, err := w.Write([]byte(line)); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	// Secrets added and text redacted meanwhile, through the same Redactor.
	wg.Go(func() {
		for i := range lines {
			r.Add(fmt.Sprintf("added-%d-zz", i))
			if got := r.String("key=" + secret); got != "key="+Marker {
				t.Errorf("String = %q", got)
				return
			}
		}
	})
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), secret) {
		t.Fatal("the output contains the secret")
	}
	got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(got) != workers*lines {
		t.Fatalf("wrote %d lines, want %d", len(got), workers*lines)
	}
	// Every line arrives whole and redacted, in some order.
	seen := map[string]bool{}
	for _, line := range got {
		seen[line] = true
	}
	for id := range workers {
		for i := range lines {
			if want := fmt.Sprintf("worker %d line %d key=%s end", id, i, Marker); !seen[want] {
				t.Fatalf("line %q is missing from the output", want)
			}
		}
	}
}

// recorder records every write it receives.
type recorder struct{ writes [][]byte }

func (r *recorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, bytes.Clone(p))
	return len(p), nil
}

func TestWriterPassesThroughWithoutSecrets(t *testing.T) {
	var rec recorder
	w := New().NewWriter(&rec)
	for _, chunk := range []string{"first ", "", "second"} {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if want := [][]byte{[]byte("first "), []byte("second")}; !reflect.DeepEqual(rec.writes, want) {
		t.Errorf("writes = %q, want %q", rec.writes, want)
	}
}

func TestWriterWriteAfterClose(t *testing.T) {
	var buf bytes.Buffer
	w := New("s3cr3t").NewWriter(&buf)
	if _, err := w.Write([]byte("s3cr")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "s3cr" {
		t.Errorf("Close wrote %q, want the held prefix %q", buf.String(), "s3cr")
	}
	if n, err := w.Write([]byte("3t")); n != 0 || !errors.Is(err, ErrClosed) {
		t.Errorf("Write after Close = %d, %v, want 0, %v", n, err, ErrClosed)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Errorf("Flush after Close: %v", err)
	}
}

// failingWriter accepts limit bytes, then fails.
type failingWriter struct {
	limit int
	err   error // returned once the limit is reached; nil for a short write
	buf   bytes.Buffer
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.buf.Len()+len(p) <= f.limit {
		return f.buf.Write(p)
	}
	n := f.limit - f.buf.Len()
	f.buf.Write(p[:n])
	return n, f.err
}

func TestWriterErrorsAreSticky(t *testing.T) {
	errBoom := errors.New("boom")
	tests := []struct {
		name    string
		secrets []string
		err     error // returned by the underlying writer
		want    error // returned by the Writer
	}{
		{"pass-through", nil, errBoom, errBoom},
		{"redacting", []string{"s3cr3t"}, errBoom, errBoom},
		{"short write", []string{"s3cr3t"}, nil, io.ErrShortWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &failingWriter{limit: 4, err: tt.err}
			w := New(tt.secrets...).NewWriter(f)
			if n, err := w.Write([]byte("abc")); n != 3 || err != nil {
				t.Fatalf("first Write = %d, %v, want 3, nil", n, err)
			}
			if n, err := w.Write([]byte("defgh")); n != 0 || !errors.Is(err, tt.want) {
				t.Errorf("failing Write = %d, %v, want 0, %v", n, err, tt.want)
			}
			if n, err := w.Write([]byte("s3cr3t")); n != 0 || !errors.Is(err, tt.want) {
				t.Errorf("Write after the failure = %d, %v, want 0, %v", n, err, tt.want)
			}
			if err := w.Flush(); !errors.Is(err, tt.want) {
				t.Errorf("Flush after the failure = %v, want %v", err, tt.want)
			}
			if err := w.Close(); !errors.Is(err, tt.want) {
				t.Errorf("Close after the failure = %v, want %v", err, tt.want)
			}
			if got := f.buf.String(); got != "abcd" {
				t.Errorf("the underlying writer got %q, want %q", got, "abcd")
			}
		})
	}
}

func TestWriterFlushError(t *testing.T) {
	errBoom := errors.New("boom")
	f := &failingWriter{limit: 0, err: errBoom}
	w := New("s3cr3t").NewWriter(f)
	if _, err := w.Write([]byte("s3cr")); err != nil {
		t.Fatalf("Write of a held prefix: %v", err)
	}
	if err := w.Close(); !errors.Is(err, errBoom) {
		t.Errorf("Close = %v, want %v", err, errBoom)
	}
}
