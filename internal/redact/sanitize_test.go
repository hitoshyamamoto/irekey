// SPDX-License-Identifier: MIT

package redact

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode"
	"unicode/utf8"
)

// sanitizeTests are shared with FuzzSanitize as seeds. Every non-ASCII
// character is written as an escape.
var sanitizeTests = []struct {
	name string
	in   string
	want string
}{
	{"empty", "", ""},
	{"plain", "plain text 123 ~!@#$%^&*()_+{}|:<>?", "plain text 123 ~!@#$%^&*()_+{}|:<>?"},
	{"newline and tab kept", "line\nnext\tcolumn\n", "line\nnext\tcolumn\n"},
	{"escape sequence", "\x1b[31mred\x1b[0m", `\x1b[31mred\x1b[0m`},
	{"terminal title", "\x1b]0;title\x07", `\x1b]0;title\x07`},
	{"CRLF becomes LF", "a\r\nb\r\n", "a\nb\n"},
	{"lone CR", "a\rb", `a\x0db`},
	{"CR at the end", "end\r", `end\x0d`},
	{"CR before CRLF", "\r\r\n", `\x0d` + "\n"},
	{"C0 controls", "\x00\x01\x08\x0b\x0c\x1f", `\x00\x01\x08\x0b\x0c\x1f`},
	{"DEL", "del\x7f", `del\x7f`},
	{"C1 controls", "\u0080\u0085\u009b\u009f", `\u{0080}\u{0085}\u{009b}\u{009f}`},
	{"spaces kept", "\u00a0\u2009\u202f\u3000", "\u00a0\u2009\u202f\u3000"},
	{"bidi embedding and override", "\u202a\u202b\u202c\u202d\u202e", `\u{202a}\u{202b}\u{202c}\u{202d}\u{202e}`},
	{"bidi isolates", "\u2066\u2067\u2068\u2069", `\u{2066}\u{2067}\u{2068}\u{2069}`},
	{"bidi marks", "\u200e\u200f\u061c", `\u{200e}\u{200f}\u{061c}`},
	{"zero-width characters", "\u200b\u200c\u200d\u2060", `\u{200b}\u{200c}\u{200d}\u{2060}`},
	{"invisible operators", "\u2061\u2064", `\u{2061}\u{2064}`},
	{"deprecated format characters", "\u206a\u206f", `\u{206a}\u{206f}`},
	{"byte order mark", "\ufeffbom", `\u{feff}bom`},
	{"soft hyphen", "soft\u00adhyphen", `soft\u{00ad}hyphen`},
	{"Mongolian vowel separator", "\u180e", `\u{180e}`},
	{"interlinear annotation", "\ufff9\ufffb", `\u{fff9}\u{fffb}`},
	{"tag characters", "\U000e0001\U000e0041\U000e007f", `\u{e0001}\u{e0041}\u{e007f}`},
	{"line and paragraph separators", "a\u2028b\u2029c", `a\u{2028}b\u{2029}c`},
	{"private use", "\ue000\U000f0000\U0010fffd", `\u{e000}\u{f0000}\u{10fffd}`},
	{"unassigned", "\u0378\u2065", `\u{0378}\u{2065}`},
	{"noncharacters", "\ufdd0\ufdef\ufffe\uffff\U0001fffe\U0010ffff", `\u{fdd0}\u{fdef}\u{fffe}\u{ffff}\u{1fffe}\u{10ffff}`},
	{"invalid byte", "a\xffb", `a\xffb`},
	{"lone C1 byte", "\x9b", `\x9b`},
	{"truncated sequence", "ab\xc3", `ab\xc3`},
	{"overlong encoding", "\xc0\x80", `\xc0\x80`},
	{"encoded surrogate", "\xed\xa0\x80", `\xed\xa0\x80`},
	{"replacement character kept", "\ufffd", "\ufffd"},
	{"non-ASCII letters kept", "caf" + nonASCII + " \u4e16", "caf" + nonASCII + " \u4e16"},
	{"emoji kept", "\U0001f600", "\U0001f600"},
	{"escape text kept", `\x1b \u{202e}`, `\x1b \u{202e}`},
}

func TestSanitize(t *testing.T) {
	for _, tt := range sanitizeTests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sanitize(tt.in)
			if got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
			checkSanitized(t, tt.in, got)
		})
	}
}

func TestSanitizeEscapesInvisibleCharacters(t *testing.T) {
	// Every rune that a terminal does not show as itself is escaped.
	for _, r := range []rune{0x200b, 0x200e, 0x200f, 0x061c, 0x2028, 0x2029, 0xfeff, 0x00ad, 0xe0041} {
		if out := Sanitize(string(r)); out == string(r) {
			t.Errorf("Sanitize keeps %U unescaped", r)
		}
	}
}

// allowed reports whether r may reach a terminal unescaped: newline, tab
// and the graphic characters, as package unicode defines them, except the
// noncharacters (which package unicode does not count as graphic either).
// It is an oracle independent of the category list in Sanitize.
func allowed(r rune) bool {
	noncharacter := (r >= 0xfdd0 && r <= 0xfdef) || r&0xfffe == 0xfffe
	return r == '\n' || r == '\t' || (unicode.IsGraphic(r) && !noncharacter)
}

// clean reports whether s is valid UTF-8 made only of allowed runes.
func clean(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !allowed(r) {
			return false
		}
	}
	return true
}

// checkSanitized checks the properties of out = Sanitize(in): out is valid
// UTF-8 and clean, sanitizing it again changes nothing, and text that was
// clean already is not changed.
func checkSanitized(t *testing.T, in, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Errorf("Sanitize(%q) = %q, which is not valid UTF-8", in, out)
	}
	for _, r := range out {
		if !allowed(r) {
			t.Errorf("Sanitize(%q) = %q, which contains %U", in, out, r)
		}
	}
	if again := Sanitize(out); again != out {
		t.Errorf("Sanitize is not idempotent: Sanitize(%q) = %q, Sanitize of that = %q", in, out, again)
	}
	if clean(in) && out != in {
		t.Errorf("Sanitize(%q) = %q, but the input was clean", in, out)
	}
}

func TestSanitizeIsIdempotent(t *testing.T) {
	// Random bytes cover invalid UTF-8; random strings cover runes.
	ok := func(in string) bool {
		out := Sanitize(in)
		return clean(out) && Sanitize(out) == out && (!clean(in) || out == in)
	}
	fromBytes := func(b []byte) bool { return ok(string(b)) }
	fromString := func(s string) bool { return ok(s) }
	for _, prop := range []any{fromBytes, fromString} {
		if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
			t.Error(err)
		}
	}
}

func TestEscapeRune(t *testing.T) {
	for r, want := range map[rune]string{
		0x80: `\u{0080}`, 0x202e: `\u{202e}`, 0xffff: `\u{ffff}`,
		0x10000: `\u{10000}`, 0xe0041: `\u{e0041}`, 0x10ffff: `\u{10ffff}`,
	} {
		var b strings.Builder
		escapeRune(&b, r)
		if b.String() != want {
			t.Errorf("escapeRune(%U) wrote %s, want %s", r, b.String(), want)
		}
	}
}
