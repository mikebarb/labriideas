// Package audit performs read-only conformance analysis over bucket keys
// and object metadata, producing the evidence base for the encoding
// refactor: how many objects are affected, in which fields, in which ways.
//
// The package is PURE: every function takes data that has already been
// fetched and returns report structures. No R2 calls, no I/O — which makes
// the analysis fully unit-testable and lets the same code later back a
// server endpoint (for administrators without bucket credentials) as well
// as this CLI tool.
//
// IMPORTANT CAVEAT the report must carry to its reader: the audit detects
// SYMPTOMS, not causes. An encoded-word inside a stored metadata value
// could have been introduced by the historical CopyObject path (RFC 2047
// encoding applied during server-side copy) OR by Excel's "CSV UTF-8
// (Comma Delimited)" export at import time — both leave the identical
// artefact. The counts are therefore a reliable measure of DAMAGE but not
// a reliable measure of how often each historical path misbehaved.
package audit

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mikebarb/labriideas-publisher/pkg/textnorm"
)

// encodedWordRe matches an RFC 2047 encoded-word anywhere in a value:
//
//	=?charset?Q|B?encoded-text?=
//
// This is the same shape as the detection regex in pkg/storage's crawler
// decoder. It is deliberately duplicated here rather than imported,
// because the codec (and this regex with it) moves to pkg/metadata in the
// refactor — when that happens this copy becomes the canonical one and
// the crawler adopts it. Keeping the patterns identical until then means
// the audit's detection agrees exactly with the decoder's behaviour.
var encodedWordRe = regexp.MustCompile(`=\?([^?]+)\?([QqBb])\?([^?]*)\?=`)

// KeyFinding is the analysis of a single object key.
type KeyFinding struct {
	Key       string        `json:"key"`
	Form      textnorm.Form `json:"form"`      // normalisation state of the filename
	SizeBytes int           `json:"sizeBytes"` // UTF-8 byte length (R2 limit: 1024)
	HasSlash  bool          `json:"hasSlash"`  // legal in R2 but complicates URL handling
	Problems  []string      `json:"problems,omitempty"`
}

// ValueFinding is the analysis of one metadata value on one object.
type ValueFinding struct {
	Field          string        `json:"field"`
	Raw            string        `json:"raw"`         // exactly what HEAD returned
	Decoded        string        `json:"decoded"`     // after RFC 2047 decode + NFC normalise
	Form           textnorm.Form `json:"form"`        // normalisation state of the RAW value
	DecodedForm    textnorm.Form `json:"decodedForm"` // normalisation state after decode
	HasEncodedWord bool          `json:"hasEncodedWord"`
	DoubleEncoded  bool          `json:"doubleEncoded,omitempty"`
	Issues         []string      `json:"issues,omitempty"`
}

// ObjectReport is the full analysis of one object.
type ObjectReport struct {
	Key           string         `json:"key"`
	ContentType   string         `json:"contentType,omitempty"` // empty means HEAD returned none
	MetaKeyCount  int            `json:"metaKeyCount"`
	MetaSizeBytes int            `json:"metaSizeBytes"` // sum of encoded key=value byte sizes
	ValueFindings []ValueFinding `json:"valueFindings,omitempty"`
	Issues        []string       `json:"issues,omitempty"`
}

// Summary is the bucket-level rollup — the "how many issues" answer.
type Summary struct {
	GeneratedAt       time.Time      `json:"generatedAt"`
	ObjectsScanned    int            `json:"objectsScanned"`
	HeadErrors        int            `json:"headErrors"`
	ObjectsWithIssues int            `json:"objectsWithIssues"`
	KeyForms          map[string]int `json:"keyForms"`          // form -> count (keys)
	ValueForms        map[string]int `json:"valueForms"`        // form -> count (raw values)
	EncodedWordValues int            `json:"encodedWordValues"` // values containing =?utf-8?Q?...?= artefacts
	DoubleEncodedVals int            `json:"doubleEncodedValues"`
	InvalidUTF8Values int            `json:"invalidUtf8Values"`
	NonASCIIKeys      int            `json:"nonAsciiKeys"`
	OversizedKeys     int            `json:"oversizedKeys"`      // keys over the 1024-byte limit
	OverMetaBudget    int            `json:"overMetadataBudget"` // objects whose encoded metadata exceeds the budget
	MetaBudgetBytes   int            `json:"metaBudgetBytes"`
	CauseCaveat       string         `json:"causeCaveat"`
}

// maxKeyBytes is R2's object-key length limit (UTF-8 bytes, not characters).
const maxKeyBytes = 1024

// metaBudgetBytes is the working budget for a complete x-amz-meta-* set.
//
// S3-compatible services cap total user metadata around 2 KB. The exact
// figure must be confirmed for R2 in the metadata probe (case: metadata
// near the limit with non-ASCII values) — the limit applies to the
// ENCODED wire form, and RFC 2047 encoding inflates non-ASCII values
// (base64 +~33%, Q-encoding up to ~3x for heavy accented text). This
// constant is the audit's measuring stick until the probe pins the real
// limit; it is deliberately conservative.
const metaBudgetBytes = 2048

// AnalyseKey inspects a single object key. Pure function.
func AnalyseKey(key string) KeyFinding {
	f := KeyFinding{
		Key:       key,
		Form:      textnorm.Classify(key),
		SizeBytes: len(key), // len() on a Go string IS the UTF-8 byte length
		HasSlash:  strings.Contains(key, "/"),
	}

	if f.Form.IsProblem() {
		f.Problems = append(f.Problems, fmt.Sprintf("key normalisation: %s", f.Form))
	}
	if f.SizeBytes > maxKeyBytes {
		f.Problems = append(f.Problems, fmt.Sprintf("key length %d exceeds %d-byte limit", f.SizeBytes, maxKeyBytes))
	}
	return f
}

// AnalyseMetadata inspects the metadata map returned by a HEAD. It never
// sees the object body. Pure function.
//
// Per-value analysis pipeline:
//  1. RFC 2047 decode the raw HEAD value (same decoder the crawler uses).
//  2. Normalise the DECODED value to NFC — the audit reports what a
//     repair would produce, so the "Decoded" column is the target state.
//  3. Detect double encoding: if the decoded value STILL contains an
//     encoded-word, an extra encoding layer was applied at some point
//     (the historical "? -> =3F" compounding). Nested patterns survive
//     one decode pass only when there were two or more layers.
func AnalyseMetadata(key string, meta map[string]string) ObjectReport {
	rep := ObjectReport{Key: key, MetaKeyCount: len(meta)}

	for k, raw := range meta {
		rep.MetaSizeBytes += len(k) + len("x-amz-meta-") + len(raw)

		vf := ValueFinding{
			Field: k,
			Raw:   raw,
			Form:  textnorm.Classify(raw),
		}
		vf.HasEncodedWord = encodedWordRe.MatchString(raw)

		decoded := raw
		if vf.HasEncodedWord {
			decoded = Decode(raw) // RFC 2047 decode — the audit's local copy; the codec moves here in the refactor
			if encodedWordRe.MatchString(decoded) {
				vf.DoubleEncoded = true
				vf.Issues = append(vf.Issues, "double-encoded: encoded-word remains after one decode pass")
			}
		}
		vf.Decoded = textnorm.ToNFC(decoded)
		vf.DecodedForm = textnorm.Classify(vf.Decoded)

		if vf.Form.IsProblem() {
			vf.Issues = append(vf.Issues, fmt.Sprintf("raw value form: %s", vf.Form))
		}
		if vf.HasEncodedWord {
			vf.Issues = append(vf.Issues, "contains RFC 2047 encoded-word artefact")
		}

		if len(vf.Issues) > 0 {
			rep.ValueFindings = append(rep.ValueFindings, vf)
		}
	}

	if rep.MetaSizeBytes > metaBudgetBytes {
		rep.Issues = append(rep.Issues,
			fmt.Sprintf("encoded metadata size %d exceeds working budget %d", rep.MetaSizeBytes, metaBudgetBytes))
	}

	return rep
}

// Summarize rolls up per-object reports into the bucket-level Summary.
func Summarize(reports []ObjectReport, headErrors int) Summary {
	s := Summary{
		GeneratedAt:     time.Now().UTC(),
		ObjectsScanned:  len(reports),
		HeadErrors:      headErrors,
		KeyForms:        map[string]int{},
		ValueForms:      map[string]int{},
		MetaBudgetBytes: metaBudgetBytes,
		// Stated in the report itself so the number is never
		// over-interpreted: damage counts, not cause counts.
		CauseCaveat: "Encoded-word artefacts may originate from the historical CopyObject path OR from Excel 'CSV UTF-8' exports at import time — both leave identical artefacts. Counts measure damage, not cause frequency.",
	}

	for _, rep := range reports {
		kf := AnalyseKey(rep.Key)
		s.KeyForms[string(kf.Form)]++
		if kf.Form != textnorm.FormASCII {
			s.NonASCIIKeys++
		}
		if kf.SizeBytes > maxKeyBytes {
			s.OversizedKeys++
		}
		if rep.MetaSizeBytes > metaBudgetBytes {
			s.OverMetaBudget++
		}

		issueCount := len(rep.Issues)
		for _, vf := range rep.ValueFindings {
			s.ValueForms[string(vf.Form)]++
			if vf.HasEncodedWord {
				s.EncodedWordValues++
			}
			if vf.DoubleEncoded {
				s.DoubleEncodedVals++
			}
			if vf.Form == textnorm.FormInvalid {
				s.InvalidUTF8Values++
			}
			issueCount += len(vf.Issues)
		}
		if issueCount > 0 {
			s.ObjectsWithIssues++ // fixed below
		}
	}
	return s
}

// Decode decodes RFC 2047 encoded-words within a string, implementing the
// adjacency rule from RFC 2047 §6.2: linear white space between two
// adjacent encoded-words is IGNORED (removed).
//
// WHY THE ADJACENCY RULE MATTERS: R2's retrieval-side encoder splits a
// word ending in '?' into TWO adjacent encoded-words:
//
//	sent:     "Unbelief?"
//	returned: "=?utf-8?Q?Unbelief?= =?utf-8?Q?=3F?="
//
// A naive regex-substitute decoder keeps the space between them and
// yields "Unbelief ?" — corrupting the value by one space. With the
// adjacency rule, decode recovers "Unbelief?" exactly, and the probe's
// round trip is lossless. The naive behaviour is also how the space-drift
// found in some stored values (titles reading "Unbelief ?") propagated
// historically: a naive decode was written back.
func Decode(s string) string {
	matches := encodedWordRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s // fast path: nothing to decode
	}

	var out strings.Builder
	prevEnd := 0            // end of the last consumed region in s
	prevWasEncoded := false // whether the previous emitted token was an encoded-word

	for _, m := range matches {
		start, end := m[0], m[1]
		gap := s[prevEnd:start]

		// RFC 2047 §6.2: whitespace between two ADJACENT encoded-words
		// is not part of the text. Only skip the gap when the previous
		// token was an encoded-word AND the gap is pure whitespace.
		if !(prevWasEncoded && strings.TrimSpace(gap) == "" && gap != "") {
			out.WriteString(gap)
		}

		out.WriteString(decodeOne(s, m))
		prevEnd = end
		prevWasEncoded = true
	}
	out.WriteString(s[prevEnd:])
	return out.String()
}

// decodeOne decodes a single encoded-word given its submatch indices.
// Non-UTF-8 charsets pass through untouched — the library's data is
// UTF-8, and silently transcoding other charsets is a corruption risk.
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

func decodeQ(text string) string {
	text = strings.ReplaceAll(text, "_", " ")
	var buf bytes.Buffer
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
