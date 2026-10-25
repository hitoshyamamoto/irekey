// SPDX-License-Identifier: MIT

package redact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"
)

// credential logs as a group that holds its token.
type credential struct{ token string }

func (c credential) LogValue() slog.Value {
	return slog.GroupValue(slog.String("token", c.token), slog.Int("n", 7))
}

// session prints its token in its String text.
type session struct{ token string }

func (s session) String() string { return "session of acme with " + s.token }

// panicky panics in String when it is nil.
type panicky struct{ s string }

func (p *panicky) String() string { return p.s }

// rawBytes is a named byte slice type.
type rawBytes []byte

// login is a plain struct; fmt prints its fields.
type login struct{ User, Pass string }

// auth and config hold a secret behind a pointer, which fmt prints as an
// address and encoding/json follows.
type auth struct{ Pass string }

type config struct {
	Host string
	Auth *auth
}

// textOnly is printed through MarshalText by both stock handlers.
type textOnly struct{ b []byte }

func (s textOnly) MarshalText() ([]byte, error) { return s.b, nil }

// textNotJSON shows its secret through MarshalText, which the text handler
// uses, and hides it from MarshalJSON and fmt.
type textNotJSON struct{ b []byte }

func (s textNotJSON) MarshalText() ([]byte, error) { return s.b, nil }

func (textNotJSON) MarshalJSON() ([]byte, error) { return []byte(`"hidden"`), nil }

// appendOnly shows its secret only through AppendText, which the text
// handler uses: fmt prints the bytes as numbers, and MarshalJSON, which
// encoding/json would otherwise build from AppendText, hides it.
type appendOnly struct{ b []byte }

func (a appendOnly) AppendText(b []byte) ([]byte, error) { return append(b, a.b...), nil }

func (appendOnly) MarshalJSON() ([]byte, error) { return []byte(`"hidden"`), nil }

// verbose prints its secret only under %+v, which the text handler uses.
type verbose struct{ s string }

// Format implements fmt.Formatter, which has no way to report a write
// error.
func (v verbose) Format(f fmt.State, _ rune) {
	if f.Flag('+') {
		_, _ = fmt.Fprintf(f, "{secret:%s}", v.s)
		return
	}
	_, _ = io.WriteString(f, "verbose")
}

// pointerValuer resolves to a pointer.
type pointerValuer struct{ p *string }

func (v pointerValuer) LogValue() slog.Value { return slog.AnyValue(v.p) }

// jsonOnly prints its secret only through MarshalJSON.
type jsonOnly struct{ s string }

func (j jsonOnly) MarshalJSON() ([]byte, error) { return json.Marshal(j.s) }

// jsonFails puts its secret in the error of MarshalJSON, which the JSON
// handler prints.
type jsonFails struct{ s string }

func (j jsonFails) MarshalJSON() ([]byte, error) { return nil, errors.New("cannot encode " + j.s) }

// jsonPanics panics in MarshalJSON.
type jsonPanics struct{}

func (jsonPanics) MarshalJSON() ([]byte, error) { panic("MarshalJSON") }

// noTime drops the time, so that it cannot spell a secret by chance.
func noTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

// logOutput logs with f through a redacting handler in front of the text
// handler of log/slog, or its JSON handler with asJSON, and returns the
// output.
func logOutput(asJSON bool, r *Redactor, f func(*slog.Logger)) string {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{ReplaceAttr: noTime}
	var next slog.Handler = slog.NewTextHandler(&buf, opts)
	if asJSON {
		next = slog.NewJSONHandler(&buf, opts)
	}
	f(slog.New(NewHandler(r, next)))
	return buf.String()
}

// handlerNames names the two stock handlers for logOutput.
var handlerNames = map[bool]string{false: "text", true: "JSON"}

// decodeLine decodes one JSON log line.
func decodeLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, line)
	}
	return m
}

// group returns the group value m[key], or nil.
func group(m map[string]any, key string) map[string]any {
	g, _ := m[key].(map[string]any)
	return g
}

// checkNoForms fails when out holds secret, verbatim or in an encoding
// a handler applies.
func checkNoForms(t *testing.T, out, secret string) {
	t.Helper()
	html, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []string{secret, strconv.Quote(secret), jsonString(t, secret), string(html)} {
		if strings.Contains(out, strings.Trim(form, `"`)) {
			t.Errorf("output holds %s:\n%s", form, out)
		}
	}
}

// jsonString returns s as encoding/json writes it, without HTML escaping.
func jsonString(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// decodedText returns every key and string value of a JSON object, decoded
// and joined.
func decodedText(t *testing.T, line string) string {
	t.Helper()
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(x + "\x00")
		case map[string]any:
			for k, e := range x {
				b.WriteString(k + "\x00")
				walk(e)
			}
		}
	}
	walk(decodeLine(t, line))
	return b.String()
}

// logEverything logs a secret in every place the handler covers.
func logEverything(log *slog.Logger, secret, pin string) {
	log = log.With("with", secret)
	log.Info("login with "+secret,
		"password", secret,
		slog.Group("g", "token", secret),
		"err", errors.New("dial: "+secret),
		"valuer", credential{secret},
		"stringer", session{secret},
		"nilptr", (*panicky)(nil),
		"raw", []byte(secret),
		"named", rawBytes(secret),
		"login", login{"acme", secret},
		"kept", login{"acme", "none"},
		"pin", mustAtoi(pin),
		"n", 7,
		"ratio", 0.5,
		"plain", "visible",
		"secret-"+pin, "key",
	)
	log.WithGroup("grp").Info("second", "inner", secret)
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

func TestDataSeparation_SlogHandler(t *testing.T) {
	// The secret holds characters that the JSON and text handlers escape.
	const secret = `pa"ss<w>rd&` + "\n" + nonASCII
	const pin = "424242"
	checks := map[bool]func(t *testing.T, out string){
		false: func(t *testing.T, out string) {
			t.Helper()
			for _, want := range []string{
				`msg="login with [REDACTED]"`, " with=[REDACTED] ", " password=[REDACTED] ",
				" g.token=[REDACTED] ", ` err="dial: [REDACTED]" `, " valuer.token=[REDACTED] ",
				" valuer.n=7 ", ` stringer="session of acme with [REDACTED]" `, " nilptr=<nil> ",
				" raw=[REDACTED] ", " named=[REDACTED] ", ` login="{User:acme Pass:[REDACTED]}" `,
				` kept="{User:acme Pass:none}" `, " pin=[REDACTED] ", " n=7 ", " ratio=0.5 ",
				" plain=visible ", " secret-[REDACTED]=key", " grp.inner=[REDACTED]",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		},
		true: func(t *testing.T, out string) {
			t.Helper()
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("got %d lines, want 2:\n%s", len(lines), out)
			}
			first, second := decodeLine(t, lines[0]), decodeLine(t, lines[1])
			for _, c := range []struct {
				got  any
				want any
			}{
				{first["msg"], "login with " + Marker},
				{first["with"], Marker},
				{first["password"], Marker},
				{group(first, "g")["token"], Marker},
				{first["err"], "dial: " + Marker},
				{group(first, "valuer")["token"], Marker},
				{group(first, "valuer")["n"], 7.0},
				{first["stringer"], "session of acme with " + Marker},
				{first["nilptr"], "<nil>"},
				{first["raw"], Marker},
				{first["named"], Marker},
				{first["login"], "{User:acme Pass:" + Marker + "}"},
				{group(first, "kept")["Pass"], "none"}, // kept as a struct
				{first["pin"], Marker},
				{first["n"], 7.0},
				{first["ratio"], 0.5},
				{first["plain"], "visible"},
				{first["secret-"+Marker], "key"},
				{second["with"], Marker},
				{group(second, "grp")["inner"], Marker},
			} {
				if c.got != c.want {
					t.Errorf("got %#v, want %#v:\n%s", c.got, c.want, out)
				}
			}
			// Decoding undoes the escaping, so no form of the secret
			// hides in a decoded value.
			for _, line := range lines {
				if strings.Contains(decodedText(t, line), secret) {
					t.Errorf("a decoded value holds the secret: %s", line)
				}
			}
		},
	}
	for _, asJSON := range []bool{false, true} {
		t.Run(handlerNames[asJSON], func(t *testing.T) {
			out := logOutput(asJSON, New(secret, pin), func(log *slog.Logger) { logEverything(log, secret, pin) })
			checkNoForms(t, out, secret)
			checkNoForms(t, out, pin)
			checks[asJSON](t, out)
		})
	}
}

func TestDataSeparation_SlogHandlerChecksEveryRendering(t *testing.T) {
	// Each value prints its secret in a rendering other than fmt's %v,
	// through at least one of the stock handlers.
	const pw = "hunter2-Zq9"
	p := pw
	tests := []struct {
		name   string
		secret string
		value  any
	}{
		{"pointer to string", pw, &p},
		{"pointer in a struct", pw, config{"h", &auth{pw}}},
		{"slice of pointers", pw, []*auth{{pw}}},
		{"map of pointers", pw, map[string]*string{"pw": &p}},
		{"TextMarshaler", pw, textOnly{[]byte(pw)}},
		{"TextMarshaler with its own MarshalJSON", pw, textNotJSON{[]byte(pw)}},
		{"TextAppender", pw, appendOnly{[]byte(pw)}},
		{"Formatter printing more under %+v", pw, verbose{pw}},
		{"LogValuer resolving to a pointer", pw, pointerValuer{&p}},
		{"MarshalJSON", pw, jsonOnly{pw}},
		{"MarshalJSON error", pw, jsonFails{pw}},
		{"float in JSON form", "4242424242", float64(4242424242)},
		{"duration in nanoseconds", "4242424242", time.Duration(4242424242)},
		// The text handler prints Duration.String, the JSON handler the
		// nanoseconds.
		{"duration in text form", "234567s", time.Duration(1234567000)},
		// The JSON handler prints RFC 3339 with nanoseconds, the text
		// handler with milliseconds; neither secret is in Time.String.
		{"time in JSON form", "T04:24:24.242000001", time.Date(2026, 10, 25, 4, 24, 24, 242000001, time.UTC)},
		{"time in text form", "24:24.242Z", time.Date(2026, 10, 25, 4, 24, 24, 242000001, time.UTC)},
	}
	for _, tt := range tests {
		for _, asJSON := range []bool{false, true} {
			t.Run(tt.name+"/"+handlerNames[asJSON], func(t *testing.T) {
				out := logOutput(asJSON, New(tt.secret), func(log *slog.Logger) { log.Info("m", "v", tt.value) })
				checkNoForms(t, out, tt.secret)
				if !strings.Contains(out, Marker) {
					t.Errorf("output lacks %s:\n%s", Marker, out)
				}
			})
		}
	}
	// Renderings that only fmt and other handlers print: %v of a struct
	// (the stock handlers print %+v or JSON) and Time.String. The value
	// next receives must already be the redacted text.
	for _, tt := range []struct {
		name   string
		secret string
		value  any
	}{
		{"%v only", "hunt er2x", twoWords{"hunt", "er2x"}},
		{"Time.String only", "04:24:24.242000001 +0000", time.Date(2026, 10, 25, 4, 24, 24, 242000001, time.UTC)},
	} {
		next := &capture{}
		rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "m", 0)
		rec.AddAttrs(slog.Any("v", tt.value))
		if err := NewHandler(New(tt.secret), next).Handle(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
		if len(next.attrs) != 1 {
			t.Fatalf("%s: next received %d attributes, want 1", tt.name, len(next.attrs))
		}
		v := next.attrs[0].Value
		if v.Kind() != slog.KindString || strings.Contains(v.String(), tt.secret) || !strings.Contains(v.String(), Marker) {
			t.Errorf("%s: next received %v %q, want a redacted string", tt.name, v.Kind(), v.String())
		}
	}
	// Without a secret in any rendering, values keep their kind.
	out := logOutput(true, New(pw), func(log *slog.Logger) {
		log.Info("m", "cfg", config{"h", &auth{"none"}}, "d", time.Second, "f", 1.5)
	})
	m := decodeLine(t, out)
	if group(group(m, "cfg"), "Auth")["Pass"] != "none" || m["d"] != 1e9 || m["f"] != 1.5 {
		t.Errorf("values without a secret were changed: %s", out)
	}
	// A secret equal to Marker is recognized as a secret: the value is
	// replaced, not printed in base64 by the JSON handler.
	out = logOutput(true, New(Marker), func(log *slog.Logger) { log.Info("m", "v", []byte(Marker)) })
	if v := decodeLine(t, out)["v"]; v != Marker {
		t.Errorf("v = %#v, want %q", v, Marker)
	}
	// A panic in MarshalJSON is not the text handler's business.
	if out := logOutput(false, New(pw), func(log *slog.Logger) { log.Info("m", "v", jsonPanics{}) }); !strings.Contains(out, " v={}") {
		t.Errorf("output = %s, want v={}", out)
	}
}

func TestDataSeparation_SlogHandlerRedactsGroupNames(t *testing.T) {
	const pw = "hunter2-Zq9"
	for _, asJSON := range []bool{false, true} {
		out := logOutput(asJSON, New(pw), func(log *slog.Logger) { log.WithGroup(pw).Info("m", "a", 1) })
		checkNoForms(t, out, pw)
		if !strings.Contains(out, Marker) {
			t.Errorf("%s: output lacks %s:\n%s", handlerNames[asJSON], Marker, out)
		}
	}
}

func TestDataSeparation_SlogHandlerRechecksAttrsAfterAdd(t *testing.T) {
	// The stock handlers format attributes added with With once, when With
	// is called; the redacting handler keeps them and checks them against
	// the secrets known when each record is handled.
	const tok, grp = "tok-7f3a9c", "grp-5e1d"
	for _, asJSON := range []bool{false, true} {
		r := New()
		out := logOutput(asJSON, r, func(log *slog.Logger) {
			log = log.With("token", tok).WithGroup(grp).With("inner", tok)
			r.Add(tok)
			r.Add(grp)
			log.Info("printed after Add", "a", 1)
		})
		checkNoForms(t, out, tok)
		checkNoForms(t, out, grp)
		if asJSON {
			m := decodeLine(t, out)
			if m["token"] != Marker || group(m, Marker)["inner"] != Marker || group(m, Marker)["a"] != 1.0 {
				t.Errorf("JSON output = %s", out)
			}
			continue
		}
		if want := " token=[REDACTED] [REDACTED].inner=[REDACTED] [REDACTED].a=1"; !strings.Contains(out, want) {
			t.Errorf("text output = %s, want it to contain %q", out, want)
		}
	}
}

// twoWords prints its fields separated by a space only under %v.
type twoWords struct{ A, B string }

// capture records the calls it receives.
type capture struct {
	attrs []slog.Attr
	calls []string
}

func (c *capture) Enabled(context.Context, slog.Level) bool { return true }

func (c *capture) Handle(_ context.Context, r slog.Record) error {
	c.calls = append(c.calls, "Handle "+r.Message)
	r.Attrs(func(a slog.Attr) bool {
		c.attrs = append(c.attrs, a)
		return true
	})
	return nil
}

func (c *capture) WithAttrs([]slog.Attr) slog.Handler {
	c.calls = append(c.calls, "WithAttrs")
	return c
}

func (c *capture) WithGroup(string) slog.Handler {
	c.calls = append(c.calls, "WithGroup")
	return c
}

func TestDataSeparation_SlogHandlerRedactsBeforeDelegating(t *testing.T) {
	const secret = "k3y-for-acme"
	next := &capture{}
	h := NewHandler(New(secret), next)
	if h.WithGroup("") != h || h.WithAttrs(nil) != h {
		t.Error(`WithGroup("") or WithAttrs(nil) did not return the handler itself`)
	}
	h = h.WithGroup("outer").WithAttrs([]slog.Attr{slog.String("a", secret)})
	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled did not delegate")
	}
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, secret, 0)
	rec.AddAttrs(slog.Any("b", credential{secret}), slog.Bool("ok", true), slog.Any("err", errors.New("plain")))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	// next receives only the record, with the groups and attributes of
	// the handler attached and redacted.
	if got, want := strings.Join(next.calls, ","), "Handle "+Marker; got != want {
		t.Errorf("next received %s, want %s", got, want)
	}
	got := slog.GroupValue(next.attrs...).String()
	if want := "[outer=[a=[REDACTED] b=[token=[REDACTED] n=7] ok=true err=plain]]"; got != want {
		t.Errorf("next received %s, want %s", got, want)
	}
	// An error reaches next as its text, never as the error value, which
	// next could format in a way the handler does not check.
	if len(next.attrs) == 1 {
		for _, a := range next.attrs[0].Value.Group() {
			if a.Key == "err" && a.Value.Kind() != slog.KindString {
				t.Errorf("err reached next as %s, want a string", a.Value.Kind())
			}
		}
	}
}
