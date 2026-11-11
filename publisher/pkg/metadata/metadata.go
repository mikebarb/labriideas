// Package metadata is the single home for the two text-level operations
// every metadata consumer shares: the RFC 2047 codec and the merge.
//
// ─── THE R2 METADATA MODEL (established by cmd/r2-metadata-probe) ────
//
// Probe evidence, against bucket labriideas-dev, SDK aws-sdk-go-v2:
//
//   - WRITE (PUT or COPY): values are sent as RAW UTF-8 in x-amz-meta-*
//     headers and R2 normalises them before storage. A raw value
//     round-trips into storage unchanged — the caller must NOT
//     pre-encode (cases 1–3: raw Unicode was accepted and stored).
//   - READ (HEAD/GET): R2 re-encodes any value containing '?' or
//     non-ASCII characters as one or more RFC 2047 encoded-words. The
//     value is DIFFERENT on the wire from what is stored.
//   - STABILITY: the encoded retrieval form is a fixed point — writing
//     a retrieved value back changes nothing (cases 4 and 5). There is
//     no compounding, no drift, no layering.
//   - COST: CopyObject with REPLACE moved 1 byte of request body for a
//     1 MB object (case 10) — a server-side metadata rewrite.
//
// Therefore:
//
//   - decode EVERY value on read, before any comparison or merge;
//   - write RAW Unicode values, never encoded ones;
//   - merge in Unicode space (decoded), never in encoded space —
//     otherwise "Café" and its encoded form compare as different
//     values and every edit rewrites every accented field.
//
// ─── THE ADJACENCY RULE ──────────────────────────────────────────────
//
// R2's encoder splits a word ending in '?' into TWO ADJACENT
// encoded-words:
//
//	sent:     "Unbelief?"
//	returned: "=?utf-8?Q?Unbelief?= =?utf-8?Q?=3F?="
//
// RFC 2047 §6.2 says linear white space between adjacent encoded-words
// is NOT part of the decoded text. A naive regex-substitute decoder
// keeps that space and yields "Unbelief ?" — a one-space corruption.
// That naive behaviour is how the space-drift visible in some stored
// titles ("Unbelief ?") propagated historically: a naive decode was
// written back through earlier tooling. Decode here implements the
// adjacency rule, and the probe's case 11 confirms the round trip is
// byte-exact.
package metadata

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
)

// encodedWordRe matches one RFC 2047 encoded-word:
//
//	=?charset?Q|B?encoded-text?=
//
// The four submatches are: whole match, charset, encoding, text.
var encodedWordRe = regexp.MustCompile(`=\?([^?]+)\?([QqBb])\?([^?]*)\?=`)

// Decode decodes every RFC 2047 encoded-word in s, applying the
// §6.2 adjacency rule (whitespace between two adjacent encoded-words is
// removed).
//
// The function is idempotent on already-decoded text UNLESS that text
// genuinely contains an encoded-word-shaped literal — e.g. the legacy
// artefact "=3F" sitting as plain text. Such literals decode to "?" on
// every pass, which is correct: they ARE encoding artefacts by
// definition, and this decoder is also the repair mechanism for them.
func Decode(s string) string {
	matches := encodedWordRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s // fast path: the overwhelming majority of values
	}

	var out strings.Builder
	prevEnd := 0            // end of the last consumed region of s
	prevWasEncoded := false // whether the previous emitted token was an encoded-word

	for _, m := range matches {
		start, end := m[0], m[1]
		gap := s[prevEnd:start]

		// §6.2 adjacency: skip the gap ONLY when the previous token was
		// an encoded-word AND the gap is entirely whitespace. A gap
		// containing visible text is real content and must be kept.
		if !(prevWasEncoded && gap != "" && strings.TrimSpace(gap) == "") {
			out.WriteString(gap)
		}

		out.WriteString(decodeOne(s, m))
		prevEnd = end
		prevWasEncoded = true
	}
	out.WriteString(s[prevEnd:])
	return out.String()
}

// decodeOne decodes a single encoded-word, given FindAllStringSubmatchIndex
// results. Non-UTF-8 charsets pass through untouched: this library's data
// is UTF-8, and silently transcoding other charsets is a corruption risk
// rather than a service.
func decodeOne(s string, m []int) string {
	charset := s[m[2]:m[3]]
	encoding := strings.ToLower(s[m[4]:m[5]])
	text := s[m[6]:m[7]]

	if !strings.EqualFold(charset, "utf-8") {
		return s[m[0]:m[1]]
	}
	switch encoding {
	case "q":
		return decodeQ(text)
	case "b":
		if data, err := base64.StdEncoding.DecodeString(text); err == nil {
			return string(data)
		}
	}
	return s[m[0]:m[1]]
}

// decodeQ implements the RFC 2047 "Q" encoding: '_' stands for space,
// and "=XX" pairs are hex escapes.
func decodeQ(text string) string {
	text = strings.ReplaceAll(text, "_", " ")
	var buf strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '=' && i+2 < len(text) {
			hex := text[i+1 : i+3]
			if b, err := strconv.ParseUint(hex, 16, 8); err == nil {
				buf.WriteByte(byte(b))
				i += 2
				continue
			}
		}
		buf.WriteByte(text[i])
	}
	return buf.String()
}

// Merge applies changes to existing metadata and returns the final map
// to be written.
//
// CONTRACT (agreed design, both server and CLI paths):
//
//   - key absent from changes        → field preserved unchanged
//   - key present with non-empty val → field replaced
//   - key present with EMPTY value   → field DELETED (omitted from result)
//
// Both maps MUST already be in Unicode space: existing values come from
// Decode(head.Metadata), changes come from the caller's validated input.
// Merging in encoded space would compare "Café" against its encoded form
// and rewrite every accented field on every edit.
//
// The result never contains empty-string values — the "R2 is sparse"
// invariant: an empty stored value adds no meaning and spends the
// metadata size budget. Keys are used as provided; key validation
// (lowercase, non-empty, collision-free) belongs to schema.Validate and
// runs BEFORE merge, on the incoming changes.
func Merge(existing, changes map[string]string) map[string]string {
	merged := make(map[string]string, len(existing)+len(changes))
	for k, v := range existing {
		// Defensive: R2 never stores empty values by our own contract,
		// but a legacy empty value must not be resurrected by a merge.
		if v != "" {
			merged[k] = v
		}
	}
	for k, v := range changes {
		if v == "" {
			delete(merged, k) // empty = delete
		} else {
			merged[k] = v
		}
	}
	return merged
}
