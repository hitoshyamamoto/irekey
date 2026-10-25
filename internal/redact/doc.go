// SPDX-License-Identifier: MIT

// Package redact keeps secrets out of logs and command output, and makes
// untrusted text safe to show on a terminal. It is how irekey upholds the
// data separation rule (DataSeparation) for what it prints and logs: no
// plaintext secret in logs or CLI output.
//
// # Redaction
//
// A [Redactor] holds the secrets of one process. [Redactor.Add] registers a
// secret of at least [MinSecretLen] bytes, together with the encoded forms
// it takes in output: its JSON string encoding, with and without the HTML
// escaping that encoding/json applies by default (so --json output and the
// JSON handler of log/slog are covered), the escaping of [strconv.Quote]
// (so %q and the text handler of log/slog are covered), and its [Sanitize]
// form. Text then passes through [Redactor.String], [Redactor.Bytes], a
// streaming [Writer] or a log/slog handler made by [NewHandler], and every
// occurrence of a registered form is replaced by [Marker]. Overlapping
// occurrences are merged: the whole stretch they cover becomes one Marker,
// so no part of any of them remains visible.
//
// A [Writer] gives the same result as [Redactor.Bytes] on the concatenation
// of everything written to it, whatever the chunk boundaries are: when the
// end of a chunk could be the start of a secret, it holds those bytes back
// until the next write shows whether the secret completes. At most the
// length of the longest registered form, minus one byte, is held back.
//
// The handler redacts each record value by value before next encodes it,
// against the secrets registered when the record is handled, including
// attributes and groups added to the logger earlier.
//
// Limits:
//
//   - A secret is recognized only from the moment it is registered. Text
//     printed before [Redactor.Add] cannot be recalled, and an occurrence
//     whose first bytes a Writer had already written when Add was called is
//     not recognized either. Call Add as soon as a secret is obtained,
//     before it can reach any output.
//   - Add matches the secret exactly as given and does not normalize it; a
//     trailing newline read from a file, for example, must be trimmed by
//     the caller.
//   - Secrets shorter than MinSecretLen bytes are rejected: Add returns
//     false. Redacting every occurrence of one to three bytes would destroy
//     unrelated output and protect next to nothing.
//   - Bytes held back are written, without waiting further, when the stream
//     ends: by [Writer.Flush] or [Writer.Close]. A secret split by a Flush is
//     therefore not recognized; flush only where the output is complete.
//   - Only the forms listed above are recognized. A secret transformed in
//     another way (base64, hexadecimal, URL encoding, %+q, or split across
//     separate log attributes) is not.
//   - The handler redacts a record's message, attribute keys, group names
//     and values. Its time, level and source location are passed on as
//     they are.
//   - Matching works on bytes, not words or tokens: text equal to a secret
//     is replaced wherever it occurs, also inside a longer word, a number
//     or an escape sequence. Leak prevention takes priority over well-formed
//     output: with a secret "1234", a Writer turns {"duration_ms":81234}
//     into {"duration_ms":8[REDACTED]}, which is no longer valid JSON.
//     Structured output is therefore redacted value by value before it is
//     encoded, as the handler does for log records, so that it stays valid;
//     a Writer on the encoded stream is defense in depth.
//
// # Terminal safety
//
// [Sanitize] makes text received from a device safe to print: it keeps
// newline, tab and graphic characters, and writes ASCII controls, invalid
// UTF-8 bytes and every non-graphic rune (C1 controls, format characters
// such as the bidirectional text controls, line and paragraph separators,
// private-use and unassigned code points) as visible escapes such as \x1b
// and \u{202e}. Escaping was chosen over stripping for three reasons: there
// is no escape-sequence parser to get wrong, nothing is lost for
// diagnostics, and a malformed sequence cannot slip through half removed.
//
// The package performs no I/O of its own. io.Writer and slog.Handler are
// interfaces: a Writer writes only to the io.Writer it is given, and a
// handler only passes records on to the handler it wraps.
package redact
