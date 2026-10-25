// SPDX-License-Identifier: MIT

package redact

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"time"
)

// handler is the log/slog handler returned by NewHandler.
type handler struct {
	r    *Redactor
	next slog.Handler
	// frames holds what WithAttrs and WithGroup added, outermost first. The
	// first frame has no group. The slice and its frames are never modified
	// after the handler is made.
	frames []frame
}

// frame is a group opened by WithGroup, with the attributes added to it by
// WithAttrs.
type frame struct {
	group string
	attrs []slog.Attr
}

// NewHandler returns a log/slog handler that redacts r's secrets from each
// record and passes the record to next.
//
// The message, every key, every group name and every value are redacted, at
// the time each record is handled, against the secrets registered then.
// Attributes added with WithAttrs and groups opened with WithGroup are
// therefore kept by the handler itself and attached to each record as
// groups and attributes, instead of being handed to next, which would format
// them once, before a later secret is known. next never sees a WithAttrs or
// WithGroup call.
//
// Values are redacted recursively, after resolving [slog.LogValuer] values:
//
//   - strings and the members of groups are redacted;
//   - an error becomes its Error text and a [fmt.Stringer] its String text,
//     redacted;
//   - any other value is kept as it is unless one of the texts next may
//     print for it contains a secret; it is then replaced by the redacted
//     text of the first such rendering.
//
// The renderings checked are those of the text and JSON handlers of
// log/slog and of fmt: for numbers, durations and times, the text of
// Value.String and the forms the two handlers print (a float as
// encoding/json writes it, a duration in nanoseconds, a time in RFC 3339
// with nanoseconds and with milliseconds); for other values, in this
// order, the output of encoding.TextAppender and encoding.TextMarshaler,
// the text of a byte slice, fmt's %+v and %v, and the JSON encoding (which
// follows pointers and calls MarshalJSON), including the text of a
// marshaling error. A handler
// that renders values in yet another way (%#v, for example) is not covered;
// for defense in depth, also give next a [Writer] of r as its output.
func NewHandler(r *Redactor, next slog.Handler) slog.Handler {
	return &handler{r: r, next: next, frames: []frame{{}}}
}

// Enabled reports whether next handles records at level.
func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle passes to next a copy of rec with its message redacted and with the
// handler's groups and attributes, followed by rec's attributes, all
// redacted.
func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	attrs := make([]slog.Attr, 0, rec.NumAttrs())
	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.r.attr(a))
		return true
	})
	for i := len(h.frames) - 1; i >= 0; i-- {
		f := h.frames[i]
		red := make([]slog.Attr, 0, len(f.attrs)+len(attrs))
		for _, a := range f.attrs {
			red = append(red, h.r.attr(a))
		}
		red = append(red, attrs...)
		if i == 0 {
			attrs = red
			break
		}
		attrs = []slog.Attr{{Key: h.r.String(f.group), Value: slog.GroupValue(red...)}}
	}
	out := slog.NewRecord(rec.Time, rec.Level, h.r.String(rec.Message), rec.PC)
	out.AddAttrs(attrs...)
	return h.next.Handle(ctx, out)
}

// WithAttrs returns a handler that adds attrs to every record, in the
// innermost group. They are redacted when each record is handled.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	frames := slices.Clone(h.frames)
	last := &frames[len(frames)-1]
	last.attrs = slices.Concat(last.attrs, attrs)
	return &handler{r: h.r, next: h.next, frames: frames}
}

// WithGroup returns a handler that puts the attributes that follow in a
// group named name. The name is redacted when each record is handled.
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	frames := append(slices.Clip(h.frames), frame{group: name})
	return &handler{r: h.r, next: h.next, frames: frames}
}

// attr returns a with its key and value redacted.
func (r *Redactor) attr(a slog.Attr) slog.Attr {
	if r.load().count == 0 {
		return a
	}
	return slog.Attr{Key: r.String(a.Key), Value: r.value(a.Value)}
}

// value returns v, resolved, with every secret replaced by [Marker].
//
// Safety (DataSeparation): next prints a value through one of its
// renderings, which may differ from fmt's (json.Marshal follows pointers,
// the text handler prefers MarshalText), so every rendering is checked. A
// value whose renderings hold no secret keeps its kind, so numbers stay
// numbers in JSON output.
func (r *Redactor) value(v slog.Value) slog.Value {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(r.String(v.String()))
	case slog.KindGroup:
		as := v.Group()
		red := make([]slog.Attr, len(as))
		for i, a := range as {
			red[i] = r.attr(a)
		}
		return slog.GroupValue(red...)
	case slog.KindAny:
		a := v.Any()
		_, isError := a.(error)
		_, isStringer := a.(fmt.Stringer)
		if isError || isStringer {
			// fmt calls Error or String and recovers from a panic in
			// them, for example on a nil pointer.
			return slog.StringValue(r.String(fmt.Sprint(a)))
		}
		return r.ifSecret(v, anyRenderings(a))
	case slog.KindFloat64:
		f := v.Float64()
		return r.ifSecret(v, []string{v.String(), strconv.FormatFloat(f, 'g', -1, 64), jsonText(f)})
	case slog.KindDuration:
		return r.ifSecret(v, []string{v.String(), strconv.FormatInt(int64(v.Duration()), 10)})
	case slog.KindTime:
		t := v.Time()
		return r.ifSecret(v, []string{v.String(), t.Format(time.RFC3339Nano), t.Format(rfc3339Millis)})
	case slog.KindBool, slog.KindInt64, slog.KindUint64, slog.KindLogValuer:
		// Both stock handlers print these as v.String() does. Resolve
		// never returns a LogValuer; the case is here for completeness.
		return r.ifSecret(v, []string{v.String()})
	}
	return r.ifSecret(v, []string{v.String()})
}

// rfc3339Millis is the time format of the text handler of log/slog.
const rfc3339Millis = "2006-01-02T15:04:05.000Z07:00"

// ifSecret returns v unchanged when none of texts holds a secret, and
// otherwise the redacted form of the first text that does.
func (r *Redactor) ifSecret(v slog.Value, texts []string) slog.Value {
	for _, t := range texts {
		if red, found := r.redactString(t); found {
			return slog.StringValue(red)
		}
	}
	return v
}

// anyRenderings returns the texts a handler may print for a, in order of
// preference for the replacement.
func anyRenderings(a any) []string {
	var out []string
	if ta, ok := a.(encoding.TextAppender); ok {
		out = append(out, safeText(func() ([]byte, error) { return ta.AppendText(nil) }))
	}
	if tm, ok := a.(encoding.TextMarshaler); ok {
		out = append(out, safeText(tm.MarshalText))
	}
	// The text handler prints a byte slice, of any named type, as the text
	// it holds; fmt prints its numbers.
	if t := reflect.TypeOf(a); t != nil && t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		out = append(out, string(reflect.ValueOf(a).Bytes()))
	}
	return append(out, fmt.Sprintf("%+v", a), fmt.Sprint(a), jsonText(a))
}

// jsonText returns a as the JSON handler of log/slog encodes it, or the
// text of the error that the handler prints instead.
func jsonText(a any) string {
	return safeText(func() ([]byte, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		err := enc.Encode(a)
		return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), err
	})
}

// safeText calls f, which renders a value, and returns its text, or the
// text of its error, which handlers print in its place. A panic in f yields
// "": a handler that renders the value the same way panics too, and prints
// nothing of it.
func safeText(f func() ([]byte, error)) (text string) {
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()
	b, err := f()
	if err != nil {
		return err.Error()
	}
	return string(b)
}
