// bulk-deleter removes catalogued tracks from R2, THROUGH THE SERVER.
//
// Deletions go through the server because deleteTrackHandler already owns
// the two-step contract (delete the object, hot-patch the catalog) and the
// partial-success reporting. A DELETE has no body, so there is no
// bandwidth argument for going direct.
//
// GATE: a name is deleted only if it exists in the catalog. A CSV may
// legitimately list files known not to be present, and "already gone" is
// not a reason to issue a delete.
//
// VERIFY then REBUILD: after deleting, each key is HEADed to prove the
// object is gone, then ONE regeneration rebuilds the catalog from R2.
// The regeneration is not decoration — concurrent hot-patches can lose a
// catalog update (Load→Save has no compare-and-swap), so the rebuild is
// what guarantees the final catalog reflects R2 exactly.
//
// DRY RUN BY DEFAULT. -apply performs the deletions.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/cache"
	"github.com/mikebarb/labriideas-publisher/pkg/catalog"
	"github.com/mikebarb/labriideas-publisher/pkg/client"
	"github.com/mikebarb/labriideas-publisher/pkg/csvio"
	"github.com/mikebarb/labriideas-publisher/pkg/jobs"
	"github.com/mikebarb/labriideas-publisher/pkg/params"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

func main() {

	godotenv.Load()

	var (
		csvPath = flag.String("csv", envOr("BULK_CSV_FILE", ""), "CSV listing tracks to delete (required)")
		apply   = flag.Bool("apply", false, "perform deletions (default: dry run)")
		yes     = flag.Bool("yes", false, "skip the confirmation prompt with -apply")
		conc    = flag.Int("concurrency", 1, "parallel delete requests")
	)
	flag.Parse()

	// ── Three-way parameter resolution ─────────────────────────────
	// 1. explicit flag  2. env (already in the flag default)  3. dialog
	// flag.Visit() lists only the flags the operator set on the command
	// line, so changed["csv"] distinguishes "flag won" from "env value
	// arrived via the default".
	changed := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { changed[f.Name] = true })

	*csvPath = resolveOrDie(params.Spec{
		Flag: "csv", Env: "BULK_CSV_FILE", What: "CSV file", Filter: "*.csv",
	}, *csvPath, changed["csv"])

	log.Printf("BULK_CSV_FILE:      %s", *csvPath)
	log.Printf("Mode:     %s", map[bool]string{true: "APPLY", false: "DRY RUN"}[*apply])

	if *conc < 1 {
		log.Fatal("-concurrency must be at least 1")
	}

	ctx := context.Background()

	// ── 1. Parse ─────────────────────────────────────────────
	raw, err := os.ReadFile(*csvPath)
	if err != nil {
		log.Fatalf("read CSV: %v", err)
	}
	parsed, err := csvio.Parse(raw)
	if err != nil {
		log.Fatalf("CSV rejected: %v", err)
	}
	if s := parsed.IssueSummary(20); s != "" {
		fmt.Print(s)
		fmt.Println()
	}
	// Names in CSV order, carrying their source rows. File order rather
	// than alphabetical: the plan then reads like the spreadsheet, and a
	// failure list stays in a sequence the operator can work down.
	names := make([]string, 0, len(parsed.Rows))
	rowOf := make(map[string]int, len(parsed.Rows))
	for i, r := range parsed.Rows {
		fn := r["filename"]
		if fn == "" {
			continue
		}
		if _, dup := rowOf[fn]; dup {
			continue
		}
		rowOf[fn] = parsed.RowNumbers[i]
		names = append(names, fn)
	}
	if len(names) == 0 {
		log.Fatal("no filenames found in CSV — nothing to delete")
	}

	// ── 2. Authenticate to the server ────────────────────────
	c := client.NewFromEnv()
	if err := c.EnsureAuthenticated(); err != nil {
		log.Fatalf("authentication failed: %v", err)
	}
	defer c.Logout()

	client.Printf("Target: %s (%s)", c.BaseURL, client.EnvironmentLabel)
	client.Printf("%s", c.Identity())

	// ── 3. GATE: which names are actually catalogued? ────────
	fmt.Println("\n--- Fetching current catalog ---")
	catalogMap, _, err := c.FetchCatalog()
	if err != nil {
		log.Fatalf("failed to fetch catalog: %v", err)
	}
	fmt.Printf("Loaded %d catalogued track(s).\n", len(catalogMap))

	var toDelete, notInCatalog []string
	for _, fn := range names {
		if _, exists := catalogMap[fn]; exists {
			toDelete = append(toDelete, fn)
		} else {
			notInCatalog = append(notInCatalog, fn)
		}
	}
	// ── PLAN BLOCK: what will be deleted ────────
	fmt.Printf("\n=== DELETE PLAN ===\n")
	fmt.Printf("🔴 To delete:            %d\n", len(toDelete))
	fmt.Printf("⚪ Not in catalog (skip): %d\n", len(notInCatalog))
	for i, fn := range notInCatalog {
		if i >= 20 {
			fmt.Printf("    … and %d more\n", len(notInCatalog)-20)
			break
		}
		fmt.Printf("    Row %-5d%s\n", rowFor(rowOf, fn), fn)
	}

	if len(toDelete) == 0 {
		fmt.Println("\nNothing to delete.")
		return
	}

	if !*apply {
		fmt.Printf("\nDry run — would delete %d file(s):\n", len(toDelete))
		for i, fn := range toDelete {
			if i >= 40 {
				fmt.Printf("  … and %d more\n", len(toDelete)-40)
				break
			}
			fmt.Printf("  Row %-5dDELETE  %s\n", rowFor(rowOf, fn), fn)
		}
		fmt.Println("\nRe-run with -apply to delete.")
		return
	}

	if !*yes {
		fmt.Printf("\nDelete %d file(s) from R2 permanently? [y/N]: ", len(toDelete))
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	// ── 4. Delete via the server ─────────────────────────────
	fmt.Println("\n--- Deleting via server ---")
	var deleted, partial, failed []string
	var mu sync.Mutex

	jobs.RunParallel(toDelete, *conc,
		func(fn string) error {
			return c.DeleteTrack(fn)
		},
		func(fn string) {
			mu.Lock()
			defer mu.Unlock()
			deleted = append(deleted, fn)
			log.Printf("  Row %-5dDELETED   %s", rowFor(rowOf, fn), fn)
		},
		func(fn string, err error) {
			mu.Lock()
			defer mu.Unlock()
			var pe *client.PartialDeleteError
			if errors.As(err, &pe) {
				// Object is gone; the catalog hot-patch failed. The final
				// regeneration repairs it — so this is not a deletion
				// failure, only a sync note.
				partial = append(partial, fn)
				log.Printf("  Row %-5dPARTIAL   %s: %v", rowFor(rowOf, fn), fn, err)
				return
			}
			failed = append(failed, fn)
			log.Printf("  Row %-5dFAILED    %s: %v", rowFor(rowOf, fn), fn, err)
		},
	)

	// Sort the outcome lists by row, so the summaries read in file order.
	sortByRow := func(s []string) {
		sort.Slice(s, func(i, j int) bool {
			return rowFor(rowOf, s[i]) < rowFor(rowOf, s[j])
		})
	}
	sortByRow(deleted)
	sortByRow(partial)
	sortByRow(failed)

	fmt.Printf("\nDeletes: %d ok, %d partial, %d failed\n",
		len(deleted), len(partial), len(failed))

	// ── 5. VERIFY in R2 ──────────────────────────────────────
	// A delete that reported success may still have left the object (a
	// transport retry, a proxy, an unexpected 4xx). HEAD is the proof.
	fmt.Println("\n--- Verifying objects are gone from R2 ---")
	r2, store := buildR2()

	raced := append(append([]string{}, deleted...), partial...)
	var stillPresent []string
	for i, fn := range raced {
		if (i+1)%50 == 0 {
			log.Printf("  … verified %d/%d", i+1, len(raced))
		}
		exists, err := objectExists(ctx, r2, fn)
		if err != nil {
			log.Printf("  verify error for %s: %v", fn, err)
			failed = append(failed, fn)
			continue
		}
		if exists {
			stillPresent = append(stillPresent, fn)
		}
	}
	sort.Strings(stillPresent)

	if len(stillPresent) > 0 {
		sortByRow(stillPresent)
		fmt.Printf("\n⚠️  %d object(s) still present in R2 after deletion:\n", len(stillPresent))
		for i, fn := range stillPresent {
			if i >= 10 {
				fmt.Printf("    … and %d more\n", len(stillPresent)-10)
				break
			}
			fmt.Printf("    Row %-5d%s\n", rowFor(rowOf, fn), fn)
		}
	}

	// ── 6. REBUILD: one authoritative catalog write-back ─────
	// Concurrent hot-patches can lose an update (Load→Save has no
	// compare-and-swap), so the per-request catalog state is not
	// trustworthy after a parallel run. A regeneration derives the
	// catalog from R2 enumeration and overwrites whatever is there.
	fmt.Println("\n--- Rebuilding catalog on the server ---")
	jobID, err := c.TriggerCrawl()
	if err != nil {
		log.Fatalf("could not start catalog rebuild: %v", err)
	}
	client.Printf("Crawl job %s started; waiting…", jobID)
	if err := c.PollCrawlStatus(jobID); err != nil {
		log.Fatalf("catalog rebuild failed: %v", err)
	}

	// ── 7. VERIFY the catalog has no entry for any deleted name ──
	fmt.Println("\n--- Verifying catalog entries are gone ---")
	after, _, err := store.Load(ctx)
	if err != nil {
		log.Fatalf("could not read the rebuilt catalog: %v", err)
	}
	remaining := make(map[string]bool, len(after))
	if tracks, ok := after["tracks"].([]interface{}); ok {
		for _, t := range tracks {
			if m, ok := t.(map[string]interface{}); ok {
				if fn, ok := m["filename"].(string); ok {
					remaining[fn] = true
				}
			}
		}
	}

	if len(stillPresent) > 0 {
		sortByRow(stillPresent)
		fmt.Printf("\n⚠️  %d object(s) still present in R2 after deletion:\n", len(stillPresent))
		for i, fn := range stillPresent {
			if i >= 10 {
				fmt.Printf("    … and %d more\n", len(stillPresent)-10)
				break
			}
			fmt.Printf("    Row %-5d%s\n", rowFor(rowOf, fn), fn)
		}
	}

	var stale []string
	for _, fn := range raced {
		if remaining[fn] {
			stale = append(stale, fn)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		sortByRow(stale)
		fmt.Printf("\n🔴 %d entr(y/ies) still in the catalog after rebuild:\n", len(stale))
		for i, fn := range stale {
			if i >= 10 {
				fmt.Printf("    … and %d more\n", len(stale)-10)
				break
			}
			fmt.Printf("    Row %-5d%s\n", rowFor(rowOf, fn), fn)
		}
	}

	// ── 8. Summary ───────────────────────────────────────────
	fmt.Printf("\n=== BULK DELETE COMPLETE ===\n")
	fmt.Printf("🟢 Deleted:               %d\n", len(deleted))
	fmt.Printf("🟡 Deleted, sync gap:     %d  (catalog repaired by rebuild)\n", len(partial))
	fmt.Printf("❌ Failed:                %d\n", len(failed))
	fmt.Printf("⚠️  Skipped (not catalogued): %d\n", len(notInCatalog))
	if len(stillPresent) > 0 {
		fmt.Printf("🔴 STILL IN R2:           %d  (deletion did not take effect)\n", len(stillPresent))
	}
	if len(stale) > 0 {
		fmt.Printf("🔴 STILL IN CATALOG:      %d  (rebuild did not clear them)\n", len(stale))
	}

	if len(stillPresent) > 0 || len(stale) > 0 || len(failed) > 0 {
		fmt.Println("\nReview required — the run did not converge.")
		os.Exit(1)
	}
	fmt.Println("\nAll deletions verified: objects gone from R2, entries gone from the catalog,")
	fmt.Println("catalog rebuilt and written back to R2.")
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

// rowFor returns the spreadsheet row for a filename, or 0 when the name
// did not come from the CSV (which would be a bug, but printing "Row 0"
// is better than a panic mid-run).
func rowFor(rowOf map[string]int, fn string) int {
	return rowOf[fn]
}

func buildR2() (*storage.Client, *catalog.Store) {
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
	c := storage.NewClient(s3c, os.Getenv("R2_BUCKET_NAME"))
	// No onMissing hook: the CLI cannot start a crawl itself, and a
	// missing catalog here means something is wrong worth reporting.
	return c, catalog.NewStore(c, cache.NewCatalogCache())
}

// objectExists reports whether the key is present in R2. A NotFound
// error is a definitive "no"; anything else is returned so the caller
// can distinguish "gone" from "could not tell".
func objectExists(ctx context.Context, c *storage.Client, key string) (bool, error) {
	if _, err := c.GetMetadata(ctx, key); err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
