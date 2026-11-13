package diff

import (
	"reflect"
	"testing"
)

var writable = []string{
	"title", "artist", "speaker", "category", "genre",
	"keywords", "year", "topten",
}

func cols(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func TestDiffRecord(t *testing.T) {
	tests := []struct {
		name    string
		present map[string]bool
		csv     map[string]string
		exist   map[string]string
		sets    map[string]string
		dels    []string
		changed bool
	}{
		{
			name:    "identical values → no change",
			present: cols("filename", "title", "speaker"),
			csv:     map[string]string{"filename": "a.mp3", "title": "A", "speaker": "S"},
			exist:   map[string]string{"title": "A", "speaker": "S"},
			sets:    map[string]string{},
			changed: false,
		},
		{
			name:    "one field changed → set",
			present: cols("filename", "title", "speaker"),
			csv:     map[string]string{"filename": "a.mp3", "title": "B", "speaker": "S"},
			exist:   map[string]string{"title": "A", "speaker": "S"},
			sets:    map[string]string{"title": "B"},
			changed: true,
		},
		{
			name:    "blank in present column → delete",
			present: cols("filename", "topten"),
			csv:     map[string]string{"filename": "a.mp3", "topten": ""},
			exist:   map[string]string{"topten": "7"},
			sets:    map[string]string{},
			dels:    []string{"topten"},
			changed: true,
		},
		{
			name:    "blank against already-empty → no change",
			present: cols("filename", "topten"),
			csv:     map[string]string{"filename": "a.mp3", "topten": ""},
			exist:   map[string]string{"topten": ""},
			sets:    map[string]string{},
			changed: false,
		},
		{
			name:    "absent column → keep, even though R2 has a value",
			present: cols("filename", "speaker"),
			csv:     map[string]string{"filename": "a.mp3", "speaker": "S"},
			exist:   map[string]string{"speaker": "S", "artist": "Someone"},
			sets:    map[string]string{},
			changed: false,
		},
		{
			name:    "whitespace-only difference → no change",
			present: cols("filename", "title"),
			csv:     map[string]string{"filename": "a.mp3", "title": "A  "},
			exist:   map[string]string{"title": "A"},
			sets:    map[string]string{},
			changed: false,
		},
		{
			name:    "multi-field change with a delete",
			present: cols("filename", "title", "year", "genre"),
			csv:     map[string]string{"filename": "a.mp3", "title": "B", "year": "1999", "genre": ""},
			exist:   map[string]string{"title": "A", "year": "", "genre": "Rock"},
			sets:    map[string]string{"title": "B", "year": "1999"},
			dels:    []string{"genre"},
			changed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := DiffRecord(tc.present, tc.csv, tc.exist, writable)
			if d.Changed != tc.changed {
				t.Fatalf("Changed = %v, want %v", d.Changed, tc.changed)
			}
			if !reflect.DeepEqual(d.Values, tc.sets) {
				t.Errorf("Values = %v, want %v", d.Values, tc.sets)
			}
			if len(tc.dels) == 0 {
				if len(d.Deletes) != 0 {
					t.Errorf("Deletes = %v, want none", d.Deletes)
				}
			} else if !reflect.DeepEqual(d.Deletes, tc.dels) {
				t.Errorf("Deletes = %v, want %v", d.Deletes, tc.dels)
			}
		})
	}
}

func TestDiffBatchClassifiesAndFindsOrphans(t *testing.T) {
	index := map[string]map[string]string{
		"a.mp3": {"filename": "a.mp3", "title": "A"},
		"b.mp3": {"filename": "b.mp3", "title": "B"},
		"c.mp3": {"filename": "c.mp3", "title": "C"},
	}
	rows := []map[string]string{
		{"filename": "a.mp3", "title": "A"},  // skip
		{"filename": "b.mp3", "title": "B2"}, // update
		{"filename": "d.mp3", "title": "D"},  // new
	}

	results, orphans := DiffBatch(rows, cols("filename", "title"), index, writable)

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Outcome != OutcomeSkip {
		t.Errorf("a.mp3 outcome = %v, want skip", results[0].Outcome)
	}
	if results[1].Outcome != OutcomeUpdate {
		t.Errorf("b.mp3 outcome = %v, want update", results[1].Outcome)
	}
	if results[2].Outcome != OutcomeNew {
		t.Errorf("d.mp3 outcome = %v, want new", results[2].Outcome)
	}
	if !reflect.DeepEqual(orphans, []string{"c.mp3"}) {
		t.Errorf("orphans = %v, want [c.mp3]", orphans)
	}
}
