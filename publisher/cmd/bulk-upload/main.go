// bulk-upload applies a catalog CSV to R2 metadata and the catalog.
//
// Dry run by default: it prints what it would do and writes nothing.
// -apply performs the writes.
//
// The pipeline is csvio.Parse → diff.DiffBatch → (dry run | apply).
// Resume is free: every run re-diffs against live catalog state, so
// records already applied classify as Skip on the next run.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/cache"
	"github.com/mikebarb/labriideas-publisher/pkg/catalog"
	"github.com/mikebarb/labriideas-publisher/pkg/csvio"
	"github.com/mikebarb/labriideas-publisher/pkg/diff"
	"github.com/mikebarb/labriideas-publisher/pkg/media"
	"github.com/mikebarb/labriideas-publisher/pkg/params"
	"github.com/mikebarb/labriideas-publisher/pkg/schema"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

// preflightResult is the plan decided BEFORE any write happens.
type preflightResult struct {
	// Workable records will be processed.
	Workable []diff.BatchResult
	// Excluded records cannot be processed: no source file, unreadable
	// file, or duplicate audio. They are reported, never attempted.
	Excluded []exclusion
	// Hashes caches the audio-hash computed during pre-flight, keyed by
	// filename, so the apply phase does not re-read the file.
	Hashes map[string]string
}

// exclusion records a row that will not be processed, and why.
type exclusion struct {
	Filename string
	Row      int
	Reason   string
}

const defaultLogDir = `C:\Users\Barbara\labri_other_files\logs`

var logFile *os.File

func main() {
	// Load .env BEFORE the flags are declared: flag.String evaluates its
	// default argument at declaration time, so envOr() reads the
	// environment the moment the flag is created. Loading afterwards
	// leaves every env-derived default stuck at its fallback.
	_ = godotenv.Load()

	var (
		csvPath    = flag.String("csv", envOr("BULK_CSV_FILE", ""), "CSV file to import (required)")
		root       = flag.String("root", envOr("BULK_MP3_DIR", ""), "directory holding source MP3s, matched by filename")
		logDir     = flag.String("log-dir", envOr("BULK_LOG_DIR", defaultLogDir), "run log directory ('' to disable)")
		apply      = flag.Bool("apply", false, "perform writes (default: dry run)")
		yes        = flag.Bool("yes", false, "skip the confirmation prompt with -apply")
		limit      = flag.Int("limit", 0, "process at most N records (0 = all)")
		allowTrunc = flag.Bool("allow-truncated", false, "proceed even if every row is short (possible truncation)")
		listIssues = flag.Bool("list-issues", false, "list every CSV row issue, not just the first 20")
		skipSchema = flag.Bool("skip-schema-check", false, "skip the server schema checksum check")
		apiBase    = flag.String("api", "http://localhost:8080", "base URL of the Go server (schema check only)")
	)
	flag.Parse()

	// ── Three-way parameter resolution ─────────────────────────────
	// 1. explicit flag  2. env (already in the flag defaults)  3. dialog
	// flag.Visit() lists only the flags the operator set on the command
	// line, so changed["csv"] distinguishes "flag won" from "env value
	// arrived via the default".
	changed := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { changed[f.Name] = true })

	// log-dir has an existing contract: an explicit '' disables logging.
	// Resolve treats an explicit empty as a usage error, so intercept
	// that case instead of handing it to Resolve.
	logDirDisabled := changed["log-dir"] && *logDir == ""

	if !logDirDisabled {
		// log-dir is optional: a cancelled dialog or a missing dialog
		// falls back to console-only, never exits.
		v, err := params.Resolve(params.Spec{
			Flag: "log-dir", Env: "BULK_LOG_DIR", What: "log directory", Dir: true,
		}, *logDir, changed["log-dir"])
		if err != nil {
			log.Printf("log dir unavailable (%v) — logging to console only", err)
			*logDir = ""
		} else {
			*logDir = v
		}
	}

	// csv and root are required: no value from flag, env, or dialog is
	// a usage error (exit 2).
	*csvPath = resolveOrDie(params.Spec{
		Flag: "csv", Env: "BULK_CSV_FILE", What: "CSV file", Filter: "*.csv",
	}, *csvPath, changed["csv"])
	*root = resolveOrDie(params.Spec{
		Flag: "root", Env: "BULK_MP3_DIR", What: "MP3 directory", Dir: true,
	}, *root, changed["root"])

	setupLogging(*logDir)
	if logFile != nil {
		defer logFile.Close()
	}

	log.Printf("CSV:      %s", *csvPath)
	log.Printf("MP3 dir:  %s", *root)
	log.Printf("Log dir:  %s", *logDir)
	log.Printf("Mode:     %s", map[bool]string{true: "APPLY", false: "DRY RUN"}[*apply])

	if *apply && *root == "" {
		log.Fatal("-apply requires -root (source files for New records)")
	}

	ctx := context.Background()

	client, store := buildClient()

	if !*skipSchema {
		checkServerSchema(*apiBase)
	}

	// ── 1. Parse ──────────────────────────────────────────────
	raw, err := os.ReadFile(*csvPath)
	if err != nil {
		log.Fatalf("read CSV: %v", err)
	}
	parsed, err := csvio.Parse(raw)
	if err != nil {
		log.Fatalf("CSV rejected: %v", err)
	}

	maxIssues := 20
	if *listIssues {
		maxIssues = len(parsed.Issues)
	}
	if s := parsed.IssueSummary(maxIssues); s != "" {
		fmt.Print(s)
		fmt.Println()
	}
	if parsed.SuspectedTruncation() && !*allowTrunc {
		log.Fatal("refusing to continue: every row is short. " +
			"A blank cell in a present column is a DELETE. " +
			"Use -allow-truncated only if you are certain.")
	}

	// ── 2. Catalog index ─────────────────────────────────────
	index, err := loadCatalogIndex(ctx, store)
	if err != nil {
		if errors.Is(err, catalog.ErrCatalogMissing) {
			log.Fatal("catalog.json.gz is missing from R2 — start the server so warmup " +
				"can rebuild it, then rerun")
		}
		log.Fatalf("read catalog: %v", err)
	}

	// ── 3. Diff ──────────────────────────────────────────────
	results, orphans := diff.DiffBatch(
		parsed.Rows, parsed.PresentColumns, index, schema.CatalogSchemaWritable)

	if *limit > 0 && len(results) > *limit {
		results = results[:*limit]
	}

	// ── 4a. PRE-FLIGHT: decide everything before any write ──
	//
	// Every record that cannot be processed is identified HERE, before
	// the operator confirms and before a single byte is written. The
	// apply phase then iterates only the workable set, so an excluded
	// row can never fail late in a long run.
	hashCache := media.NewHashCache()
	pf := preflightResult{Hashes: make(map[string]string)}

	// The hash index starts from the catalog: an existing track's
	// audio-hash blocks a new upload of identical audio.
	hashIndex := buildAudioHashIndex(index)

	// the source-file index — the disk's actual names, read once.
	// Every file decision below compares against these exact strings.
	sourceFiles, err := buildSourceFileIndex(*root)
	if err != nil {
		log.Fatalf("cannot read MP3 dir %s: %v", *root, err)
	}
	log.Printf("MP3 dir contains %d file(s)", len(sourceFiles))

	for _, r := range results {
		if r.Outcome != diff.OutcomeNew {
			// Updates and skips need no local file.
			pf.Workable = append(pf.Workable, r)
			continue
		}

		row := parsed.RowNumber(r.Filename)

		// File lookup is a case-sensitive string comparison against the
		// directory listing — steps 1–4 of the agreed rule: find the
		// file, read its actual name, compare exactly, accept only on
		// a true match. NTFS would resolve any case; this check never
		// asks it to.
		if _, ok := sourceFiles[r.Filename]; !ok {
			// Distinguish "absent" from "case mismatch" so the report
			// names the disk's version of the name and the fix is
			// obvious (correct the CSV, or rename the file).
			reason := "no source file in " + *root
			for disk := range sourceFiles {
				if strings.EqualFold(disk, r.Filename) {
					reason = fmt.Sprintf("case mismatch: disk has %q", disk)
					break
				}
			}
			pf.Excluded = append(pf.Excluded, exclusion{
				Filename: r.Filename, Row: row,
				Reason: reason,
			})
			continue
		}

		// Exact match confirmed: the disk name IS the CSV name, so this
		// path is correct by construction.
		srcPath := filepath.Join(*root, r.Filename)

		hash, _, err := hashCache.Hash(srcPath)
		if err != nil {
			pf.Excluded = append(pf.Excluded, exclusion{
				Filename: r.Filename, Row: row,
				Reason: "audio-hash failed: " + err.Error(),
			})
			continue
		}

		if dupOf, isDup := hashIndex[hash]; isDup {
			pf.Excluded = append(pf.Excluded, exclusion{
				Filename: r.Filename, Row: row,
				Reason: "duplicate audio of " + dupOf,
			})
			continue
		}

		// Register now, so a duplicate WITHIN this CSV is caught too.
		hashIndex[hash] = r.Filename
		pf.Hashes[r.Filename] = hash
		pf.Workable = append(pf.Workable, r)
	}

	// ── 4b. Show the plan — workable rows only ────────────────
	//
	// Excluded rows are not rendered here. They were never going to
	// be processed, and a plan that lists them as NEW and then
	// retracts them in the exclusion block makes the reader reconcile
	// the two lists by hand. The plan is the workable set; the
	// exclusion report is its complement.
	for _, r := range pf.Workable {
		switch r.Outcome {
		case diff.OutcomeNew:
			say("  Row %-5d%-9s%s\n", parsed.RowNumber(r.Filename), "NEW", r.Filename)
			renderNew(findRow(parsed, r.Filename), parsed)
		case diff.OutcomeUpdate:
			say("  Row %-5d%-9s%s\n", parsed.RowNumber(r.Filename), "UPDATE", r.Filename)
			say("%s", renderChanges(index[r.Filename], r.Diff))
		}
	}

	// ── 4c. Report the exclusions ────────────────────────────
	if len(pf.Excluded) > 0 {
		say("\n⛔ EXCLUDED — these rows will NOT be processed (%d):\n", len(pf.Excluded))
		for i, e := range pf.Excluded {
			if i >= 20 {
				say("    … and %d more\n", len(pf.Excluded)-20)
				break
			}
			say("    Row %-5d%-45s %s\n", e.Row, e.Filename, e.Reason)
		}
	}

	// ── 4d. Tally — counted from the WORKABLE set ─────────────
	// This must come after pre-flight, and must count pf.Workable
	// rather than results: a row that is excluded will not be
	// uploaded, so counting it as "to upload" describes an intent
	// that will not happen. Excluded rows are counted separately.
	nNew, nUpd, nSkip := 0, 0, 0
	for _, r := range pf.Workable {
		switch r.Outcome {
		case diff.OutcomeNew:
			nNew++
		case diff.OutcomeUpdate:
			nUpd++
		default:
			nSkip++
		}
	}
	nExcluded := len(pf.Excluded)

	say("\n=== DIFFING RESULTS ===\n")
	say("🟢 NEW (To Upload):      %d\n", nNew)
	say("🟡 CHANGED (To Update):  %d\n", nUpd)
	say("⚪ UNCHANGED (Skip):     %d\n", nSkip)
	say("⛔ EXCLUDED (not run):   %d\n", nExcluded)
	if len(orphans) > 0 {
		say("🔵 ORPHANS (report only): %d\n", len(orphans))
	}

	if !*apply {
		say("\nDry run — nothing written. Re-run with -apply to perform these changes.\n")
		return
	}

	// ── 4e. Confirmation gate ────────────────────────────────
	// The old bulk tool prompted before writing, and that guard is
	// worth keeping: a dry run is cheap to re-read, a partial apply is
	// not. -yes skips it for scripted runs.
	if !*yes {
		say("\nApply %d change(s) — %d upload(s), %d metadata update(s)? [y/N]: ",
			nNew+nUpd, nNew, nUpd)
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			say("Aborted by the user.\n")
			return
		}
	}

	// ── 5. Apply ─────────────────────────────────────────────
	fmt.Println("\nApplying…")

	patches := make(map[string]catalog.TrackPatch, nUpd)
	var inserts []catalog.NewTrack
	var failed []string

	actioned := 0
	actionTotal := nNew + nUpd

	for _, r := range pf.Workable {
		row := parsed.RowNumber(r.Filename)

		switch r.Outcome {
		case diff.OutcomeSkip:
			continue

		case diff.OutcomeUpdate:
			actioned++
			merged := make(map[string]string, len(r.Diff.Values)+len(r.Diff.Deletes))
			for k, v := range r.Diff.Values {
				merged[k] = v
			}
			for _, k := range r.Diff.Deletes {
				merged[k] = ""
			}

			log.Printf("[%d/%d] Row %-5d UPDATE  %s",
				actioned, actionTotal, row, r.Filename)

			if err := client.UpdateMetadata(ctx, r.Filename, merged); err != nil {
				log.Printf("[%d/%d] Row %-5d FAILED  %s: %v",
					actioned, actionTotal, row, r.Filename, err)
				failed = append(failed, r.Filename)
				continue
			}
			patches[r.Filename] = catalog.TrackPatch{
				Values:  r.Diff.Values,
				Deletes: r.Diff.Deletes,
			}

		case diff.OutcomeNew:
			actioned++
			srcPath := filepath.Join(*root, r.Filename)

			// The hash came from the pre-flight cache — no second read.
			audioHash := pf.Hashes[r.Filename]

			log.Printf("[%d/%d] Row %-5d UPLOAD  %s",
				actioned, actionTotal, row, r.Filename)

			values := make(map[string]string)
			if rowIdx := findRow(parsed, r.Filename); rowIdx >= 0 {
				for _, f := range schema.CatalogSchemaWritable {
					if v := parsed.Rows[rowIdx][f]; v != "" {
						values[f] = v
					}
				}
			}
			values["audio-hash"] = audioHash

			etag, err := uploadNewFile(ctx, client, r.Filename, srcPath, values)
			if err != nil {
				log.Printf("[%d/%d] Row %-5d FAILED  %s: %v",
					actioned, actionTotal, row, r.Filename, err)
				failed = append(failed, r.Filename)
				continue
			}
			inserts = append(inserts, catalog.NewTrack{
				Filename: r.Filename, Hash: etag, Values: values,
			})
		}
	}

	// ── 6. One catalog write for the whole run ───────────────
	var catalogReport *catalog.ApplyReport
	if len(patches) > 0 || len(inserts) > 0 {
		report, err := store.ApplyChanges(ctx, patches, inserts)
		if err != nil {
			log.Printf("CATALOG WRITE FAILED: %v", err)
			log.Printf("R2 metadata is updated but the catalog is stale. " +
				"Trigger a regeneration (start the server, or POST /api/start-crawl) " +
				"to rebuild it from R2 metadata.")
		} else {
			catalogReport = report
			say("\nCatalog: %d updated, %d inserted\n", report.Updated, report.Inserted)
		}
	} else {
		say("\nNo catalog changes needed.\n")
	}

	// ── 7. Summary ───────────────────────────────────────────
	inserted, updated := 0, 0
	if catalogReport != nil {
		inserted, updated = catalogReport.Inserted, catalogReport.Updated
	}

	say("\n=== BULK UPLOAD COMPLETE ===\n")
	say("🟢 NEW (uploaded):        %d\n", inserted)
	say("🟡 CHANGED (updated):     %d\n", updated)
	say("⚪ UNCHANGED (skip):      %d\n", nSkip)
	say("⛔ EXCLUDED (not run):    %d\n", nExcluded)
	say("❌ FAILED:                %d\n", len(failed))

	// The exclusions are repeated at the end, in full, because the
	// operator reading the tail of a long log needs the exceptions in
	// front of them rather than buried in the plan.
	if nExcluded > 0 {
		say("\n⚠️  %d row(s) were EXCLUDED and NOT processed:\n", nExcluded)
		for i, e := range pf.Excluded {
			if i >= 20 {
				say("    … and %d more\n", nExcluded-20)
				break
			}
			say("    Row %-5d%-45s %s\n", e.Row, e.Filename, e.Reason)
		}
	}
	if len(failed) > 0 {
		say("\n❌ Failed (%d):\n", len(failed))
		for i, fn := range failed {
			if i >= 20 {
				say("    … and %d more (see log)\n", len(failed)-20)
				break
			}
			say("    Row %-5d%s\n", parsed.RowNumber(fn), fn)
		}
	}

	// ── 8. Exit code ─────────────────────────────────────────
	// 0 = everything requested was processed
	// 1 = completed, but some rows were excluded (data problems)
	// 2 = completed, but some rows failed at runtime
	// 3 = both
	// Higher is more severe; non-zero means "do not treat this as a
	// clean run".
	code := 0
	if nExcluded > 0 {
		code = 1
	}
	if len(failed) > 0 {
		if code == 1 {
			code = 3
		} else {
			code = 2
		}
	}
	if code != 0 {
		os.Exit(code)
	}

}

// ─────────────────────────────────────────────────────────────

// resolveOrDie resolves a required parameter; a missing value is a
// usage error (exit 2, per the agreed exit-code scheme).
func resolveOrDie(spec params.Spec, val string, changed bool) string {
	v, err := params.Resolve(spec, val, changed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return v
}

func buildAudioHashIndex(index map[string]map[string]string) map[string]string {
	byHash := make(map[string]string, len(index))
	for filename, row := range index {
		if h := row["audio-hash"]; h != "" {
			byHash[h] = filename
		}
	}
	return byHash
}

// buildSourceFileIndex scans the MP3 directory once and returns the
// filenames exactly as stored on disk — the case is the disk's case,
// not the CSV's case. Used by pre-flight so that file lookup is a
// case-sensitive string comparison against names read from the
// directory, never a filesystem resolve (NTFS would resolve "test1"
// to "Test1" and hide the mismatch).
func buildSourceFileIndex(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	idx := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".mp3") {
			idx[e.Name()] = struct{}{}
		}
	}
	return idx, nil
}

func buildClient() (*storage.Client, *catalog.Store) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			os.Getenv("R2_ACCESS_KEY_ID"), os.Getenv("R2_SECRET_ACCESS_KEY"), "")),
		awsconfig.WithRegion("auto"))
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + os.Getenv("R2_ACCOUNT_ID") + ".r2.cloudflarestorage.com")
	})
	client := storage.NewClient(s3c, os.Getenv("R2_BUCKET_NAME"))
	// No onMissing hook: the CLI cannot start a crawl. It reports
	// ErrCatalogMissing so the operator can start the server instead.
	store := catalog.NewStore(client, cache.NewCatalogCache())
	return client, store
}

func loadCatalogIndex(ctx context.Context, store *catalog.Store) (map[string]map[string]string, error) {
	data, _, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	tracks, ok := data["tracks"].([]interface{})
	if !ok {
		return nil, errors.New("catalog has no tracks array")
	}
	index := make(map[string]map[string]string, len(tracks))
	for _, t := range tracks {
		m, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		row := make(map[string]string, len(m))
		for k, v := range m {
			row[k] = catalog.NormaliseValue(v)
		}
		index[row["filename"]] = row
	}
	return index, nil
}

// checkServerSchema compares this binary's schema checksum against the
// server's. A mismatch means one side is stale — and a bulk run writing
// fields the running server would purge is worth catching first.
func checkServerSchema(apiBase string) {
	resp, err := http.Get(strings.TrimRight(apiBase, "/") + "/api/schema/catalog")
	if err != nil {
		log.Fatalf("schema check: %v (use -skip-schema-check to bypass)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("schema check: server returned %d", resp.StatusCode)
	}
	var got struct {
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		log.Fatalf("schema check: bad response: %v", err)
	}
	if want := schema.Checksum(); got.Checksum != want {
		log.Fatalf("schema drift: server %s, this binary %s — rebuild one of them "+
			"(use -skip-schema-check to bypass)", got.Checksum[:12], want[:12])
	}
}

// renderChanges is the dry-run rendering. It lives here, not in pkg/diff,
// because only the caller holds the OLD values needed for "old → new".
func renderChanges(old map[string]string, rd diff.RecordDiff) string {
	fields := make([]string, 0, len(rd.Actions))
	for f := range rd.Actions {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	var b strings.Builder
	for _, f := range fields {
		switch rd.Actions[f] {
		case diff.Set:
			fmt.Fprintf(&b, "             %-10s %q → %q\n", f, old[f], rd.Values[f])
		case diff.Delete:
			fmt.Fprintf(&b, "             %-10s %q → (deleted)\n", f, old[f])
		}
	}
	return b.String()
}

func renderNew(rowIdx int, parsed *csvio.ParsedCSV) {
	if rowIdx < 0 || rowIdx >= len(parsed.Rows) {
		return
	}
	row := parsed.Rows[rowIdx]
	fields := make([]string, 0, len(row))
	for k := range row {
		if schema.IsWritable(k) && k != "filename" {
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)
	for _, f := range fields {
		if row[f] != "" {
			say("             %-10s %q\n", f, row[f])
		}
	}
}

func findRow(parsed *csvio.ParsedCSV, filename string) int {
	for i, r := range parsed.Rows {
		if r["filename"] == filename {
			return i
		}
	}
	return -1
}

// uploadNewFile streams one local MP3 to R2 with its metadata in the
// same PUT, then reads back the ETag for the catalog's derived "hash".
//
// Metadata at PUT time is deliberately preferred over PUT-then-
// UpdateMetadata: the latter is a copy-over-self, so a new upload would
// cost two R2 writes and two body transfers. One write, one transfer.
//
// filename and hash are excluded from the metadata map: filename is the
// object key, and hash IS the ETag — storing either would duplicate or
// contradict the catalog's derived fields.
func uploadNewFile(ctx context.Context, client *storage.Client,
	key, path string, values map[string]string) (string, error) {

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	meta := make(map[string]string, len(values))
	for k, v := range values {
		if k == "filename" || k == "hash" {
			continue
		}
		meta[k] = v
	}

	if err := client.UploadFile(ctx, key, stat.Size(), f, meta); err != nil {
		return "", err
	}

	head, err := client.GetMetadata(ctx, key)
	if err != nil {
		return "", fmt.Errorf("uploaded but could not read ETag: %w", err)
	}
	if head == nil || head.ETag == nil {
		return "", errors.New("uploaded but ETag absent")
	}
	return strings.Trim(*head.ETag, `"`), nil
}

// setupLogging mirrors the old tool's behaviour: one file per run,
// timestamped, with log.Printf going to both console and file.
func setupLogging(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("warning: log directory unavailable (%v); console only", err)
		return
	}
	path := filepath.Join(dir,
		fmt.Sprintf("bulk-upload-%s.log", time.Now().Format("20060102-150405")))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("warning: could not open log file (%v); console only", err)
		return
	}
	logFile = f
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	fmt.Printf("📝 Logging to: %s\n", path)
}

// say writes to console AND the run log, so the dry-run plan is
// recoverable from the log file afterwards. Use it for report output;
// use log.Printf for events that want a timestamp.
func say(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Print(msg)
	if logFile != nil {
		fmt.Fprint(logFile, msg)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
