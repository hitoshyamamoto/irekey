// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestLineLanguage(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string // substring of the message; "" for no finding
	}{
		{"ascii", "plain English text, 100% ASCII", ""},
		{"em dash", "irekey \u2014 inspect, adjust, deliver", ""},
		{"arrows", "cmd \u2192 cli \u25ba app \u2190", ""},
		{"box drawing", "\u251c\u2500\u2500 cmd/", ""},
		{"math signs", "a \u2264 b \u00d7 c \u2265 d", ""},
		{"warning sign", "\u26a0 careful", ""},
		{"degree and section signs", "20 \u00b0C, see \u00a7 4", ""},
		{"accented letter", "abc" + eAcute, "U+00E9"},
		{"escaped accented letter", "x\u00efy", "U+00EF"},
		{"cyrillic", "\u0430", "U+0430"},
		{"kana", "\u3042", "U+3042"},
		{"letter after symbols", "\u2014 \u00e7", "U+00E7"},
		{"invalid UTF-8", "abc" + string([]byte{0xe9}), "invalid UTF-8 byte 0xE9"},
		{"truncated sequence", string([]byte{0xc3}), "invalid UTF-8"},
		{"decomposed accent", "abce\u0301", "combining mark U+0301"},
		{"decomposed accent, upper case", "E\u0300", "combining mark U+0300"},
		{"emoji with a variation selector", "\u26a0\ufe0f careful", ""},
		{"keycap", "1\ufe0f\u20e3", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineLanguage(tt.line)
			if (got == "") != (tt.want == "") || !strings.Contains(got, tt.want) {
				t.Errorf("lineLanguage(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestLanguageExempt(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docs/spec/masterprompt.md", true},
		{"README.md", true},
		{"internal/redact/testdata/utf8_names.txt", true},
		{"testdata/compat/utf8.json", true},
		{"docs/README.md", false},
		{"docs/glossary.md", false},
		{"internal/redact/testdata/names.txt", false},
		{"internal/utf8/names.go", false},
		{"utf8.txt", false},
	}
	for _, tt := range tests {
		if got := languageExempt(tt.path); got != tt.want {
			t.Errorf("languageExempt(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCheckLanguage_Tree(t *testing.T) {
	word := "x" + string([]byte{0xc3, 0xa1}) + "y" // U+00E1, a letter with an acute accent
	root := writeTree(t, map[string]string{
		"README.md":                          word + "\n",
		"docs/spec/masterprompt.md":          word + "\n",
		"internal/x/testdata/utf8_input.txt": word + "\n",
		"docs/guide.md":                      "line one\n" + word + "\n",
		"internal/x/x.go":                    goFile("// " + word),
		"symbols.md":                         "a \u2192 b \u2014 c\n",
	})
	code, out := runGuard(t, "language", "-root", root)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, exitFindings, out)
	}
	for _, want := range []string{
		"docs/guide.md:2: non-ASCII letter U+00E1",
		"internal/x/x.go:5: ",
		"error: language guard: 2 finding(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestCheckLanguage_MissingRoot(t *testing.T) {
	if _, err := checkLanguage(t.TempDir() + "/missing"); err == nil {
		t.Error("want an error for a missing root")
	}
}
