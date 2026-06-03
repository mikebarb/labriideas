package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mikebarb/labriideas-publisher/pkg/csvparser"
)

// --- CONFIGURATION ---
const serverBaseURL = "http://localhost:8080"

func main() {
	fmt.Println("=== Labriideas Bulk Upload Tool ===")
	fmt.Println("Initializing...")

	csvPath := "./enriched_catalog_short.csv"
	audioDirPath := `C:\Users\Barbara\labri_audio_files`

	// ==========================================
	// STEP 1: Parse CSV & Verify Local Files
	// ==========================================
	file, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("Error opening CSV: %v", err)
	}
	defer file.Close()

	records, err := csvparser.Parse(file)
	if err != nil {
		log.Fatalf("Error parsing CSV: %v", err)
	}
	fmt.Printf("\nSuccessfully parsed %d records from CSV.\n", len(records))

	var readyToProcess []csvparser.ParsedRecord
	var missingFiles []string

	for _, rec := range records {
		fullPath := filepath.Join(audioDirPath, rec.Filename)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, rec.Filename)
		} else {
			readyToProcess = append(readyToProcess, rec)
		}
	}

	if len(missingFiles) > 0 {
		fmt.Printf("\n⚠️ WARNING: %d files missing from disk:\n", len(missingFiles))
		for _, f := range missingFiles {
			fmt.Printf("  - %s\n", f)
		}
	}
	fmt.Printf("✅ %d files verified on disk.\n", len(readyToProcess))

	// ==========================================
	// STEP 2: Fetch Current Catalog from Server (NEW)
	// ==========================================
	fmt.Println("\n--- Fetching current catalog from server ---")
	r2Catalog, err := fetchCatalogFromServer()
	if err != nil {
		log.Fatalf("Failed to fetch catalog: %v", err)
	}
	fmt.Printf("✅ Loaded %d existing tracks from server.\n", len(r2Catalog))

	// ==========================================
	// STEP 3: The Diffing Engine
	// ==========================================
	fmt.Println("\n--- Running Diffing Engine ---")

	var listUpload []csvparser.ParsedRecord // Not in R2
	var listUpdate []csvparser.ParsedRecord // In R2, but metadata differs
	var listSkip []csvparser.ParsedRecord   // In R2 and identical

	for _, localRec := range readyToProcess {
		r2Track, existsInR2 := r2Catalog[localRec.Filename]

		if !existsInR2 {
			listUpload = append(listUpload, localRec)
		} else {
			if metadataMatches(localRec.Metadata, r2Track) {
				listSkip = append(listSkip, localRec)
			} else {
				listUpdate = append(listUpdate, localRec)
			}
		}
	}

	fmt.Println("\n=== DIFFING RESULTS ===")
	fmt.Printf("🟢 NEW (To Upload):    %d files\n", len(listUpload))
	fmt.Printf("🟡 CHANGED (To Update): %d files\n", len(listUpdate))
	fmt.Printf("⚪ UNCHANGED (Skip):    %d files\n", len(listSkip))

	// ==========================================
	// STEP 4: Execute Uploads & Updates via Server APIs
	// ==========================================
	successCount := 0

	// 4A: Upload New Files
	if len(listUpload) > 0 {
		fmt.Println("\n--- Uploading New Files to R2 ---")
		for _, rec := range listUpload {
			err := uploadViaPresignedURL(rec, audioDirPath)
			if err != nil {
				fmt.Printf("  ❌ Upload Failed: %s (%v)\n", rec.Filename, err)
				continue
			}
			successCount++
		}
	}

	// 4B: Update Existing Metadata (No file upload needed, just API call)
	if len(listUpdate) > 0 {
		fmt.Println("\n--- Updating Metadata on Existing Files ---")
		for _, rec := range listUpdate {
			err := updateMetadataViaServer(rec)
			if err != nil {
				fmt.Printf("  ❌ Metadata Update Failed: %s (%v)\n", rec.Filename, err)
				continue
			}
			successCount++
		}
	}

	if successCount == 0 {
		fmt.Println("\nNo changes made. Exiting.")
		return
	}

	// ==========================================
	// STEP 5: Trigger Asynchronous Crawl on Server (NEW)
	// ==========================================
	fmt.Println("\n--- Triggering Catalog Rebuild on Server ---")
	jobID, err := triggerCrawlOnServer()
	if err != nil {
		log.Fatalf("Failed to trigger crawl: %v", err)
	}

	// ==========================================
	// STEP 6: Poll Crawl Status until Complete (NEW)
	// ==========================================
	fmt.Printf("Crawl Job Started (ID: %s). Waiting for server to rebuild catalog...\n", jobID)
	pollCrawlStatus(jobID)

	fmt.Println("\n=== Bulk Upload Complete! ===")
}

// ==========================================
// HELPER FUNCTIONS
// ==========================================

// Fetches the catalog JSON directly from the web server's cache/R2
func fetchCatalogFromServer() (map[string]map[string]string, error) {
	emptyCatalog := make(map[string]map[string]string)
	resp, err := http.Get(serverBaseURL + "/api/catalog")
	if err != nil {
		return emptyCatalog, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return emptyCatalog, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	// 1. Read the raw bytes from the response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return emptyCatalog, fmt.Errorf("failed to read response body: %w", err)
	}

	// 2. Check if the response is Gzipped (GZIP magic number is 0x1f 0x8b)
	if len(bodyBytes) >= 2 && bodyBytes[0] == 0x1f && bodyBytes[1] == 0x8b {
		gzReader, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		if err != nil {
			return emptyCatalog, fmt.Errorf("failed to init gzip reader: %w", err)
		}
		defer gzReader.Close()

		// Decompress into a new byte slice
		bodyBytes, err = io.ReadAll(gzReader)
		if err != nil {
			return emptyCatalog, fmt.Errorf("failed to decompress gzip: %w", err)
		}
	}

	// 3. Now parse the uncompressed JSON bytes
	var catalogData struct {
		Tracks []map[string]interface{} `json:"tracks"`
	}
	if err := json.Unmarshal(bodyBytes, &catalogData); err != nil {
		return emptyCatalog, fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	// 4. Convert to our lookup map
	catalogMap := make(map[string]map[string]string)
	for _, track := range catalogData.Tracks {
		if filename, ok := track["filename"].(string); ok {
			metaMap := make(map[string]string)
			for k, v := range track {
				if k == "id" || k == "filename" || k == "hash" {
					continue
				}
				metaMap[k] = fmt.Sprintf("%v", v)
			}
			catalogMap[filename] = metaMap
		}
	}
	return catalogMap, nil
}

func metadataMatches(csvMeta map[string]string, r2Meta map[string]string) bool {
	for k, csvVal := range csvMeta {
		r2Val, exists := r2Meta[k]
		if !exists {
			return false
		}
		if strings.TrimSpace(strings.ToLower(csvVal)) != strings.TrimSpace(strings.ToLower(r2Val)) {
			return false
		}
	}
	return true
}

// 1. Gets Presigned URL from Server -> 2. PUTs file directly to R2 -> 3. Updates Metadata via Server
func uploadViaPresignedURL(rec csvparser.ParsedRecord, audioDirPath string) error {
	encodedFilename := url.QueryEscape(rec.Filename)

	// 1. Get Presigned Upload URL
	resp, err := http.Get(serverBaseURL + "/api/get-upload-url?filename=" + encodedFilename)
	if err != nil {
		return err
	}
	var urlResp struct {
		URL string `json:"url"`
	}
	json.NewDecoder(resp.Body).Decode(&urlResp)
	resp.Body.Close()

	if urlResp.URL == "" {
		return fmt.Errorf("empty presigned url received")
	}

	// 2. Open local file and PUT directly to R2
	filePath := filepath.Join(audioDirPath, rec.Filename)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 3. Get File Stats (Crucial for R2 to know the Content-Length)
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// 4. Create the PUT request
	req, err := http.NewRequest("PUT", urlResp.URL, file)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "audio/mpeg")

	// FORCE Go to tell R2 exactly how big the file is (prevents chunked encoding)
	req.ContentLength = stat.Size()

	// 5. Execute the Upload
	client := &http.Client{}
	r2Resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("network error on upload: %w", err)
	}
	defer r2Resp.Body.Close()

	// 6. CRITICAL: Did R2 actually accept it?
	if r2Resp.StatusCode != 200 {
		// Read R2's error message so we know WHY it failed
		bodyBytes, _ := io.ReadAll(r2Resp.Body)
		return fmt.Errorf("R2 rejected upload (Status %d): %s", r2Resp.StatusCode, string(bodyBytes))
	}

	// 7. File is safely in R2, now attach the metadata
	err = updateMetadataViaServer(rec)
	if err != nil {
		return fmt.Errorf("file uploaded but metadata failed: %w", err)
	}

	fmt.Printf("  ✅ Uploaded: %s\n", rec.Filename)
	return nil
}

// Tells the server to perform the R2 Copy-Over-Self to update metadata tags
func updateMetadataViaServer(rec csvparser.ParsedRecord) error {
	payload := map[string]interface{}{
		"filename": rec.Filename,
		"metadata": rec.Metadata,
	}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(serverBaseURL+"/api/update-metadata", "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// Triggers the background Crawler on the server and returns the Job ID
func triggerCrawlOnServer() (string, error) {
	resp, err := http.Post(serverBaseURL+"/api/start-crawl", "application/json", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var jobResp struct {
		JobID string `json:"job_id"`
	}
	json.NewDecoder(resp.Body).Decode(&jobResp)
	return jobResp.JobID, nil
}

// Polls the server's job status endpoint every second until complete
func pollCrawlStatus(jobID string) {
	for {
		resp, err := http.Get(fmt.Sprintf("%s/api/crawl-status?job_id=%s", serverBaseURL, jobID))
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		var status struct {
			Status   string `json:"status"`
			Progress int    `json:"progress"`
			Message  string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()

		fmt.Printf("\r  Server Progress: %d%% - %s", status.Progress, status.Message)

		if status.Status == "completed" {
			fmt.Println("\n✅ Catalog rebuild finished on server.")
			return
		} else if status.Status == "failed" {
			log.Fatalf("\n❌ Crawl failed on server: %s", status.Message)
		}

		time.Sleep(1 * time.Second)
	}
}
