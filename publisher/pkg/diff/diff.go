// Package diff decides, per record and per field, whether a CSV row
// requires a write to R2 — and exactly which fields that write touches.
//
// It is PURE: no R2, no HTTP, no filesystem. Everything it needs is in
// its arguments, so every rule below is testable in isolation.
//
// ─── THE AUTHORITY RULE ───
//
// A CSV column PRESENT in the header is authoritative for every row in
// that file, including blank cells:
//
//	present column, non-blank value  → SET     (write the value)
//	present column, blank value      → DELETE  (remove the field)
//	ABSENT column                    → KEEP    (leave R2 untouched)
//
// The absent/blank distinction is the whole point. A partial CSV
// carrying only title and speaker must not wipe artist, genre and the
// rest — an absent column is not an instruction to delete, it is an
// absence of instruction. A present column with a blank cell IS an
// instruction: the operator typed nothing, so the field should go.
package diff

import (
	"sort"
	"strings"
)

// Action — what the engine decided for one field.
type Action int

const (
	// Keep: the CSV did not speak about this field. No action.
	Keep Action = iota
	// Set: write the CSV value to R2 metadata.
	Set
	// Delete: remove the field from R2 metadata.
	Delete
)

func (a Action) String() string {
	switch a {
	case Set:
		return "set"
	case Delete:
		return "delete"
	}
	return "keep"
}

// Outcome — what should happen to a whole record.
type Outcome int

const (
	// OutcomeNew: in the CSV, no catalog entry — upload or append.
	OutcomeNew Outcome = iota
	// OutcomeUpdate: entry exists, at least one field differs.
	OutcomeUpdate
	// OutcomeSkip: entry exists, nothing differs — NO HTTP call at all.
	OutcomeSkip
)

func (o Outcome) String() string {
	switch o {
	case OutcomeNew:
		return "new"
	case OutcomeUpdate:
		return "update"
	}
	return "skip"
}

// RecordDiff is the per-field decision for one record.
//
// Actions holds an entry for every field the engine considered
// actionable (Set or Delete only — Keep fields are omitted). Values
// holds the new value for each Set. Deletes lists the fields to remove.
type RecordDiff struct {
	Actions map[string]Action
	Values  map[string]string
	Deletes []string
	Changed bool
}

// BatchResult is the verdict for one CSV row.
type BatchResult struct {
	Filename string
	Outcome  Outcome
	Diff     RecordDiff // meaningful only for OutcomeUpdate
}

// DiffRecord compares one row against one existing entry.
//
// presentColumns: the columns the CSV header declared. Absent column =
// no instruction (keep existing). This is the authority rule.
// csvValues: the row, keyed by field name.
// existing: the catalog's current values for this track. Pass nil for
// a filename with no catalog entry (not used by this function's
// callers in that case — DiffBatch handles New separately).
// writableFields: the fields the engine may author. Derived fields are
// excluded by the caller — the engine is deliberately indifferent to
// how they are classified.
//
// Values are trimmed before comparison, and this is not cosmetic. R2
// custom metadata travels as HTTP headers, and header transport strips
// leading/trailing whitespace, so "Title " can never actually be
// stored. Comparing untrimmed would report a difference on every run
// and rewrite the record forever — exactly the churn loop the Skip
// outcome exists to prevent.
func DiffRecord(presentColumns map[string]bool, csvValues map[string]string,
	existing map[string]string, writableFields []string) RecordDiff {

	d := RecordDiff{
		Actions: make(map[string]Action),
		Values:  make(map[string]string),
	}

	for _, field := range writableFields {
		if !presentColumns[field] {
			continue // column absent → no instruction
		}
		newVal := strings.TrimSpace(csvValues[field])
		oldVal := strings.TrimSpace(existing[field])

		switch {
		case newVal == "" && oldVal == "":
			// Blank against empty: nothing to delete, nothing to write.
			continue
		case newVal == "":
			// Present column, blank cell, existing value → delete.
			d.Actions[field] = Delete
			d.Deletes = append(d.Deletes, field)
			d.Changed = true
		case newVal == oldVal:
			continue
		default:
			d.Actions[field] = Set
			d.Values[field] = newVal
			d.Changed = true
		}
	}
	return d
}

// DiffBatch classifies every CSV row against the catalog index and
// reports catalog entries the CSV does not cover.
//
// rows: parsed CSV rows, each with a "filename" key.
// presentColumns: the authority set from the parser.
// index: catalog entries keyed by filename → map[field]value, values
// already normalised (JSON null → "").
// writableFields: the fields the engine may author.
//
// Returns the per-row results in CSV order, plus orphans sorted
// alphabetically for deterministic reporting. Orphans are REPORTED,
// never acted on: absence from a CSV is not evidence the object should
// be deleted, because a CSV may legitimately cover part of the library.
func DiffBatch(rows []map[string]string, presentColumns map[string]bool,
	index map[string]map[string]string, writableFields []string) ([]BatchResult, []string) {

	results := make([]BatchResult, 0, len(rows))
	covered := make(map[string]bool, len(rows))

	for _, row := range rows {
		filename := strings.TrimSpace(row["filename"])
		if filename == "" {
			// The parser rejects these; this is the last line of defence.
			continue
		}
		covered[filename] = true

		entry, exists := index[filename]
		if !exists {
			results = append(results, BatchResult{
				Filename: filename,
				Outcome:  OutcomeNew,
			})
			continue
		}

		d := DiffRecord(presentColumns, row, entry, writableFields)
		if !d.Changed {
			results = append(results, BatchResult{
				Filename: filename,
				Outcome:  OutcomeSkip,
			})
			continue
		}
		results = append(results, BatchResult{
			Filename: filename,
			Outcome:  OutcomeUpdate,
			Diff:     d,
		})
	}

	orphans := make([]string, 0)
	for filename := range index {
		if !covered[filename] {
			orphans = append(orphans, filename)
		}
	}
	sort.Strings(orphans)

	return results, orphans
}
