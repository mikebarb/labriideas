// Package catalog owns the read-modify-write lifecycle of
// catalog.json.gz in R2.
//
// It exists because the server's updateMetadataHandler,
// deleteTrackHandler and uploadTrackHandler each contained a
// near-identical 60-line copy of the same sequence (HEAD → cache check →
// GET → gunzip → unmarshal → mutate → marshal → gzip → PUT → HEAD for
// new ETag → cache update). Three copies of subtle code drift apart;
// this package is the one copy, with every handler as a thin caller.
//
// ─── THE DENSE-CATALOG INVARIANT ───
//
// R2 metadata is SPARSE (absent field = no value, empty values never
// stored) but the catalog is DENSE (every schema field is present on
// every track, "" where there is no value — blank and null are the same
// thing to a person reading it). The crawler enforces this on
// regeneration; the patch/delete/append operations below enforce it on
// hot-patching. If either path stopped emitting empty strings, the
// catalog would oscillate between dense and sparse representations
// depending on which operation last touched it.
//
// ─── CACHE SEMANTICS (preserved from the handlers verbatim) ───
//
// Every mutation begins with a HEAD of catalog.json.gz. If the RAM
// cache's ETag matches R2's, the cached bytes are used and the download
// is skipped; otherwise a fresh GET occurs. After each successful save,
// a fresh HEAD fetches the NEW ETag and the cache is updated with the
// new bytes.
//
// Errors are RETURNED, never swallowed: the handlers decide how to map
// them to HTTP responses, and "R2 metadata updated but catalog sync
// failed" must reach the caller as a distinct outcome rather than the
// historical silent success.
package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mikebarb/labriideas-publisher/pkg/cache"
	"github.com/mikebarb/labriideas-publisher/pkg/schema"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

// Store couples the R2 client with the process-lifetime catalog cache.
// Both are injected, so tests can drive every path with fakes.
type Store struct {
	client *storage.Client
	cache  *cache.CatalogCache

	// onMissing, when set, is invoked whenever Load discovers the
	// catalog is ABSENT from R2 (distinct from a transient failure).
	// The server wires this to a guarded crawl trigger; CLI tools leave
	// it nil and trigger crawls explicitly. Every consumer of the
	// catalog routes through Load, so installing this hook once covers
	// the update, delete and upload handlers AND any future consumer —
	// without each call site repeating the errors.Is dance.
	onMissing func(reason string)
}

// TrackPatch is one existing track's pending changes.
type TrackPatch struct {
	Values  map[string]string
	Deletes []string
}

// NewTrack is one track to insert.
//
// audio-hash arrives in Values because it is computed by the caller from
// the source MP3 — creation is the one legitimate supply point for a
// derived field, exactly as with AppendTrack.
type NewTrack struct {
	Filename string
	Hash     string // object ETag, read back after upload
	Values   map[string]string
}

// ApplyReport summarises a batch.
type ApplyReport struct {
	Updated  int
	Inserted int
	NotFound []string // patched filenames with no catalog entry
	Existing []string // insert filenames already present (skipped)
}

// ApplyChanges applies many track changes in ONE load/save cycle.
//
// Per-record PatchTrack would be correct but wasteful: every call loads
// and saves the whole catalog, so a 2,064-record run would perform 2,064
// full read-modify-write cycles against R2. This performs one.
//
// PATCHES are guarded against derived fields (authoring them through an
// update is always a bug). INSERTS are not guarded — creation is where
// derived values are first observed.
//
// If nothing changes, nothing is written: no HTTP call, no new ETag.
func (s *Store) ApplyChanges(ctx context.Context, patches map[string]TrackPatch, inserts []NewTrack) (*ApplyReport, error) {
	report := &ApplyReport{}

	// Guard patches against derived-field authoring.
	for filename, p := range patches {
		for k := range p.Values {
			if schema.IsDerived(k) {
				return nil, fmt.Errorf("refusing to write derived field %q (track %q)", k, filename)
			}
		}
		for _, k := range p.Deletes {
			if schema.IsDerived(k) {
				return nil, fmt.Errorf("refusing to delete derived field %q (track %q)", k, filename)
			}
		}
	}

	if len(patches) == 0 && len(inserts) == 0 {
		return report, nil
	}

	data, _, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}

	tracks, ok := data["tracks"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("catalog has no tracks array")
	}

	existing := make(map[string]bool, len(tracks))
	for _, t := range tracks {
		if trackMap, ok := t.(map[string]interface{}); ok {
			if name, ok := trackMap["filename"].(string); ok {
				existing[name] = true
			}
		}
	}

	// --- patches ---
	matched := make(map[string]bool, len(patches))
	for _, t := range tracks {
		trackMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := trackMap["filename"].(string)
		p, ok := patches[name]
		if !ok {
			continue
		}
		// Dense invariant: purge non-schema pollution, then apply.
		for k := range trackMap {
			if !schema.IsSchemaField(k) {
				delete(trackMap, k)
			}
		}
		for k, v := range p.Values {
			trackMap[k] = v
		}
		for _, k := range p.Deletes {
			if schema.IsSchemaField(k) && !schema.IsDerived(k) {
				trackMap[k] = ""
			}
		}
		matched[name] = true
		report.Updated++
	}
	for filename := range patches {
		if !matched[filename] {
			report.NotFound = append(report.NotFound, filename)
		}
	}
	sort.Strings(report.NotFound)

	// --- inserts ---
	for _, ins := range inserts {
		if existing[ins.Filename] {
			report.Existing = append(report.Existing, ins.Filename)
			continue
		}
		entry := make(map[string]interface{}, len(schema.CatalogSchema))
		for _, f := range schema.CatalogSchema {
			switch f {
			case "filename":
				entry[f] = ins.Filename
			case "hash":
				entry[f] = ins.Hash
			default:
				if v, ok := ins.Values[f]; ok {
					entry[f] = v
				} else {
					entry[f] = "" // dense invariant
				}
			}
		}
		tracks = append(tracks, entry)
		existing[ins.Filename] = true
		report.Inserted++
	}
	sort.Strings(report.Existing)

	if report.Updated == 0 && report.Inserted == 0 {
		return report, nil // nothing changed → no write
	}

	data["tracks"] = tracks
	data["count"] = len(tracks)
	if err := s.Save(ctx, data); err != nil {
		return report, err
	}
	return report, nil
}

// ErrCatalogMissing reports that catalog.json.gz does not exist in R2.
// It is DISTINCT from a transient read failure because the remedy
// differs: a missing catalog cannot be patched (there is nothing to
// patch) — it must be REBUILT by a regeneration, which derives every
// entry from R2 metadata directly. Callers use errors.Is to detect this
// case and trigger a crawl instead of retrying the patch.
var ErrCatalogMissing = errors.New("catalog.json.gz does not exist in R2")

// NewStore builds a Store. Typically one per server process, constructed
// in main() alongside the storage client.
func NewStore(client *storage.Client, c *cache.CatalogCache) *Store {
	return &Store{client: client, cache: c}
}

// SetOnMissing installs the regeneration hook. The server calls this in
// main() after constructing the store; the CLI tools leave it nil.
// Kept as a setter rather than a NewStore parameter so the existing
// constructor signature (already in use) is unchanged.
func (s *Store) SetOnMissing(fn func(reason string)) {
	s.onMissing = fn
}

// Load returns the current catalog as a parsed dynamic map, using the
// RAM cache when its ETag matches R2's.
//
// The returned etag is R2's current ETag at load time — useful for
// diagnostics, not for write conditioning (the write path re-HEADs).
func (s *Store) Load(ctx context.Context) (map[string]interface{}, string, error) {
	// 1. Current ETag from R2 — the source of truth.
	r2Head, err := s.client.GetMetadata(ctx, catalogKey)
	if err != nil {
		// Distinguish "object absent" from "R2 unreachable/transient".
		// A missing catalog cannot be patched — there is nothing to
		// patch — so it is reported distinctly and the regeneration
		// hook fires: the catalog is a DERIVABLE artefact and the
		// crawler rebuilds it entirely from R2 metadata.
		var nf *types.NotFound
		if errors.As(err, &nf) {
			wrapped := fmt.Errorf("%w: %v", ErrCatalogMissing, err)
			if s.onMissing != nil {
				s.onMissing("catalog missing")
			}
			return nil, "", wrapped
		}
		return nil, "", fmt.Errorf("could not check catalog ETag: %w", err)
	}

	r2Etag := ""
	if r2Head.ETag != nil {
		r2Etag = *r2Head.ETag
	}

	// 2. Cache hit? Same ETag and non-empty bytes means RAM is fresh.
	var catalogGzBytes []byte
	ramEtag, ramBytes := s.cache.Get()
	if ramEtag == r2Etag && len(ramBytes) > 0 {
		catalogGzBytes = ramBytes
	} else {
		// 3. Cache stale or empty: fetch the absolute latest from R2.
		freshBytes, err := s.client.GetObjectBytes(ctx, catalogKey)
		if err != nil {
			s.cache.Clear()
			return nil, r2Etag, fmt.Errorf("could not fetch catalog from R2: %w", err)
		}
		catalogGzBytes = freshBytes
	}

	// 4. Decompress, unmarshal, null-normalise.
	data, err := decodeCatalog(catalogGzBytes)
	if err != nil {
		// A corrupt catalog must not be served from a stale-looking
		// cache again; clear so the next attempt re-downloads.
		s.cache.Clear()
		return nil, r2Etag, err
	}
	return data, r2Etag, nil
}

// Save serialises, compresses and uploads the catalog, then refreshes
// the RAM cache with the NEW ETag (fetched, not assumed).
func (s *Store) Save(ctx context.Context, data map[string]interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal catalog: %w", err)
	}

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	gzWriter.Name = "catalog.json"
	if _, err := gzWriter.Write(jsonBytes); err != nil {
		return fmt.Errorf("failed to compress catalog: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to finish compressing catalog: %w", err)
	}
	newGzBytes := buf.Bytes()

	if err := s.client.PutObjectBytes(ctx, catalogKey, newGzBytes); err != nil {
		return fmt.Errorf("failed to upload patched catalog to R2: %w", err)
	}

	// Refresh the cache with the NEW ETag. The upload SUCCEEDED even if
	// this HEAD fails — degrade to cache-clear, don't fail a durable
	// write.
	newHead, err := s.client.GetMetadata(ctx, catalogKey)
	if err != nil {
		log.Printf("Warning: catalog saved but new ETag check failed: %v", err)
		s.cache.Clear()
		return nil
	}
	newEtag := ""
	if newHead != nil && newHead.ETag != nil {
		newEtag = *newHead.ETag
	}
	s.cache.Update(newEtag, newGzBytes)
	return nil
}

// PatchTrack applies metadata changes to one track, INCLUDING deletions.
//
//   - values:  non-empty values — set/overwrite as-is.
//   - deletes: field names to blank out. Per the dense-catalog
//     invariant they are written as "" rather than removed, so the
//     catalog stays dense.
//
// The separation exists because the R2 metadata write
// (storage.UpdateMetadata) consumes the merged map — where an empty value
// means DELETE — while the catalog needs the OPERATOR'S INTENT: a
// deletion must become "" here, not vanish.
//
// Derived-field guard (B): any attempt to author a derived field is a
// hard error, not a silent drop. The handler's schema filter should
// have removed them already, so reaching this error means something
// bypassed the filter — always a bug worth reporting. AppendTrack is
// NOT guarded: creation is the one legitimate supply point, because
// creation is where derived values are first observed.
//
// Returns found=false when no track carries this filename; the caller
// decides whether that is an error or a no-op.
func (s *Store) PatchTrack(ctx context.Context, filename string, values map[string]string, deletes []string) (bool, error) {
	// --- Derived-field guard ---
	for k := range values {
		if schema.IsDerived(k) {
			return false, fmt.Errorf("refusing to write derived field %q", k)
		}
	}
	for _, k := range deletes {
		if schema.IsDerived(k) {
			return false, fmt.Errorf("refusing to delete derived field %q", k)
		}
	}

	data, _, err := s.Load(ctx)
	if err != nil {
		return false, err
	}

	tracks, ok := data["tracks"].([]interface{})
	if !ok {
		return false, fmt.Errorf("catalog has no tracks array")
	}

	found := false
	for _, t := range tracks {
		trackMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		if trackMap["filename"] == filename {
			found = true
			// Schema purge (preserved from the handler): any key not
			// in CatalogSchema is historical pollution, healed on
			// contact. Schema fields survive, including the derived
			// ones — the purge removes pollution, not classification.
			for k := range trackMap {
				if !schema.IsSchemaField(k) {
					delete(trackMap, k)
				}
			}
			for k, v := range values {
				trackMap[k] = v
			}
			for _, k := range deletes {
				// IsDerived replaces the old hardcoded exclusions
				// (filename, hash): it means the same thing and stays
				// correct if the derived set ever changes. Values and
				// deletes never contain a derived field at this point
				// — the guard above rejected them — so this check is
				// depth, not the primary defence.
				if schema.IsSchemaField(k) && !schema.IsDerived(k) {
					trackMap[k] = ""
				}
			}
			break
		}
	}
	if !found {
		return false, nil
	}

	return true, s.Save(ctx, data)
}

// RemoveTrack deletes a track's entry and updates the count.
// Returns found=false when no track carries this filename.
func (s *Store) RemoveTrack(ctx context.Context, filename string) (bool, error) {
	data, _, err := s.Load(ctx)
	if err != nil {
		return false, err
	}

	tracks, ok := data["tracks"].([]interface{})
	if !ok {
		return false, fmt.Errorf("catalog has no tracks array")
	}

	updated := make([]interface{}, 0, len(tracks))
	found := false
	for _, t := range tracks {
		trackMap, ok := t.(map[string]interface{})
		if ok && trackMap["filename"] == filename {
			found = true
			continue // skip — being deleted
		}
		updated = append(updated, t)
	}
	if !found {
		return false, nil
	}

	data["tracks"] = updated
	data["count"] = len(updated)
	return true, s.Save(ctx, data)
}

// AppendTrack adds a new track entry (uploadTrackHandler step 6).
//
// The phantom "id" field the old handler injected is deliberately NOT
// written: it is not in CatalogSchema, the crawler never emits it, and
// PatchTrack's purge deletes it on first contact. Dropping it here means
// every writer of the catalog produces the same shape.
func (s *Store) AppendTrack(ctx context.Context, filename, hash string, values map[string]string) error {
	data, _, err := s.Load(ctx)
	if err != nil {
		return err
	}

	tracks, ok := data["tracks"].([]interface{})
	if !ok {
		return fmt.Errorf("catalog has no tracks array")
	}

	// Every schema field is present (dense invariant); derived fields
	// take their computed values.
	newTrack := make(map[string]interface{}, len(schema.CatalogSchema))
	for _, f := range schema.CatalogSchema {
		switch f {
		case "filename":
			newTrack[f] = filename
		case "hash":
			newTrack[f] = hash
		default:
			if v, ok := values[f]; ok {
				newTrack[f] = v
			} else {
				newTrack[f] = ""
			}
		}
	}

	data["tracks"] = append(tracks, newTrack)
	data["count"] = len(data["tracks"].([]interface{}))
	return s.Save(ctx, data)
}

// ─── internals ───

const catalogKey = "catalog.json.gz"

// decodeCatalog gunzips and unmarshals catalog bytes, normalising JSON
// nulls to "" so every consumer sees one representation.
func decodeCatalog(gzBytes []byte) (map[string]interface{}, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(gzBytes))
	if err != nil {
		return nil, fmt.Errorf("catalog gunzip failed: %w", err)
	}
	defer gzReader.Close()

	jsonBytes, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decompressed catalog: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, fmt.Errorf("catalog JSON unmarshal failed: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("catalog JSON decoded to a null document")
	}

	if tracks, ok := data["tracks"].([]interface{}); ok {
		for _, t := range tracks {
			if trackMap, ok := t.(map[string]interface{}); ok {
				for k, v := range trackMap {
					trackMap[k] = NormaliseValue(v)
				}
			}
		}
	}
	return data, nil
}

// NormaliseValue collapses a dynamic catalog value to the string form
// every comparison uses: null and "" are the same thing.
func NormaliseValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", val))
	}
}
