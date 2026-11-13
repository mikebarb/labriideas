// Package csvio handles reading and writing catalog CSVs in the format
// defined by schema.CatalogSchema. Used by the catalog-exporter,
// csv-reformatter, and any future import/export tools.
//
// ─── IMPORT PARSER ───
//
// Parse is the input stage of the bulk metadata pipeline:
//
//	CSV bytes → csvio.Parse → diff.DiffBatch → (dry run | apply)
//
// It is deliberately STRICTER and more REPORTING than ReadCatalogCSV,
// which serves the export/reformat tools. ReadCatalogCSV is tolerant
// because it reads files the app itself wrote; Parse is suspicious
// because it reads files a human edited in a spreadsheet, and every
// silent repair it made could destroy metadata.
//
// THE COLUMN SET IS THE CONTRACT. PresentColumns is what the diff engine
// consults to distinguish "the file is silent about this field" from
// "the file says this field is empty". Those are different instructions
// (keep vs delete), and a CSV cannot express the difference any other
// way — so the parser reports the header faithfully and never invents a
// column.
//
// TRAILING OMISSION IS NORMAL, NOT SUSPICIOUS. Excel omits trailing
// empty cells when saving CSV, so a row whose last three fields are
// blank arrives with three fewer fields than the header. That is the
// same information a blank cell carries, so the parser pads such rows
// and records them as short. They are nonetheless reported loudly,
// because the SAME signature appears when a file has been truncated —
// and padding a truncated file would turn every omitted trailing field
// into a delete instruction.

package csvio

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mikebarb/labriideas-publisher/pkg/schema"
)

// ReadCatalogCSV reads a CSV file and returns a map of filename -> metadata.
//
// The first row is treated as the header. Column names are trimmed and
// lowercased for case-insensitive matching against schema.CatalogSchema.
// Extra columns not in the schema are silently ignored.
//
// If the file has no header (first row is data), the function will fail
// with an error because no schema column names are found in the header.
/*
func ReadCatalogCSV(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // tolerate variable column counts

	// Read the first row — this MUST be the header
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header (first row): %w", err)
	}

	// Normalize headers and build a map of column-name -> index
	headerMap := make(map[string]int)
	for i, h := range headers {
		normalized := strings.TrimSpace(strings.ToLower(h))
		headerMap[normalized] = i
	}

	// Ensure the mandatory 'filename' column exists
	if _, ok := headerMap["filename"]; !ok {
		return nil, fmt.Errorf("mandatory 'filename' column missing in CSV header. " +
			"The first row of the file must contain column names, including 'filename'.")
	}

	// Validate at least one schema column is found in the header
	// (otherwise the user might have a headerless CSV)
	schemaColFound := false
	for _, col := range schema.CatalogSchema {
		if _, ok := headerMap[col]; ok {
			schemaColFound = true
			break
		}
	}
	if !schemaColFound {
		return nil, fmt.Errorf("no schema columns found in CSV header. " +
			"The first row appears to be data, not column names. " +
			"Please add a header row (filename, title, artist, ...) to the file.")
	}

	// Read the remaining rows
	catalog := make(map[string]map[string]string)
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV data row: %w", err)
		}

		// Extract the filename
		filename := ""
		if idx, ok := headerMap["filename"]; ok && idx < len(row) {
			filename = strings.TrimSpace(row[idx])
		}
		if filename == "" {
			continue // silently skip rows without a filename
		}

		// Check for duplicate filename — warn and keep the last one
		if _, exists := catalog[filename]; exists {
			fmt.Printf("⚠️  Duplicate filename found: %s — keeping the last occurrence.\n", filename)
		}

		// Build metadata using the schema column order
		meta := make(map[string]string)
		for _, col := range schema.CatalogSchema {
			if idx, ok := headerMap[col]; ok && idx < len(row) {
				meta[col] = strings.TrimSpace(row[idx])
			}
		}
		catalog[filename] = meta
	}

	return catalog, nil
}
*/
// WriteCatalogCSV writes the catalog to a CSV file using the column order
// from schema.CatalogSchema. Rows are sorted by filename for a stable,
// diff-friendly output.
//
// The map key is always written as the "filename" column, regardless of
// whether the metadata map contains a "filename" field. This makes the
// function correct for both:
//   - Catalogs where filename is the map key (e.g., fetched from the API)
//   - Catalogs where filename is in the metadata (e.g., read from a CSV)
//
// Returns the number of data rows written (excluding the header).

func WriteCatalogCSV(path string, catalogMap map[string]map[string]string) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("could not create %s: %w", path, err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// Write header row
	if err := writer.Write(schema.CatalogSchema); err != nil {
		return 0, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Sort filenames for stable output
	filenames := make([]string, 0, len(catalogMap))
	for fn := range catalogMap {
		filenames = append(filenames, fn)
	}
	sort.Strings(filenames)

	// Write each data row
	rowCount := 0
	for _, fn := range filenames {
		meta := catalogMap[fn]
		row := make([]string, len(schema.CatalogSchema))
		for i, col := range schema.CatalogSchema {
			// Special case: the map key is always the filename,
			// even if the metadata doesn't contain a "filename" field.
			if col == "filename" {
				row[i] = fn
			} else {
				row[i] = meta[col] // empty string if missing
			}
		}
		if err := writer.Write(row); err != nil {
			return rowCount, fmt.Errorf("failed to write row for %s: %w", fn, err)
		}
		rowCount++
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return rowCount, err
	}

	return rowCount, nil
}

// ReadCatalogCSV reads a catalog CSV into a filename→metadata map.
//
// It is a thin wrapper over Parse so the package has ONE read
// implementation. Parse's tolerance and reporting are not discarded —
// they are simply not surfaced here, because callers of this function
// (the export/reformat tools) are reading files the app itself wrote and
// have nowhere to put a row-issue report.
func ReadCatalogCSV(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %w", path, err)
	}
	p, err := Parse(data)
	if err != nil {
		return nil, err
	}

	out := make(map[string]map[string]string, len(p.Rows))
	for _, row := range p.Rows {
		fn := row["filename"]
		meta := make(map[string]string, len(schema.CatalogSchema))
		for _, col := range schema.CatalogSchema {
			if v, ok := row[col]; ok {
				meta[col] = v
			}
		}
		out[fn] = meta
	}
	return out, nil
}

// SortedFilenames returns a sorted slice of filenames from a catalog map.
// Useful for iterating over a catalog in a predictable order.
func SortedFilenames(catalogMap map[string]map[string]string) []string {
	out := make([]string, 0, len(catalogMap))
	for fn := range catalogMap {
		out = append(out, fn)
	}
	sort.Strings(out)
	return out
}

const utf8BOM = "\ufeff"

// RowIssue kinds.
const (
	IssueShort         = "short"          // fewer cells than columns; padded
	IssueLong          = "long"           // more cells than columns; row skipped
	IssueEmptyFilename = "empty-filename" // no filename value; row skipped
	IssueDuplicateFile = "duplicate-file" // filename appears twice; last kept
)

// RowIssue is one repair or rejection the parser had to make. Reported,
// never silently applied — the operator decides whether to proceed.
type RowIssue struct {
	Line   int    // 1-based line number, as an editor would show it
	Kind   string // one of the Issue* constants
	Detail string // human-readable explanation
}

func (i RowIssue) String() string {
	return fmt.Sprintf("line %d: %s — %s", i.Line, i.Kind, i.Detail)
}

// ParsedCSV is the parser's output.
type ParsedCSV struct {
	// Header is the normalised column list, in file order (trimmed and
	// lowercased, so a hand-edited "Title" column still works).
	Header []string
	// PresentColumns is the authority set handed to diff.DiffBatch.
	// It records every column declared in the header, including derived
	// and unknown ones — presence is a fact about the file, and what is
	// actionable is decided by the writable-field list.
	PresentColumns map[string]bool
	// Rows are the accepted records, keyed by column name, values trimmed.
	Rows []map[string]string
	// RowNumbers[i] is the spreadsheet row number of Rows[i]: the 1-based
	// source line, so the header is row 1 and the first data row is
	// normally row 2. This is what an operator needs to find the row they
	// must fix, and it stays correct even when rows were skipped — it
	// comes from the file, not from the position within Rows.
	//
	// For a CSV with no embedded newlines this equals the spreadsheet row
	// exactly. A quoted field containing newlines makes the physical line
	// drift from the record ordinal; the physical line is reported because
	// that is what an editor will jump to.
	RowNumbers []int
	// Issues records every short row, over-long row, empty filename and
	// duplicate filename.
	Issues []RowIssue
	// IgnoredColumns are derived fields present in the header other than
	// filename (hash, audio-hash). Ignored by design: they are computed,
	// and importing them would corrupt duplicate detection.
	IgnoredColumns []string
	// UnknownColumns are header names that are not in the schema at all
	// (e.g. mp3scan's file_path). Ignored, and reported so the operator
	// knows they had no effect.
	UnknownColumns []string
	// Delimiter is the separator the parser detected.
	Delimiter rune
	// SourceRows counts data rows read, including skipped ones.
	SourceRows int
}

// Parse reads a CSV and prepares it for diffing.
//
// Errors are for conditions that make the whole file unprocessable:
// empty file, invalid UTF-8, malformed CSV, duplicate or empty header
// column, missing filename column. Row-level problems are Issues, not
// errors, because the operator decides whether to proceed.
func Parse(data []byte) (*ParsedCSV, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("CSV file is empty")
	}

	// 1. Strip the UTF-8 BOM. Excel writes one with "CSV UTF-8", and an
	//    unstripped BOM turns the first header cell into "\ufefffilename",
	//    which fails every filename lookup in the file.
	data = bytes.TrimPrefix(data, []byte(utf8BOM))

	// 2. Require UTF-8. Excel's plain "CSV (Comma delimited)" writes the
	//    system codepage (Windows-1252 in Western locales), mangling every
	//    accented character — the same damage class the metadata codec
	//    exists to undo. Refusing the file is better than writing mojibake
	//    into R2, and the fix is one save-as away.
	if !utf8.Valid(data) {
		return nil, errors.New(
			"CSV is not valid UTF-8 — in Excel use File → Save As → " +
				"'CSV UTF-8 (Comma delimited)'")
	}

	// 3. Detect the delimiter. European Excel writes semicolon-separated
	//    files, which matters here because the library has French content.
	delim := detectDelimiter(data)

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1 // length mismatches are reported, not raised
	r.TrimLeadingSpace = true

	rawHeader, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV header unreadable: %w", err)
	}

	// 4. Normalise and validate the header. Lowercasing means a
	//    hand-edited "Title" column works; schema field names are all
	//    lowercase, so this can only help. Duplicates are a hard error
	//    because "last wins" and "first wins" are both guesses.
	header := make([]string, len(rawHeader))
	seen := make(map[string]int, len(rawHeader))
	for i, cell := range rawHeader {
		name := strings.ToLower(strings.TrimSpace(cell))
		if name == "" {
			return nil, fmt.Errorf("CSV header: empty column name at position %d", i+1)
		}
		if prev, dup := seen[name]; dup {
			return nil, fmt.Errorf("CSV header: duplicate column %q (positions %d and %d)",
				name, prev+1, i+1)
		}
		seen[name] = i
		header[i] = name
	}

	// 5. Classify columns and require the primary key. filename is
	//    derived but is the key itself, so it is required rather than
	//    ignored; the other derived fields are computed and reported as
	//    ignored so the operator knows their columns had no effect.
	present := make(map[string]bool, len(header))
	var ignored, unknown []string
	hasFilename := false
	for _, name := range header {
		present[name] = true
		switch {
		case name == "filename":
			hasFilename = true
		case schema.IsDerived(name):
			ignored = append(ignored, name)
		case schema.IsWritable(name):
			// authored field — processed normally
		default:
			unknown = append(unknown, name)
		}
	}
	if !hasFilename {
		return nil, errors.New("CSV header has no 'filename' column")
	}

	// 6. Read rows.
	rows := make([]map[string]string, 0, 512)
	rowNumbers := make([]int, 0, 512)
	var issues []RowIssue
	sourceRows := 0
	rowIndex := make(map[string]int) // filename → index in rows

	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, fmt.Errorf("CSV line %d, column %d: %v", pe.Line, pe.Column, pe.Err)
			}
			return nil, fmt.Errorf("CSV read failed: %w", err)
		}
		sourceRows++

		line := sourceRows + 1 // header is line 1; best effort
		// FieldPos reports the actual source line of the most recent
		// record's field 0 — more accurate than counting records, since
		// multi-line quoted values span several lines. It PANICS if the
		// index is out of range, so guard on the record length.
		if len(record) > 0 {
			if pos, _ := r.FieldPos(0); pos > 0 {
				line = pos
			}
		}

		switch {
		case len(record) < len(header):
			// Trailing omission (normal for Excel) or truncation. Pad so
			// the row means what a blank cell means, and report it so
			// the operator can tell the two apart. The missing columns
			// are identified by POSITION, which is what Excel omitted.
			missing := make([]string, 0, len(header)-len(record))
			for i := len(record); i < len(header); i++ {
				missing = append(missing, header[i])
			}
			issues = append(issues, RowIssue{
				Line: line, Kind: IssueShort,
				Detail: fmt.Sprintf("%d of %d columns present; treated as blank: %s",
					len(record), len(header), strings.Join(missing, ", ")),
			})
			record = append(record, make([]string, len(header)-len(record))...)

		case len(record) > len(header):
			// Extra cells mean malformed data (or an unquoted comma).
			// Skipping is safer than guessing which cell belongs where.
			issues = append(issues, RowIssue{
				Line: line, Kind: IssueLong,
				Detail: fmt.Sprintf("%d values for %d columns; row skipped",
					len(record), len(header)),
			})
			continue
		}

		row := make(map[string]string, len(header))
		for i, name := range header {
			row[name] = strings.TrimSpace(record[i])
		}
		filename := row["filename"]
		if filename == "" {
			issues = append(issues, RowIssue{
				Line: line, Kind: IssueEmptyFilename,
				Detail: "no filename value; row skipped",
			})
			continue
		}

		// Duplicate filename: keep the LAST occurrence (matching the
		// historical behaviour) but say so. Silently keeping either one
		// makes a two-row edit to the same track look like one edit.
		if idx, dup := rowIndex[filename]; dup {
			issues = append(issues, RowIssue{
				Line: line, Kind: IssueDuplicateFile,
				Detail: fmt.Sprintf("%s appears more than once; this later row replaces the earlier one",
					filename),
			})
			rows[idx] = row
			rowNumbers[idx] = line
			continue
		}
		rowIndex[filename] = len(rows)
		rows = append(rows, row)
		rowNumbers = append(rowNumbers, line)
	}

	return &ParsedCSV{
		Header:         header,
		PresentColumns: present,
		Rows:           rows,
		RowNumbers:     rowNumbers,
		Issues:         issues,
		IgnoredColumns: ignored,
		UnknownColumns: unknown,
		Delimiter:      delim,
		SourceRows:     sourceRows,
	}, nil
}

// RowNumber returns the spreadsheet row for a filename, or 0 when the
// name is not in the parsed set.
func (p *ParsedCSV) RowNumber(filename string) int {
	for i, row := range p.Rows {
		if row["filename"] == filename {
			if i < len(p.RowNumbers) {
				return p.RowNumbers[i]
			}
			return 0
		}
	}
	return 0
}

// detectDelimiter picks between comma and semicolon from the header line.
// European Excel saves semicolon-separated CSV; guessing wrong on a
// comma-decimal locale is the difference between a working import and a
// single-column file.
func detectDelimiter(data []byte) rune {
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	if bytes.Count(line, []byte(";")) > bytes.Count(line, []byte(",")) {
		return ';'
	}
	return ','
}

// ShortRowCount returns how many rows were padded for missing cells.
func (p *ParsedCSV) ShortRowCount() int {
	n := 0
	for _, i := range p.Issues {
		if i.Kind == IssueShort {
			n++
		}
	}
	return n
}

// SkippedRowCount returns how many rows were rejected outright.
func (p *ParsedCSV) SkippedRowCount() int {
	n := 0
	for _, i := range p.Issues {
		if i.Kind == IssueLong || i.Kind == IssueEmptyFilename {
			n++
		}
	}
	return n
}

// DuplicateCount returns how many rows were replaced by a later row with
// the same filename.
func (p *ParsedCSV) DuplicateCount() int {
	n := 0
	for _, i := range p.Issues {
		if i.Kind == IssueDuplicateFile {
			n++
		}
	}
	return n
}

// SuspectedTruncation reports the case the padding rule cannot
// distinguish from normal trailing omission: every data row short. A
// file whose rows are ALL short is far more likely to be truncated than
// to have every row's tail deliberately emptied.
func (p *ParsedCSV) SuspectedTruncation() bool {
	return p.SourceRows > 0 && p.ShortRowCount() == p.SourceRows
}

// IssueSummary renders the operator-facing block for the end of a dry
// run: counts first, then up to maxLines individual issues.
func (p *ParsedCSV) IssueSummary(maxLines int) string {
	if len(p.Issues) == 0 && len(p.IgnoredColumns) == 0 && len(p.UnknownColumns) == 0 {
		return ""
	}
	var b strings.Builder
	if len(p.Issues) > 0 {
		fmt.Fprintf(&b, "CSV issues: %d short row(s), %d skipped row(s), %d duplicate filename(s), %d of %d row(s) read\n",
			p.ShortRowCount(), p.SkippedRowCount(), p.DuplicateCount(), len(p.Rows), p.SourceRows)
	}
	if p.SuspectedTruncation() {
		b.WriteString("  WARNING: every row is short — this file looks TRUNCATED.\n")
		b.WriteString("           Short rows are padded with blanks, and a blank in a present\n")
		b.WriteString("           column is a DELETE. Abort unless you are certain.\n")
	}
	if len(p.IgnoredColumns) > 0 {
		fmt.Fprintf(&b, "  ignored (derived, never imported): %s\n",
			strings.Join(p.IgnoredColumns, ", "))
	}
	if len(p.UnknownColumns) > 0 {
		fmt.Fprintf(&b, "  ignored (not in schema): %s\n",
			strings.Join(p.UnknownColumns, ", "))
	}
	for i, issue := range p.Issues {
		if i >= maxLines {
			fmt.Fprintf(&b, "  … and %d more (rerun with -list-issues to see all)\n",
				len(p.Issues)-maxLines)
			break
		}
		fmt.Fprintf(&b, "  %s\n", issue)
	}
	return b.String()
}
