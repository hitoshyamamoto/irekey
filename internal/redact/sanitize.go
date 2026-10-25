// SPDX-License-Identifier: MIT

package redact

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize makes untrusted text, such as output read from a device, safe to
// print on a terminal. It keeps newline, tab and every graphic character
// (letters, marks, numbers, punctuation, symbols and spaces: Unicode
// categories L, M, N, P, S and Zs, as [unicode.IsGraphic] defines them),
// turns "\r\n" into "\n", and writes everything else as a visible escape:
//
//   - an ASCII control other than newline and tab (including a carriage
//     return not followed by a newline), DEL, and a byte that is not part
//     of valid UTF-8 become \xNN, for example \x1b and \xff;
//   - every other rune becomes \u{NNNN}, with at least four lowercase hex
//     digits, for example \u{202e} and \u{e0041}. These are the C1
//     controls (category Cc), the format characters (Cf: among them the
//     bidirectional text controls, zero-width characters, the byte order
//     mark, the soft hyphen and the tag characters), the line and paragraph
//     separators (Zl, Zp), private-use characters (Co), and code points
//     unassigned in the Unicode version of package unicode (Cn), which
//     include the noncharacters such as U+FFFF.
//
// The result is valid UTF-8 and contains only newline, tab and graphic
// characters, and Sanitize(Sanitize(s)) == Sanitize(s). Escaping is for
// display and is not reversible: a backslash in s is kept as it is, so text
// that already reads \x1b looks the same as an escaped ESC character.
//
// Escaping was chosen over stripping: there is no escape-sequence parser to
// get wrong, nothing is lost for diagnostics, and a malformed sequence cannot
// slip through half removed.
func Sanitize(s string) string {
	i := 0
	for i < len(s) {
		size, ok := safeAt(s, i)
		if !ok {
			break
		}
		i += size
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	b.WriteString(s[:i])
	for i < len(s) {
		size, ok := safeAt(s, i)
		switch {
		case ok:
			b.WriteString(s[i : i+size])
		case s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n':
			// Dropped; the newline that follows is kept.
		case size == 1:
			escapeByte(&b, s[i])
		default:
			r, _ := utf8.DecodeRuneInString(s[i:])
			escapeRune(&b, r)
		}
		i += size
	}
	return b.String()
}

// safeAt returns the length of the character at s[i], or 1 for an invalid
// UTF-8 byte, and whether it can be printed as it is.
func safeAt(s string, i int) (size int, ok bool) {
	c := s[i]
	if c < utf8.RuneSelf {
		return 1, c == '\n' || c == '\t' || (c >= ' ' && c != 0x7f)
	}
	r, size := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && size == 1 {
		return 1, false
	}
	return size, !unsafeRune(r)
}

// unsafeRune reports whether a valid non-ASCII rune must be escaped. The
// categories listed are exactly the runes that are not graphic.
func unsafeRune(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Co, unicode.Cn)
}

const hexDigits = "0123456789abcdef"

// escapeByte writes c as \xNN.
func escapeByte(b *strings.Builder, c byte) {
	b.WriteString(`\x`)
	b.WriteByte(hexDigits[c>>4])
	b.WriteByte(hexDigits[c&0xf])
}

// escapeRune writes r as \u{NNNN}, with at least four hex digits and as
// many as r needs, up to six for U+10FFFF.
func escapeRune(b *strings.Builder, r rune) {
	b.WriteString(`\u{`)
	shift := 12
	for shift < 20 && r>>(shift+4) != 0 {
		shift += 4
	}
	for ; shift >= 0; shift -= 4 {
		b.WriteByte(hexDigits[(r>>shift)&0xf])
	}
	b.WriteByte('}')
}
