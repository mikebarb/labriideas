package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Track represents a single audio file in our catalog
//type Track struct {
//	ID       string `json:"id"`
//	Title    string `json:"title"`
//	Artist   string `json:"artist"`
//	FileName string `json:"fileName"`
//}

// Catalog is the wrapper for our versioned manifest
//type Catalog struct {
//	Version string  `json:"version"`
//	Count   int     `json:"count"`
//	Tracks  []Track `json:"tracks"`
//}

// CatalogSchema defines the exact fields we want to extract from any data source (like a CSV).
// If you add a new field to the system, add its JSON key name here.
var CatalogSchema = []string{
	"filename",
	"title",
	"artist",
	"hash",
	"speaker",
	"category",
	"genre",
	"keywords",
	"year",
	"topten",
	"audio-hash",
}

// ─────────────────────────────────────────────────────────────────────────
// FIELD CLASSIFICATION
//
// Not every schema field is a *metadata* field. Two of them are derived
// at catalogue-build time from R2 state itself, and accepting them from
// any caller would store values that can never be true:
//
//   filename — the R2 object key; the crawler takes it from the key.
//   hash     — the R2 ETag; the crawler takes it from the HEAD response.
//
// Storing either as custom metadata would create a second, stale copy of
// a value the crawler recomputes — state that invites confusion during
// diagnosis (a stored "hash" that disagrees with the real ETag is worse
// than no stored hash at all). CSV readers may still *encounter* these
// columns — a catalogue export round-trips them — so the exclusion is a
// parser-level rule, not a change to CatalogSchema itself, which must
// keep listing every field the catalog carries.
// ─────────────────────────────────────────────────────────────────────────

// DerivedFields are computed from R2 state at crawl time and are never
// accepted as writable metadata from any caller.
var DerivedFields = []string{"filename", "hash"}

// IsDerived reports whether fieldName is a derived (non-writable) field.
func IsDerived(fieldName string) bool {
	for _, d := range DerivedFields {
		if d == fieldName {
			return true
		}
	}
	return false
}

// WritableFields returns the schema fields that may be written to R2
// custom metadata: CatalogSchema minus the derived fields. The order
// follows CatalogSchema, which callers rely on for stable iteration.
func WritableFields() []string {
	out := make([]string, 0, len(CatalogSchema))
	for _, f := range CatalogSchema {
		if !IsDerived(f) {
			out = append(out, f)
		}
	}
	return out
}

// Checksum returns a hex SHA-256 over a CANONICAL form of the schema:
// every field, sorted, deduplicated, newline-joined.
//
// WHY SORTED: reordering the CatalogSchema literal must NOT change the
// checksum — only adding, removing, or renaming a field should. The CLI
// tools compare this checksum against the value served by the server to
// detect schema drift ("same or different; rectify if different"), and a
// false mismatch caused by column reordering would be pure noise.
func Checksum() string {
	sorted := make([]string, len(CatalogSchema))
	copy(sorted, CatalogSchema)
	sort.Strings(sorted)

	h := sha256.New()
	prev := ""
	for i, f := range sorted {
		if i > 0 && f == prev {
			continue // deduplicate
		}
		h.Write([]byte(f))
		h.Write([]byte("\n"))
		prev = f
	}
	return hex.EncodeToString(h.Sum(nil))
}
