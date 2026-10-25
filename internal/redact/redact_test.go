// SPDX-License-Identifier: MIT

package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

// nonASCII is a non-ASCII letter, written as an escape so that the source
// stays ASCII.
const nonASCII = "\u00e9"

func TestDataSeparation_StringRedactsEverySecret(t *testing.T) {
	const m = Marker
	tests := []struct {
		name    string
		secrets []string
		in      string
		want    string
	}{
		{"no secrets", nil, "user=acme pass=s3cr3t", "user=acme pass=s3cr3t"},
		{"one secret", []string{"s3cr3t"}, "user=acme pass=s3cr3t", "user=acme pass=" + m},
		{"every occurrence", []string{"s3cr3t"}, "s3cr3ts3cr3t s3cr3t", m + m + " " + m},
		{"inside a word", []string{"cr3t"}, "s3cr3ts", "s3" + m + "s"},
		{"several secrets", []string{"k3y9", "t0k9"}, "k3y9 t0k9", m + " " + m},
		{"longest wins", []string{"abcd", "abcdefg"}, "abcdefgh abcdx", m + "h " + m + "x"},
		{"longest wins whatever the order", []string{"abcdefg", "abcd"}, "abcdefgh abcdx", m + "h " + m + "x"},
		{"overlapping secrets merge", []string{"abcd", "cdef"}, "abcdef!", m + "!"},
		{"overlap from inside a longer secret", []string{"abcdefgh", "cdefgh12"}, "abcdefgh12!", m + "!"},
		{"chain of overlaps", []string{"aaab", "abcc", "ccdd"}, "aaabccdd!", m + "!"},
		{"self-overlapping secret", []string{"1212"}, "121212!", m + "!"},
		{"touching occurrences stay separate", []string{"abcd", "efgh"}, "abcdefgh", m + m},
		{"prefix at the end", []string{"s3cr3t"}, "pass=s3cr3", "pass=s3cr3"},
		{"duplicate secrets", []string{"s3cr3t", "s3cr3t"}, "s3cr3t", m},
		{"short secrets ignored", []string{"", "x", "x9", "x9y"}, "a x9y b", "a x9y b"},
		{"invalid UTF-8 secret", []string{"\xff\xfe\xfd\xfc"}, "a\xff\xfe\xfd\xfcb", "a" + m + "b"},
		{"secret equal to Marker", []string{m}, "x" + m, "x" + m},
		{"empty input", []string{"s3cr3t"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.secrets...)
			if got := r.String(tt.in); got != tt.want {
				t.Errorf("String(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if got := string(r.Bytes([]byte(tt.in))); got != tt.want {
				t.Errorf("Bytes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDataSeparation_OverlappingSecretsAreRedactedWhole(t *testing.T) {
	// Without merging, the first secret would win and leave 15 of the 16
	// bytes of the second one visible, and "1212" would leave "12".
	tests := []struct {
		secrets []string
		in      string
	}{
		{[]string{"hunter2", "2fa-backup-77319"}, "hunter2fa-backup-77319"},
		{[]string{"2fa-backup-77319", "hunter2"}, "hunter2fa-backup-77319"},
		{[]string{"1212"}, "121212"},
	}
	for _, tt := range tests {
		r := New(tt.secrets...)
		if got := r.String(tt.in); got != Marker {
			t.Errorf("secrets %q: String(%q) = %q, want %q", tt.secrets, tt.in, got, Marker)
		}
		var buf bytes.Buffer
		writeChunks(t, r.NewWriter(&buf), []byte(tt.in), ones(len(tt.in)))
		if buf.String() != Marker {
			t.Errorf("secrets %q: Writer wrote %q, want %q", tt.secrets, buf.String(), Marker)
		}
	}
}

func TestDataSeparation_AddRejectsShortSecrets(t *testing.T) {
	r := New()
	for _, tt := range []struct {
		secret string
		want   bool
	}{
		{"", false},
		{"a", false},
		{"abc", false},
		{"abcd", true},
		{"abcd", true},              // already registered
		{nonASCII + nonASCII, true}, // four bytes, two runes
	} {
		if got := r.Add(tt.secret); got != tt.want {
			t.Errorf("Add(%q) = %v, want %v", tt.secret, got, tt.want)
		}
	}
	if got, want := r.String("abc abcd"), "abc "+Marker; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	// An encoded form shorter than MinSecretLen is not registered either:
	// the Sanitize form of "\r\n\r\n" is "\n\n".
	r = New("\r\n\r\n")
	if got := r.String("a\n\nb"); got != "a\n\nb" {
		t.Errorf("a two-byte form was registered: String = %q", got)
	}
	if got, want := r.String("a\r\n\r\nb"), "a"+Marker+"b"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestDataSeparation_ZeroRedactorLearnsSecrets(t *testing.T) {
	var r Redactor
	if got := r.String("pass=s3cr3t"); got != "pass=s3cr3t" {
		t.Errorf("zero Redactor: String = %q, want the input unchanged", got)
	}
	r.Add("s3cr3t")
	if got := r.String("pass=s3cr3t"); got != "pass="+Marker {
		t.Errorf("after Add: String = %q, want %q", got, "pass="+Marker)
	}
}

func TestBytesReturnsACopy(t *testing.T) {
	for _, secrets := range [][]string{nil, {"zzzz"}} {
		r := New(secrets...)
		in := []byte("nothing to redact")
		out := r.Bytes(in)
		out[0] = 'N'
		if in[0] != 'n' {
			t.Errorf("secrets %q: Bytes returned its argument, not a copy", secrets)
		}
	}
	if out := New("zzzz").Bytes(nil); len(out) != 0 {
		t.Errorf("Bytes(nil) = %q, want empty", out)
	}
}

// encoder writes a value in one of the encodings a secret may take in
// output, and decodes it back to the text a reader sees.
type encoder struct {
	name   string
	encode func(t *testing.T, s string) string
	decode func(t *testing.T, s string) string
}

var encoders = []encoder{
	{
		name: "JSON without HTML escaping",
		encode: func(t *testing.T, s string) string {
			t.Helper()
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(map[string]string{"value": s}); err != nil {
				t.Fatal(err)
			}
			return buf.String()
		},
		decode: decodeJSON,
	},
	{
		name: "HTML-escaped JSON",
		encode: func(t *testing.T, s string) string {
			t.Helper()
			b, err := json.Marshal(map[string]string{"value": s})
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		},
		decode: decodeJSON,
	},
	{
		name: "%q",
		encode: func(t *testing.T, s string) string {
			t.Helper()
			return fmt.Sprintf("value=%q", s)
		},
		decode: func(t *testing.T, s string) string {
			t.Helper()
			v, err := strconv.Unquote(strings.TrimPrefix(s, "value="))
			if err != nil {
				t.Fatalf("redacted output %q is no longer a quoted string: %v", s, err)
			}
			return v
		},
	},
	{
		// Text read from a device is sanitized before it is printed.
		name:   "Sanitize",
		encode: func(_ *testing.T, s string) string { return Sanitize(s) },
		decode: func(_ *testing.T, s string) string { return s },
	},
}

func decodeJSON(t *testing.T, s string) string {
	t.Helper()
	var v map[string]string
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("redacted output %q is no longer valid JSON: %v", s, err)
	}
	return v["value"]
}

func TestDataSeparation_EncodedFormsAreRedacted(t *testing.T) {
	// Each secret holds characters that at least one encoding escapes, and
	// the encodings differ: "nb sp" (with a no-break space) is escaped only
	// by %q, a carriage return differently by JSON, %q and Sanitize, BEL by
	// all three in three different ways.
	secrets := []string{
		`quo"te`,
		`back\slash`,
		"less<than",
		"more>than",
		"amp&ersand",
		"new\nline",
		"caf" + nonASCII,
		"nb\u00a0sp",
		"bell\a",
		"ab\rcd",
		"all\"\\<>&\n\a\r" + nonASCII + "-in-one",
	}
	for _, enc := range encoders {
		for _, secret := range secrets {
			t.Run(enc.name+"/"+strconv.Quote(secret), func(t *testing.T) {
				r := New(secret)
				encoded := enc.encode(t, "user=acme secret="+secret+" end")
				got := r.String(encoded)
				if want := "user=acme secret=" + Marker + " end"; enc.decode(t, got) != want {
					t.Errorf("redacted %q to %q, which reads %q, want %q",
						encoded, got, enc.decode(t, got), want)
				}
				if strings.Contains(got, secret) {
					t.Errorf("redacted output %q contains the secret", got)
				}
				// The streaming Writer, fed one byte at a time, agrees.
				var buf bytes.Buffer
				w := r.NewWriter(&buf)
				writeChunks(t, w, []byte(encoded), ones(len(encoded)))
				if buf.String() != got {
					t.Errorf("Writer wrote %q, String returned %q", buf.String(), got)
				}
			})
		}
	}
}

func TestDataSeparation_RedactionWinsOverValidJSON(t *testing.T) {
	// Byte-level redaction of encoded JSON may break the document: the
	// documented trade-off, since leak prevention comes first.
	const secret = "1234"
	got := New(secret).String(`{"duration_ms":81234}`)
	if want := `{"duration_ms":8` + Marker + `}`; got != want {
		t.Errorf("String = %s, want %s", got, want)
	}
	// Redacting by value before encoding, as the handler does, keeps the
	// output valid.
	out := logOutput(true, New(secret), func(log *slog.Logger) { log.Info("done", "duration_ms", 81234) })
	if !json.Valid([]byte(out)) || strings.Contains(out, secret) {
		t.Errorf("handler output is invalid JSON or holds the secret: %s", out)
	}
	if v := decodeLine(t, out)["duration_ms"]; v != "8"+Marker {
		t.Errorf("duration_ms = %#v, want %q", v, "8"+Marker)
	}
}

func TestUnquote(t *testing.T) {
	for in, want := range map[string]string{`"a"`: "a", `""`: "", `"`: `"`, "a": "a", `a"`: `a"`} {
		if got := string(unquote([]byte(in))); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}
