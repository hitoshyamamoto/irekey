// SPDX-License-Identifier: MIT

package redact

import (
	"errors"
	"io"
	"slices"
	"sync"
)

// ErrClosed is returned by a write to a [Writer] after Close.
var ErrClosed = errors.New("redact: write to a closed Writer")

// A Writer redacts a stream of bytes on its way to another [io.Writer]. Its
// output is the same as [Redactor.Bytes] on everything written to it,
// whatever the chunk boundaries are, as long as no secret is added and Flush
// is not called while the stream is in progress. It is safe for concurrent
// use: each Write is processed as a whole, in the order the calls take the
// Writer's lock.
//
// When the end of a chunk could be the start of a secret, the Writer holds
// those bytes back until the next Write shows whether the secret completes;
// at most the length of the longest registered form, minus one byte. Flush
// and Close write the held bytes: call Close when the output ends, or a held
// prefix is never written.
//
// After the underlying writer fails, nothing more is written: the failing
// Write reports 0 bytes written, and every later Write, Flush and Close
// returns that error, except that a Write after Close returns [ErrClosed].
// A secret cut short by the failure is therefore not completed by later
// writes.
//
// The Writer holds its lock while it calls the underlying writer, so the
// underlying writer must not write to the Writer itself; such a re-entrant
// call blocks forever.
type Writer struct {
	r *Redactor
	w io.Writer

	mu      sync.Mutex
	pending []byte    // held back: a proper prefix of a pattern, or empty
	state   scanState // where pending starts: inside a covered run or not
	err     error     // first error of the underlying writer
	closed  bool
}

// NewWriter returns a Writer that writes to w with r's secrets redacted,
// including secrets added to r later. With no secret registered, it passes
// every write through unchanged.
func (r *Redactor) NewWriter(w io.Writer) *Writer {
	return &Writer{r: r, w: w}
}

// Write redacts p and writes what can be decided to the underlying writer,
// holding back a trailing proper prefix of a secret. It returns len(p) on
// success. On a failure of the underlying writer it returns 0 and the error,
// since redacted output cannot be mapped back to bytes of p.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, ErrClosed
	}
	if w.err != nil {
		return 0, w.err
	}
	pats := w.r.load()
	if pats.count == 0 {
		// Nothing is held back without a pattern, and patterns are never
		// removed, so there is nothing pending either.
		if len(p) > 0 {
			if err := w.emit(p); err != nil {
				return 0, err
			}
		}
		return len(p), nil
	}
	src := p
	if len(w.pending) > 0 {
		src = slices.Concat(w.pending, p)
	}
	out, n, st, _ := pats.redact(make([]byte, 0, len(src)), src, w.state, true)
	w.pending = append(w.pending[:0], src[n:]...)
	w.state = st
	if len(out) > 0 {
		if err := w.emit(out); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush writes the held bytes to the underlying writer, redacting the
// secrets they contain without waiting for more input. It ends the stream
// as far as matching goes: a secret whose bytes are split by a Flush is not
// recognized, so flush only where the output is complete. Flush does not
// flush the underlying writer.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flush()
}

// Close flushes the held bytes and makes later writes fail with
// [ErrClosed]. It does not close the underlying writer. Closing again
// returns nil, unless the underlying writer has failed.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.flush()
	w.closed = true
	w.pending = nil
	return err
}

// flush is Flush with w.mu held.
func (w *Writer) flush() error {
	if w.err != nil {
		return w.err
	}
	if len(w.pending) == 0 {
		return nil
	}
	out, _, _, _ := w.r.load().redact(nil, w.pending, w.state, false)
	w.pending = w.pending[:0]
	w.state = scanState{}
	return w.emit(out)
}

// emit writes b to the underlying writer and records its first error.
func (w *Writer) emit(b []byte) error {
	n, err := w.w.Write(b)
	if err == nil && n < len(b) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return err
}
