// content-type-fix repairs the Content-Type of audio objects in R2.
//
// 2,027 library objects were written before UploadFile and
// PutObjectWithMetadata set audio/mpeg explicitly, so they carry
// application/octet-stream. This is a ONE-OFF repair, not an ongoing
// tool: the write paths are fixed, and this only corrects history.
//
// The mechanism is a server-side CopyObject onto itself with
// MetadataDirective REPLACE — the object body never leaves R2. It
// follows UpdateMetadata's write contract exactly (decode on read, send
// raw, verify before trusting) because the encoded-word corruption
// class applies to ANY REPLACE copy.
//
// Dry run by default: it HEADs everything, reports the Content-Type
// distribution and the metadata exposure, and writes nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/jobs"
	"github.com/mikebarb/labriideas-publisher/pkg/metadata"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

const defaultType = "audio/mpeg"

func main() {
	_ = godotenv.Load()

	var (
		applyFlag = flag.Bool("apply", false, "perform the rewrite (default: dry run)")
		keyFlag   = flag.String("key", "", "fix a single object key (smoke test); empty = whole bucket")
		conc      = flag.Int("concurrency", 4, "parallel HEAD/copy operations")
		wantType  = flag.String("type", defaultType, "Content-Type to set")
		limit     = flag.Int("limit", 0, "fix at most N objects (0 = all)")
	)
	flag.Parse()

	ctx := context.Background()
	client := buildClient()

	keys, err := selectKeys(ctx, client, *keyFlag)
	if err != nil {
		log.Fatalf("list objects: %v", err)
	}

	if len(keys) == 0 {
		fmt.Println("No .mp3 objects found — nothing to do.")
		return
	}

	if *limit > 0 && len(keys) > *limit {
		keys = keys[:*limit]
		log.Printf("Limited to the first %d object(s)", *limit)
	}

	log.Printf("Mode:     %s", map[bool]string{true: "APPLY", false: "DRY RUN"}[*applyFlag])
	log.Printf("Target:   %s", *wantType)
	log.Printf("Objects:  %d", len(keys))

	// Single-object mode: the smoke test, and the only place ETag
	// stability is measured (cost doesn't matter for one object).
	if *keyFlag != "" {
		smokeTest(ctx, client, *keyFlag, *wantType, *applyFlag)
		return
	}

	if !*applyFlag {
		toFix := reportDistribution(ctx, client, keys, *conc, *wantType)
		fmt.Printf("\nDry run — nothing written. Re-run with -apply to rewrite %d object(s).\n", toFix)
		return
	}

	fixed, already, failed := applyAll(ctx, client, keys, *conc, *wantType)

	fmt.Println("\n=== CONTENT-TYPE FIX COMPLETE ===")
	fmt.Printf("🟢 Fixed:           %d\n", fixed)
	fmt.Printf("⚪ Already correct: %d\n", already)
	fmt.Printf("❌ Failed:          %d\n", len(failed))
	if len(failed) > 0 {
		fmt.Println("\n❌ Failed objects:")
		for i, k := range failed {
			if i >= 20 {
				fmt.Printf("    … and %d more (see log)\n", len(failed)-20)
				break
			}
			fmt.Printf("    %s\n", k)
		}
		fmt.Println("\nRe-run to retry — the tool is idempotent; objects already fixed are skipped.")
		os.Exit(1)
	}
	fmt.Println("\nRe-run the dry run to confirm convergence (expect 0 to fix).")
	fmt.Println("If the catalog looks stale afterwards, regenerate it (start the server,")
	fmt.Println("or POST /api/start-crawl) — a copy should not change the ETag, but verify.")
}

// ─────────────────────────────────────────────────────────────

func buildClient() *storage.Client {
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
	return storage.NewClient(s3c, os.Getenv("R2_BUCKET_NAME"))
}

// selectKeys returns the objects to consider. Audio only: catalog.json.gz
// and any other non-.mp3 key is excluded by the extension filter — this
// tool must never rewrite the catalog's Content-Type.
func selectKeys(ctx context.Context, client *storage.Client, only string) ([]string, error) {
	if only != "" {
		return []string{only}, nil
	}
	all, err := client.ListObjectKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(all))
	for _, k := range all {
		if strings.EqualFold(filepath.Ext(k), ".mp3") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

// qEntry is one object with at least one '?' in its metadata values.
type qEntry struct {
	Key    string
	Fields []qField
}

// reportDistribution HEADs every object and prints the audit picture:
// what Content-Types exist, how many need fixing, how much of the
// library carries the metadata shapes that trigger encoding, and — the
// classification list — which field holds each '?'.
//
// All lists are collected in full and sorted before printing, so two
// runs over the same bucket produce identical output: concurrency
// determines completion order, not report order.
func reportDistribution(ctx context.Context, client *storage.Client, keys []string, conc int, want string) int {
	var (
		mu        sync.Mutex
		dist      = map[string]int{}
		toFix     int
		nonASCII  int
		headFails int
		repair    []string
		nonAscii  []string
		qEntries  []qEntry
	)

	jobs.RunParallel(keys, conc,
		func(k string) error {
			h, err := client.GetMetadata(ctx, k)
			if err != nil {
				return err
			}
			ct := "(none)"
			if h.ContentType != nil && *h.ContentType != "" {
				ct = *h.ContentType
			}
			na, _ := exposure(h.Metadata)
			qf := questionFields(h.Metadata)

			mu.Lock()
			defer mu.Unlock()
			dist[ct]++
			if ct != want {
				toFix++
				repair = append(repair, k)
			}
			if na {
				nonASCII++
				nonAscii = append(nonAscii, k)
			}
			if len(qf) > 0 {
				qEntries = append(qEntries, qEntry{Key: k, Fields: qf})
			}
			return nil
		},
		func(string) {},
		func(k string, err error) {
			mu.Lock()
			headFails++
			mu.Unlock()
			log.Printf("HEAD failed  %s: %v", k, err)
		},
	)

	sort.Strings(repair)
	sort.Strings(nonAscii)
	sort.Slice(qEntries, func(i, j int) bool { return qEntries[i].Key < qEntries[j].Key })

	// ── Content-Type distribution ──
	types := make([]string, 0, len(dist))
	for t := range dist {
		types = append(types, t)
	}
	sort.Strings(types)

	fmt.Println("\n=== CONTENT-TYPE DISTRIBUTION (.mp3 only) ===")
	for _, t := range types {
		fmt.Printf("  %-28s %d\n", t, dist[t])
	}
	fmt.Printf("\n  Objects needing repair:   %d\n", toFix)
	if headFails > 0 {
		fmt.Printf("  HEAD failures:            %d\n", headFails)
	}

	// ── Metadata exposure ──
	fmt.Println("\n=== METADATA EXPOSURE (value shapes that trigger encoding) ===")
	fmt.Printf("  non-ASCII value present:  %d\n", nonASCII)
	fmt.Printf("  '?' in a value:           %d\n", len(qEntries))

	if len(nonAscii) > 0 {
		fmt.Println("\n  Objects with a non-ASCII value:")
		for i, k := range nonAscii {
			if i >= 10 {
				fmt.Printf("    … and %d more\n", len(nonAscii)-10)
				break
			}
			fmt.Printf("    %s\n", k)
		}
	}

	// ── Work list ──
	fmt.Println("\n=== OBJECTS TO REPAIR (first 20) ===")
	if len(repair) == 0 {
		fmt.Println("  (none — every object already has the target Content-Type)")
	} else {
		for i, k := range repair {
			if i >= 20 {
				fmt.Printf("  … and %d more\n", len(repair)-20)
				break
			}
			fmt.Printf("  %s\n", k)
		}
	}

	// ── The classification list: every '?' with the field and value ──
	// Purpose: decide, object by object, whether the '?' is punctuation
	// (legitimate) or a substituted character (damage). One object per
	// block — filename first, then each field carrying a '?' with its
	// decoded value, so the operator judges from the text itself.
	if len(qEntries) > 0 {
		fmt.Printf("\n=== '?' FIELD REPORT (%d object(s)) ===\n", len(qEntries))
		fmt.Println("  Each block: the object key, then one line per field carrying a")
		fmt.Println("  '?' with its decoded value (what the catalog sees).")
		fmt.Println("  Legitimate: the '?' belongs to the wording. Damage: a '?' where")
		fmt.Println("  an apostrophe, accent, or dash belongs.")
		fmt.Println()
		for _, e := range qEntries {
			fmt.Printf("  %s\n", e.Key)
			for _, f := range e.Fields {
				fmt.Printf("      %-10s %q\n", f.Name, f.Value)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("\n  No '?' found in any metadata value.")
	}

	return toFix
}

// exposure reports whether an object's metadata carries the value shapes
// that historically triggered CopyObject encoded-word corruption.
// Values are decoded first: the question is what the CATALOG would see.
func exposure(meta map[string]string) (nonASCII bool, question bool) {
	for _, raw := range meta {
		d := metadata.Decode(raw)
		if strings.ContainsRune(d, '?') {
			question = true
		}
		for _, r := range d {
			if r > 127 {
				nonASCII = true
				break
			}
		}
		if nonASCII && question {
			return
		}
	}
	return
}

func applyAll(ctx context.Context, client *storage.Client, keys []string, conc int, want string) (fixed, already int, failed []string) {
	var (
		mu    sync.Mutex
		done  int64
		total = len(keys)
	)

	jobs.RunParallel(keys, conc,
		func(k string) error {
			changed, err := client.FixContentType(ctx, k, want)
			n := atomic.AddInt64(&done, 1)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			if changed {
				fixed++
				log.Printf("[%d/%d] FIXED  %s", n, total, k)
			} else {
				already++
				log.Printf("[%d/%d] SKIP   %s (already %s)", n, total, k, want)
			}
			return nil
		},
		func(string) {},
		func(k string, err error) {
			mu.Lock()
			failed = append(failed, k)
			mu.Unlock()
			log.Printf("FAILED  %s: %v", k, err)
		},
	)
	return
}

// smokeTest runs one object end to end and reports the ETag before and
// after — the one measurement that answers whether a catalog rebuild is
// required afterwards. Copying identical content should preserve the
// MD5 ETag, but this proves it on a real object instead of assuming it.
func smokeTest(ctx context.Context, client *storage.Client, key, want string, apply bool) {
	before, err := client.GetMetadata(ctx, key)
	if err != nil {
		log.Fatalf("HEAD %s: %v", key, err)
	}
	ctBefore := "(none)"
	if before.ContentType != nil && *before.ContentType != "" {
		ctBefore = *before.ContentType
	}
	na, q := exposure(before.Metadata)

	fmt.Printf("\n=== SMOKE TEST: %s ===\n", key)
	fmt.Printf("  Content-Type before:  %s\n", ctBefore)
	fmt.Printf("  Non-ASCII values:     %v\n", na)
	fmt.Printf("  '?' in a value:       %v\n", q)
	if qf := questionFields(before.Metadata); len(qf) > 0 {
		names := make([]string, 0, len(qf))
		for _, f := range qf {
			names = append(names, fmt.Sprintf("%s=%q", f.Name, f.Value))
		}
		fmt.Printf("  '?' fields:           %s\n", strings.Join(names, ", "))
	}
	fmt.Printf("  ETag before:          %s\n", aws.ToString(before.ETag))
	fmt.Printf("  Metadata fields:      %d\n", len(before.Metadata))
	printMeta(before.Metadata)

	if !apply {
		fmt.Println("\n  Dry run — nothing written. Add -apply to perform this one object.")
		return
	}

	changed, err := client.FixContentType(ctx, key, want)
	if err != nil {
		log.Fatalf("fix %s: %v", key, err)
	}

	after, err := client.GetMetadata(ctx, key)
	if err != nil {
		log.Fatalf("verification HEAD %s: %v", key, err)
	}
	ctAfter := "(none)"
	if after.ContentType != nil && *after.ContentType != "" {
		ctAfter = *after.ContentType
	}

	fmt.Printf("  Changed:              %v\n", changed)
	fmt.Printf("  Content-Type after:   %s\n", ctAfter)
	fmt.Printf("  ETag after:           %s\n", aws.ToString(after.ETag))
	fmt.Printf("  Metadata fields:      %d\n", len(after.Metadata))
	printMeta(after.Metadata)

	// Decisive checks: decoded values must be identical, and any change
	// to the RAW transit form is reported — that is how a '?' → '=3F'
	// re-encoding becomes visible, since it leaves the field count and
	// the ETag untouched.
	rawChanged := 0
	for k, rawBefore := range before.Metadata {
		if rawAfter, ok := after.Metadata[k]; ok && rawAfter != rawBefore {
			rawChanged++
			fmt.Printf("    raw %s: %q → %q\n", k, rawBefore, rawAfter)
		}
	}
	if rawChanged == 0 {
		fmt.Println("  Raw transit forms:    unchanged")
	} else {
		fmt.Printf("  Raw transit forms:    %d changed (decoded values above are what the catalog sees)\n", rawChanged)
	}

	if aws.ToString(before.ETag) == aws.ToString(after.ETag) {
		fmt.Println("\n  ✅ ETag unchanged — the catalog's hash values remain valid; no rebuild required.")
	} else {
		fmt.Println("\n  ⚠️  ETag CHANGED — the catalog's hash values are now stale. Regenerate the catalog")
		fmt.Println("      (start the server, or POST /api/start-crawl) before the compliance sweep.")
	}
}

// printMeta prints decoded metadata, sorted, one field per line. Decoded
// because that is what the catalog sees; the raw transit form is shown
// separately when it changes.
func printMeta(meta map[string]string) {
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("    %-12s %q\n", k, metadata.Decode(meta[k]))
	}
}

// qField is one metadata field whose DECODED value contains a '?'.
type qField struct {
	Name  string // metadata key, e.g. "title"
	Value string // decoded value — what the catalog sees
}

// questionFields returns every field whose decoded value contains a '?',
// sorted by field name. Decoded, because at the byte level a legitimate
// question mark and a substituted character are identical — the operator
// classifies them by reading the value, which is why it is carried here.
func questionFields(meta map[string]string) []qField {
	out := make([]qField, 0, len(meta))
	for k, raw := range meta {
		d := metadata.Decode(raw)
		if strings.ContainsRune(d, '?') {
			out = append(out, qField{Name: k, Value: d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
