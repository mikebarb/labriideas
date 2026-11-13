package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// CatalogSchema is the canonical field list for a catalog track entry.
// Order is document order; the checksum is computed over a sorted copy,
// so reordering here does NOT change the checksum — only adding,
// removing, or reclassifying a field does.
var CatalogSchema = []string{
	"filename",
	"hash",
	"audio-hash",
	"title",
	"artist",
	"speaker",
	"category",
	"genre",
	"keywords",
	"year",
	"topten",
}

// CatalogSchemaDerived lists fields computed by the system.
// Derived fields are NEVER authored:
//   - never written by the CSV importer,
//   - never accepted by PatchTrack (values or deletes),
//   - never written to R2 metadata by an update request.
//
// AppendTrack is the one legitimate supply point: creation is where the
// derived values are first observed and recorded.
var CatalogSchemaDerived = []string{
	"filename",   // R2 object key — the primary key
	"hash",       // object ETag, observed at crawl time
	"audio-hash", // MD5 of audio frames, computed at upload
}

// CatalogSchemaWritable lists fields that can be authored.
var CatalogSchemaWritable = []string{
	"title",
	"artist",
	"speaker",
	"category",
	"genre",
	"keywords",
	"year",
	"topten",
}

// IsDerived reports whether a field is computed by the system and must
// never appear in authoring input.
func IsDerived(field string) bool {
	for _, f := range CatalogSchemaDerived {
		if f == field {
			return true
		}
	}
	return false
}

// IsSchemaField reports whether a field belongs to the catalog schema.
func IsSchemaField(field string) bool {
	for _, f := range CatalogSchema {
		if f == field {
			return true
		}
	}
	return false
}

// IsWritable reports whether a field can be authored.
func IsWritable(field string) bool {
	for _, f := range CatalogSchemaWritable {
		if f == field {
			return true
		}
	}
	return false
}

// Checksum computes the schema checksum over the canonical field list
// AND its classification.
//
// Canonical form: each field on its own line, prefixed by its class
// ("w:" or "d:"), sorted lexicographically, joined with "\n", then
// SHA-256 as hex.
//
// Moving a field between writable and derived changes the checksum — not
// just adding or removing a field. Reordering CatalogSchema does not.
//
// Record the value after any schema change; consumers compare it against
// the server's /api/schema/catalog response.
func Checksum() string {
	lines := make([]string, 0, len(CatalogSchema))
	for _, f := range CatalogSchema {
		class := "d"
		if IsWritable(f) {
			class = "w"
		}
		lines = append(lines, class+":"+f)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
