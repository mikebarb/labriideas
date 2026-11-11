// Package textnorm provides Unicode normalisation classification and
// normalisation helpers for every text boundary in the publisher system.
//
// # WHY THIS PACKAGE EXISTS
//
// The same visible text can be stored in two different byte sequences:
//
//	NFC (composed):   "é"  = U+00E9                   (2 bytes in UTF-8)
//	NFD (decomposed): "é"  = U+0065 U+0301             (3 bytes in UTF-8)
//
// Both are canonically equivalent per the Unicode Standard, and both are
// spec-conformant everywhere — but a byte-wise comparison treats them as
// DIFFERENT strings. That difference is invisible to a human and fatal to
// an exact-match lookup, which is exactly how R2 object keys are resolved.
//
// The practical sources of NFD text in this system:
//   - macOS HFS+ materialises filenames in NFD; files copied to Windows
//     arrive still decomposed (NTFS preserves bytes as given).
//   - APFS is normalisation-insensitive but normalisation-PRESERVING, so a
//     Mac can hold either form and look identical either way.
//   - ID3v2 does NOT standardise normalisation; a title tagged by different
//     tools can differ in form. mp3scan reads tags, writes the CSV, so tag
//     form becomes CSV form.
//   - A single CSV can mix forms row-by-row and field-by-field, because the
//     values come from independent sources.
//
// POLICY (decided in design): NFC everywhere. NFC is the Unicode
// Standard's preferred interchange form, is shorter for Latin text, and is
// what modern tooling expects. Normalise at the boundary — on read — so no
// component downstream ever has to defend against the other forms.
//
// IMPORTANT SCOPE RULE: normalisation applies to VALUES and to filename
// COMPARISON only. Object keys stored in R2 are written once (NFC, for new
// uploads) and are NEVER rewritten — normalising an existing key would
// invalidate presigned URLs and external references. On-disk files are
// never renamed. Normalisation here is a comparison concern, not a
// storage-mutation concern.
package textnorm

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Form classifies the normalisation state of a string.
//
// The classification is deliberately five-valued rather than binary:
//   - ASCII content is invariant under both forms, so reporting "nfc"
//     for it would be technically true but uninformative; audits want to
//     know that normalisation is IRRELEVANT for that value.
//   - Invalid UTF-8 cannot be normalised at all (norm treats invalid
//     sequences as pass-through rather than repairing them), so it must
//     be reported as its own case — such a value can never be made
//     reliably comparable across systems.
type Form string

const (
	FormASCII   Form = "ascii"        // pure ASCII — normalisation is a no-op
	FormNFC     Form = "nfc"          // already composed; includes strings invariant under both forms
	FormNFD     Form = "nfd"          // decomposed — convertible to NFC
	FormMixed   Form = "mixed"        // partially decomposed — convertible to NFC
	FormInvalid Form = "invalid-utf8" // not valid UTF-8 — cannot be normalised
)

// Classify returns the normalisation Form of s.
//
// Note that a string with no canonically decomposable characters (pure
// ASCII, or scripts like CJK whose characters have no canonical
// decomposition) is normal under BOTH NFC and NFD. Such strings are
// reported as FormNFC — they are already canonical — after the ASCII
// fast-path check.
func Classify(s string) Form {
	if !utf8.ValidString(s) {
		return FormInvalid
	}
	if isASCII(s) {
		return FormASCII
	}
	isNFC := norm.NFC.IsNormalString(s)
	isNFD := norm.NFD.IsNormalString(s)
	switch {
	case isNFC:
		// Covers both "NFC only" and "normal under both forms".
		// Either way the value is already in the preferred form.
		return FormNFC
	case isNFD:
		return FormNFD
	default:
		// Neither form as a whole: the string mixes precomposed and
		// decomposed sequences — typically the result of concatenating
		// values from different sources. Still convertible to NFC.
		return FormMixed
	}
}

// IsProblem reports whether the form needs attention in an audit report.
// ASCII and NFC content are fine as-is; NFD and mixed content will compare
// unequal to their NFC equivalents; invalid UTF-8 can never be normalised.
func (f Form) IsProblem() bool {
	return f == FormNFD || f == FormMixed || f == FormInvalid
}

// ToNFC returns the NFC form of s.
//
// The operation is idempotent: normalising an already-NFC string returns
// it byte-for-byte unchanged, so callers can normalise unconditionally
// without checking first. Invalid UTF-8 sequences pass through unchanged
// (norm does not repair them) — use Classify to detect that case first
// where it matters.
func ToNFC(s string) string {
	return norm.NFC.String(s)
}

// EqualNFC compares two strings for canonical equivalence by normalising
// both to NFC first. Use this wherever filenames (or any other text that
// may have crossed operating systems) are matched — NOT for general
// metadata comparison, which happens after boundary normalisation has
// already been applied.
func EqualNFC(a, b string) bool {
	return norm.NFC.String(a) == norm.NFC.String(b)
}

// isASCII reports whether s is entirely 7-bit ASCII. A byte-wise scan is
// sufficient: in valid UTF-8, any byte >= 0x80 begins a multi-byte
// sequence. (utf8.RuneSelf == 0x80.)
func isASCII(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return r >= utf8.RuneSelf }) == -1
}
