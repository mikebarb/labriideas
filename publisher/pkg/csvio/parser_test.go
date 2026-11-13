package csvio

import (
	"strings"
	"testing"
)

func TestParseBasics(t *testing.T) {
	data := []byte("filename,title,artist\na.mp3,Title A,Artist A\nb.mp3,Title B,Artist B\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Delimiter != ',' {
		t.Errorf("delimiter = %q, want ','", p.Delimiter)
	}
	if len(p.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(p.Rows))
	}
	if p.Rows[0]["title"] != "Title A" {
		t.Errorf("rows[0][title] = %q, want \"Title A\"", p.Rows[0]["title"])
	}
	if !p.PresentColumns["title"] {
		t.Error("PresentColumns should include title")
	}
	if p.PresentColumns["genre"] {
		t.Error("PresentColumns should not include an absent column")
	}
	if len(p.Issues) != 0 {
		t.Errorf("unexpected issues: %v", p.Issues)
	}
	if p.SourceRows != 2 {
		t.Errorf("SourceRows = %d, want 2", p.SourceRows)
	}
}

// An unstripped BOM turns the first header cell into "\ufefffilename", so
// every filename lookup in the file fails. This is the single most
// silent failure mode in the Excel round-trip.
func TestParseStripsBOM(t *testing.T) {
	data := []byte("\ufefffilename,title\na.mp3,T\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Header[0] != "filename" {
		t.Fatalf("header[0] = %q, want \"filename\" (BOM not stripped)", p.Header[0])
	}
	if _, ok := p.Rows[0]["filename"]; !ok {
		t.Fatal("filename lookup failed — BOM still attached to the header cell")
	}
}

func TestParseDuplicateHeaderIsFatal(t *testing.T) {
	_, err := Parse([]byte("filename,title,title\na.mp3,X,Y\n"))
	if err == nil {
		t.Fatal("expected error for duplicate header column")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should say duplicate: %v", err)
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error should name the offending column: %v", err)
	}
}

func TestParseEmptyHeaderCellIsFatal(t *testing.T) {
	_, err := Parse([]byte("filename,,title\na.mp3,x,T\n"))
	if err == nil || !strings.Contains(err.Error(), "empty column name") {
		t.Fatalf("expected empty-column-name error, got %v", err)
	}
}

func TestParseMissingFilenameColumn(t *testing.T) {
	_, err := Parse([]byte("title,artist\nT,A\n"))
	if err == nil || !strings.Contains(err.Error(), "filename") {
		t.Fatalf("expected missing-filename error, got %v", err)
	}
}

func TestParseEmptyFile(t *testing.T) {
	if _, err := Parse([]byte("   \n\n")); err == nil {
		t.Fatal("expected error for empty file")
	}
}

// Excel omits trailing empty cells, so a row can arrive with fewer fields
// than the header. Padding makes it mean what a blank cell means — but it
// must be reported, because truncation produces the same signature.
func TestParseShortRowPaddedAndReported(t *testing.T) {
	// 4 columns; the row supplies 3, so ONLY genre is omitted.
	data := []byte("filename,title,artist,genre\na.mp3,T,A\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 (short rows are padded, not dropped)", len(p.Rows))
	}
	// Supplied values must survive untouched...
	if p.Rows[0]["artist"] != "A" {
		t.Errorf("artist = %q, want \"A\" (supplied, must not be blanked)", p.Rows[0]["artist"])
	}
	// ...and only the omitted trailing column is padded.
	if p.Rows[0]["genre"] != "" {
		t.Errorf("genre = %q, want \"\" (omitted, padded)", p.Rows[0]["genre"])
	}
	if p.ShortRowCount() != 1 {
		t.Errorf("ShortRowCount = %d, want 1", p.ShortRowCount())
	}
	if len(p.Issues) != 1 || p.Issues[0].Kind != IssueShort {
		t.Fatalf("issues = %v, want one %q issue", p.Issues, IssueShort)
	}
	if !strings.Contains(p.Issues[0].Detail, "genre") {
		t.Errorf("issue should name the padded column: %q", p.Issues[0].Detail)
	}
	if strings.Contains(p.Issues[0].Detail, "artist") {
		t.Errorf("artist was supplied, so it must not be listed as padded: %q", p.Issues[0].Detail)
	}
}

func TestParseLongRowSkipped(t *testing.T) {
	data := []byte("filename,title\na.mp3,T,EXTRA\nb.mp3,T2\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 1 || p.Rows[0]["filename"] != "b.mp3" {
		t.Fatalf("long row should be skipped, got %v", p.Rows)
	}
	if p.SkippedRowCount() != 1 {
		t.Errorf("SkippedRowCount = %d, want 1", p.SkippedRowCount())
	}
	if p.Issues[0].Kind != IssueLong {
		t.Errorf("issue kind = %q, want %q", p.Issues[0].Kind, IssueLong)
	}
}

func TestParseEmptyFilenameSkipped(t *testing.T) {
	data := []byte("filename,title\n,T\nb.mp3,T2\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 1 || p.Rows[0]["filename"] != "b.mp3" {
		t.Fatalf("row without a filename should be skipped, got %v", p.Rows)
	}
	if p.Issues[0].Kind != IssueEmptyFilename {
		t.Errorf("issue kind = %q, want %q", p.Issues[0].Kind, IssueEmptyFilename)
	}
	if p.ShortRowCount() != 0 {
		t.Errorf("ShortRowCount = %d, want 0 (this is not a short row)", p.ShortRowCount())
	}
}

// Catalog exports always carry hash and audio-hash. They are derived, so
// the parser records their presence and reports them as ignored — never
// authored. This is the runtime proof of the derived-field guard.
func TestParseDerivedColumnsIgnored(t *testing.T) {
	data := []byte("filename,hash,audio-hash,title\na.mp3,dead,beef,T\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.IgnoredColumns) != 2 {
		t.Fatalf("IgnoredColumns = %v, want [hash audio-hash]", p.IgnoredColumns)
	}
	got := strings.Join(p.IgnoredColumns, ",")
	if got != "hash,audio-hash" {
		t.Errorf("IgnoredColumns = %q, want \"hash,audio-hash\"", got)
	}
	// Presence is still recorded — the writable-field list decides what
	// is actionable, not the parser.
	if !p.PresentColumns["hash"] || !p.PresentColumns["audio-hash"] {
		t.Error("PresentColumns should record derived columns' presence")
	}
	// filename is derived too, but it is the primary key, so it is not
	// reported as ignored.
	for _, c := range p.IgnoredColumns {
		if c == "filename" {
			t.Error("filename must not be listed as an ignored derived column")
		}
	}
	if s := p.IssueSummary(10); !strings.Contains(s, "audio-hash") {
		t.Errorf("summary should report the ignored columns: %q", s)
	}
}

func TestParseUnknownColumnsReported(t *testing.T) {
	data := []byte("filename,title,file_path\na.mp3,T,C:\\music\\a.mp3\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.UnknownColumns) != 1 || p.UnknownColumns[0] != "file_path" {
		t.Errorf("UnknownColumns = %v, want [file_path]", p.UnknownColumns)
	}
	if s := p.IssueSummary(10); !strings.Contains(s, "file_path") {
		t.Errorf("summary should report unknown columns: %q", s)
	}
}

// European Excel saves semicolon-separated CSV, and the library has
// French content. Guessing wrong yields a single-column file.
func TestParseSemicolonDelimiter(t *testing.T) {
	data := []byte("filename;title;artist\na.mp3;T;A\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Delimiter != ';' {
		t.Errorf("delimiter = %q, want ';'", p.Delimiter)
	}
	if p.Rows[0]["title"] != "T" || p.Rows[0]["artist"] != "A" {
		t.Errorf("row parsed wrong: %v", p.Rows[0])
	}
}

// Excel's plain "CSV (Comma delimited)" writes the system codepage.
// 0xE9 is é in Windows-1252 and invalid on its own in UTF-8. Refusing is
// better than writing mojibake into R2 metadata.
func TestParseRejectsInvalidUTF8(t *testing.T) {
	data := []byte("filename,title\na.mp3,Caf\xe9\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for non-UTF-8 input")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error should mention UTF-8: %v", err)
	}
}

func TestParseTrimsValues(t *testing.T) {
	data := []byte("filename,title\na.mp3,  Padded Title  \n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Rows[0]["title"] != "Padded Title" {
		t.Errorf("title = %q, want trimmed", p.Rows[0]["title"])
	}
}

func TestParseLowercasesHeader(t *testing.T) {
	data := []byte("Filename,Title\na.mp3,T\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Header[0] != "filename" || p.Header[1] != "title" {
		t.Errorf("header = %v, want lowercased", p.Header)
	}
	if p.Rows[0]["title"] != "T" {
		t.Errorf("row keyed by lowercased name is missing: %v", p.Rows[0])
	}
}

// Every row short is far more likely truncation than deliberate
// empty-tails, and padding a truncated file turns each omitted field into
// a delete instruction.
func TestParseSuspectedTruncation(t *testing.T) {
	data := []byte("filename,title,artist,genre\na.mp3,T,A\nb.mp3,T2,A2\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.SuspectedTruncation() {
		t.Fatal("every row short should be flagged as suspected truncation")
	}
	if !strings.Contains(p.IssueSummary(5), "TRUNCATED") {
		t.Error("summary should carry the truncation warning")
	}
}

// Some rows short is normal Excel behaviour, not truncation.
func TestParsePartialShortRowsNotTruncation(t *testing.T) {
	data := []byte("filename,title,artist,genre\na.mp3,T,A,G\nb.mp3,T2\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ShortRowCount() != 1 {
		t.Errorf("ShortRowCount = %d, want 1", p.ShortRowCount())
	}
	if p.SuspectedTruncation() {
		t.Error("one short row out of two must not be flagged as truncation")
	}
}

func TestParseBlankLinesIgnored(t *testing.T) {
	data := []byte("filename,title\na.mp3,T\n\nb.mp3,T2\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 2 {
		t.Errorf("rows = %d, want 2 (blank lines skipped)", len(p.Rows))
	}
	if len(p.Issues) != 0 {
		t.Errorf("blank lines should produce no issues: %v", p.Issues)
	}
}

func TestParseDuplicateFilenameKeepsLast(t *testing.T) {
	data := []byte("filename,title\na.mp3,First\na.mp3,Second\nb.mp3,B\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (duplicate collapsed to one)", len(p.Rows))
	}
	if p.Rows[0]["filename"] != "a.mp3" || p.Rows[0]["title"] != "Second" {
		t.Errorf("duplicate should keep the LAST row: %v", p.Rows[0])
	}
	if p.DuplicateCount() != 1 {
		t.Errorf("DuplicateCount = %d, want 1", p.DuplicateCount())
	}
	if !strings.Contains(p.IssueSummary(10), "duplicate") {
		t.Errorf("summary should report the duplicate: %q", p.IssueSummary(10))
	}
}
func TestParseRowNumbersSkipNothing(t *testing.T) {
	// one empty-filename row is skipped, so Rows[1] is source row 4
	data := []byte("filename,title\na.mp3,A\n,B\nc.mp3,C\n")
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(p.Rows))
	}
	if p.RowNumbers[0] != 2 || p.RowNumbers[1] != 4 {
		t.Errorf("RowNumbers = %v, want [2 4] (skipped row must not shift them)",
			p.RowNumbers)
	}
	if got := p.RowNumber("c.mp3"); got != 4 {
		t.Errorf("RowNumber(c.mp3) = %d, want 4", got)
	}
}
