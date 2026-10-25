// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// checkLanguage fails on non-ASCII letters, which would be words of
// another natural language: the repository is English only. Punctuation
// and symbols (dashes, arrows, box drawing, math signs) are not letters
// and are allowed. Also reported: a combining mark on an ASCII letter (an
// accented letter in decomposed form), and text that is not valid UTF-8,
// which usually hides letters in a legacy encoding.
func checkLanguage(root string) ([]finding, error) {
	files, err := loadTextFiles(root, func(p string) bool { return !languageExempt(p) })
	if err != nil {
		return nil, err
	}
	var out []finding
	for _, f := range files {
		out = append(out, languageFindings(f.path, f.data)...)
	}
	return out, nil
}

// languageExempt lists the files that may contain non-ASCII letters: the
// specification and the README, which explain the Japanese terms behind
// the project's name, and UTF-8 test fixtures.
func languageExempt(p string) bool {
	switch {
	case p == "docs/spec/masterprompt.md", p == "README.md":
		return true
	case inTestdata(p) && strings.Contains(path.Base(p), "utf8"):
		return true
	}
	return false
}

func languageFindings(p string, data []byte) []finding {
	var out []finding
	for i, line := range splitLines(data) {
		if msg := lineLanguage(line); msg != "" {
			out = append(out, finding{p, i + 1, msg})
		}
	}
	return out
}

// lineLanguage describes the first problem in a line, or returns "".
func lineLanguage(line string) string {
	prev := rune(0)
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			return fmt.Sprintf("invalid UTF-8 byte 0x%02X (text files must be UTF-8)", line[i])
		case r > unicode.MaxASCII && unicode.IsLetter(r):
			return fmt.Sprintf("non-ASCII letter %U %q (the repository is English only, in ASCII letters)", r, r)
		case unicode.Is(unicode.Mn, r) && isASCIILetter(prev):
			// A decomposed accented letter: an ASCII letter followed by a
			// combining mark. Marks after symbols, such as the variation
			// selector of an emoji, are allowed.
			return fmt.Sprintf("combining mark %U after the letter %q (a decomposed accented letter; "+
				"the repository is English only, in ASCII letters)", r, prev)
		}
		prev = r
		i += size
	}
	return ""
}

func isASCIILetter(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}
