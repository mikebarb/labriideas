// Command r2-encoding-audit performs a READ-ONLY conformance audit of the
// R2 bucket: every object key and every metadata value is classified for
// UTF-8 validity, Unicode normalisation form (NFC/NFD), RFC 2047 encoded-
// word artefacts, double encoding, and size budget.
//
// It writes nothing to R2. Its outputs are:
//   - a console summary (the headline counts), and
//   - a JSON report on disk, so the audit can be re-run after the repair
//     work and the two reports diffed.
//
// Credential model: direct bucket credentials from the developer .env
// (R2_* variables, same names as the server). This tool is developer-only
// and is never shipped to administrators — listing requires direct
// access, which presigned URLs cannot provide.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/audit"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

// manifestKeys are the bucket's own control files — metadata-free, not
// part of the library, and skipped exactly as CrawlCatalog skips them.
var manifestKeys = map[string]bool{
	"catalog.json":    true,
	"catalog.json.gz": true,
	"version.json":    true,
}

func main() {
	// The tool owns .env loading (see storage.NewClientFromEnv's comment).
	_ = godotenv.Load()

	logPath := fmt.Sprintf("r2-encoding-audit-%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	fmt.Println("=== R2 Encoding & Normalisation Audit ===")
	fmt.Println("READ-ONLY: this tool writes nothing to R2.")

	ctx := context.Background()
	client, err := storage.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatalf("Setup failed: %v", err)
	}
	fmt.Printf("Bucket: %s\n", client.Bucket())

	// 1. List every key (all object types — unexpected keys are findings).
	keys, err := client.ListObjectKeys(ctx)
	if err != nil {
		log.Fatalf("Listing bucket failed: %v", err)
	}
	fmt.Printf("Found %d objects.\n", len(keys))

	// 2. HEAD each object — headers only, no bodies. On a 2K-object
	//    bucket this is a few hundred KB of headers total.
	var reports []audit.ObjectReport
	headErrors := 0
	processed := 0
	for _, key := range keys {
		if manifestKeys[key] {
			continue
		}
		processed++

		head, err := client.GetMetadata(ctx, key)
		if err != nil {
			// A HEAD failure is itself reported as an object with an
			// issue, rather than aborting the audit — one unreadable
			// object must not hide the state of the other 2,000.
			headErrors++
			reports = append(reports, audit.ObjectReport{
				Key:    key,
				Issues: []string{fmt.Sprintf("HEAD failed: %v", err)},
			})
			continue
		}

		rep := audit.AnalyseMetadata(key, head.Metadata)
		if head.ContentType != nil {
			rep.ContentType = *head.ContentType
		}
		// Content-Type anomaly: the metadata rewrite path has never
		// preserved ContentType, so objects edited in the past may have
		// lost audio/mpeg. Reported here; repair comes with the refactor.
		if rep.ContentType != "" && rep.ContentType != "audio/mpeg" &&
			len(key) > 4 && key[len(key)-4:] == ".mp3" {
			rep.Issues = append(rep.Issues, fmt.Sprintf("unexpected Content-Type on .mp3: %q", rep.ContentType))
		}
		reports = append(reports, rep)

		if processed%50 == 0 {
			fmt.Printf("  audited %d/%d...\n", processed, len(keys))
		}
	}

	// 3. Summarise and print the headline numbers.
	sum := audit.Summarize(reports, headErrors)
	fmt.Printf("Objects scanned:        %d\n", sum.ObjectsScanned)
	fmt.Printf("Objects with issues:    %d\n", sum.ObjectsWithIssues)
	fmt.Printf("HEAD errors:            %d\n", sum.HeadErrors)
	fmt.Printf("Encoded-word values:    %d  (corruption artefacts)\n", sum.EncodedWordValues)
	fmt.Printf("Double-encoded values:  %d  (one decode pass did not clean them)\n", sum.DoubleEncodedVals)
	fmt.Printf("Invalid UTF-8 values:   %d\n", sum.InvalidUTF8Values)
	fmt.Printf("Non-ASCII keys:         %d\n", sum.NonASCIIKeys)
	fmt.Printf("Oversized keys:         %d\n", sum.OversizedKeys)
	fmt.Printf("Over metadata budget:   %d  (budget %d bytes)\n", sum.OverMetaBudget, sum.MetaBudgetBytes)
	fmt.Printf("Key forms:              %v\n", sum.KeyForms)
	fmt.Printf("Value forms (flagged):  %v\n", sum.ValueForms)
	fmt.Println("\nNOTE: " + sum.CauseCaveat)

	// 4. Detailed listing — only objects with issues, to keep the file
	//    readable; the summary already counts the clean population.
	detailed := make([]audit.ObjectReport, 0, sum.ObjectsWithIssues)
	for _, rep := range reports {
		if len(rep.Issues) > 0 || len(rep.ValueFindings) > 0 {
			detailed = append(detailed, rep)
		}
	}
	out := struct {
		Summary  audit.Summary        `json:"summary"`
		Findings []audit.ObjectReport `json:"findings"`
	}{sum, detailed}

	reportPath := fmt.Sprintf("r2-encoding-audit-%s.json", time.Now().Format("2006-01-02_15-04-05"))
	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		log.Fatalf("Could not write report: %v", err)
	}

	fmt.Printf("\n✅ Detailed report: %s\n", reportPath)
	fmt.Printf("   Log file:       %s\n", logPath)
}
