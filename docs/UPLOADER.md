# Bulk Upload Tool: Architecture & Workflow

## 1. Overview
The **Bulk Upload Tool** is a high-performance desktop CLI application built in Go. It is designed for administrative bulk ingestion of audio assets into the R2 storage bucket. By operating as a native desktop tool, it bypasses browser memory constraints and timeout limits, allowing for maximum network throughput and operational resilience.

## 2. Core Design Decisions

### A. Schema-Driven CSV Ingestion
The tool ingests comma-delimited files (CSV) exported from Excel, Google Sheets, or external CMS systems. To accommodate varied export formats, the ingestion engine uses a **Flexible Mapping Architecture** rather than rigid struct tags.
*   **Header Discovery over Column Index:** The tool reads the first row as headers and maps them to a dynamic `map[string]int` (Header -> Column Index). Administrators can rearrange columns in their spreadsheet without breaking the tool.
*   **Mandatory vs. Optional Fields:** The only mandatory column header is `filename`. All other fields are considered optional.
*   **Graceful Ignore (Spurious Columns):** If the CSV contains columns *not* defined in the `CatalogSchema` (e.g., internal CMS notes), the tool ignores them. They will not cause errors and will not be uploaded to R2.

### B. Security & Performance: Presigned URLs
To achieve maximum upload speed while maintaining strict security, the tool utilizes **Presigned URLs**.
*   **Zero Client-Side R2 Credentials:** The desktop tool does not possess, nor does it require, the top-secret R2 API keys. It communicates solely via standard HTTP with the Go web server.
*   **Direct-to-Edge Streaming:** The server issues a time-limited, cryptographically signed "Presigned PUT URL" for a specific filename. The desktop tool streams the heavy MP3 bytes directly to the Cloudflare R2 edge, bypassing the web server entirely. This maximizes bandwidth and prevents the server from becoming a bottleneck.

### C. Consistency: The Crawler-as-API Pattern
For single-file browser uploads, the server uses a "Hot Patch" technique to instantly update `catalog.json.gz`. However, for bulk uploads involving dozens or hundreds of files, Hot Patching is inefficient (it would require downloading/re-uploading the catalog JSON hundreds of times).
*   **Atomic Rebuild:** Instead of patching, the Bulk Tool defers catalog consistency to the **Crawler**. Once all MP3 files and their metadata are secured in R2, the tool triggers an asynchronous Crawler Job on the server.
*   **Source of Truth:** The Crawler scans the R2 bucket, generating a fresh, perfectly accurate `catalog.json.gz`. The Go web server's RAM cache automatically detects the new ETag on the next request, ensuring instant state consistency for all frontend users.

### D. Asynchronous Job Queue
Because cataloging thousands of files can take time, the server employs an asynchronous **`202 Accepted`** pattern.
*   **Non-Blocking:** When the Bulk Tool triggers a crawl, the server immediately returns a `JobID`. The tool then polls the server for progress, preventing HTTP timeouts and allowing the user to monitor the rebuild status in real-time.

## 3. Operational Pipeline

The tool executes a strict, sequential pipeline to ensure data integrity before any irreversible network operations occur.

### Phase 1: Ingest & Diff
1.  **Parse CSV:** The tool ingests the CSV manifest using the Schema-Driven parser.
2.  **Fetch Current Catalog:** The tool downloads the current `catalog.json.gz` from the server and decompresses it.
3.  **Diffing Engine:** It compares the CSV manifest against the existing catalog to determine the required action for each item:
    *   **UPLOAD:** File exists in CSV but not in R2.
    *   **UPDATE:** File exists in both, but CSV contains differing metadata.
    *   **SKIP:** File and metadata are identical.

### Phase 2: Filesystem Verification
The user specifies a local root directory containing the audio files.
*   **Pre-flight Check:** The tool walks the local directory and cross-references it against the "UPLOAD/UPDATE" lists using `os.Stat()`.
*   **Residual Changes:** If a file specified in the CSV is missing locally, it is logged to a `residual_changes.csv` file. The tool proceeds only with the verified files, preventing premature catalog entries for missing audio.

### Phase 3: Direct-to-R2 Upload & Metadata Sync
For every verified file in the Actionable List:
1.  **Get Presigned URL:** Tool requests a PUT URL from the server.
2.  **Stream to R2:** Tool streams the MP3 bytes directly to the Cloudflare Edge. It explicitly sets `Content-Length` and `Content-Type: audio/mpeg` to ensure R2 accepts the payload.
3.  **Apply Metadata:** Tool sends a POST request to the server's `/api/update-metadata` endpoint. The server performs a **Copy-Over-Self** operation on the R2 object, injecting the CSV metadata into the R2 object's HTTP headers so the Crawler can read them later.

### Phase 4: Asynchronous Catalog Rebuild
Once the upload queue reaches 100% success:
1.  **Trigger Crawl:** The tool calls `POST /api/start-crawl`.
2.  **Server Worker:** The server spawns a background Goroutine to run `CrawlCatalog()`, returning a `JobID`.
3.  **Polling:** The tool polls `GET /api/crawl-status?job_id=...` every second, displaying progress.
4.  **Finalization:** The Crawler generates `catalog.json.gz`, pushes it to R2, and the server clears its RAM cache.

## 4. Server API Endpoints Utilized

The desktop tool relies on the following existing web server endpoints:

| Method | Endpoint | Purpose |
| :--- | :--- | :--- |
| `GET` | `/api/catalog` | Fetches the current catalog JSON for diffing. |
| `GET` | `/api/get-upload-url?filename=` | Retrieves a time-limited Presigned PUT URL. |
| `POST` | `/api/update-metadata` | Triggers R2 Copy-Over-Self to inject ID3 headers. |
| `POST` | `/api/start-crawl` | Initiates the asynchronous catalog rebuild job. |
| `GET` | `/api/crawl-status?job_id=` | Polls the status/progress of the rebuild job. |

## 5. Future Enhancements / TODO List

The following features have been identified as high-value upgrades to further improve the tool's resilience and storage efficiency:

*   **[ ] Content Fingerprinting (Duplicate Detection):**
    *   **Objective:** Prevent the storage of duplicate audio content under different filenames.
    *   **Implementation:** Compute MD5 hashes of local audio files during the pre-flight verification stage. Compare these against the `hash` values already present in the `catalog.json.gz` manifest.
    *   **Benefit:** Significant bandwidth and R2 storage savings.

*   **[ ] Resume-on-Interrupt:**
    *   **Objective:** Allow the tool to recover instantly if the network drops or the tool is closed during a large batch.
    *   **Implementation:** Maintain a local SQLite or JSON 'state' file that tracks `pending`, `uploaded`, and `metadata-synced` states for each record in the current batch.

*   **[ ] Concurrent Upload Schedulers:**
    *   **Objective:** Increase throughput for very large libraries.
    *   **Implementation:** Implement a Go worker pool using `channels` and `sync.WaitGroup` to perform 3-5 concurrent uploads rather than the current sequential upload process.

*   **[ ] Automatic ID3 Tag Correction:**
    *   **Objective:** Automatically propagate local ID3 tag changes discovered during the crawler phase back to the CSV manifest.
    *   **Implementation:** Create a "Sync-Back" routine that exports the R2-derived metadata from the crawler into an updated local CSV.


The following features have been identified as high-value upgrades to further improve the tool's resilience and storage efficiency:

*   **[ ] Standardized Audio Fingerprinting (Content-Based Deduplication):**
    *   **Objective:** Detect duplicate audio content regardless of filename changes or ID3 metadata edits.
    *   **Design Decision:** We will implement an "Audio-Only" MD5 hash. Since ID3 tags are stored inside the MP3 container but *outside* the actual audio frames, we must strip the headers before hashing.
    *   **Implementation Strategy:**
        1.  **Shared Algorithm:** Use a "Jump-to-Audio" algorithm. Both Go (in the Bulk Tool/Server) and Svelte (in the Browser) will:
            *   Inspect the file header for `ID3` bytes.
            *   Calculate the size of the ID3 tag (using synchsafe integer logic).
            *   Seek past the tag header.
            *   Compute the MD5 hash of the remaining byte stream.
        2.  **Storage:** Store the resulting hash as a permanent R2 object header: `x-amz-meta-audio-hash`.
        3.  **Cross-Platform Consistency:** By following the same byte-seeking logic in both Go and JS, the hash will be identical regardless of whether it was calculated on the client's PC or the web browser.
    *   **Benefit:** Enables instant duplicate detection, prevents redundant bandwidth usage, and ensures data integrity even if files are renamed or re-tagged.

*   **[ ] Resume-on-Interrupt:**
    *   **Objective:** Allow the tool to recover instantly if the network drops or the tool is closed during a large batch.
    *   **Implementation:** Maintain a local SQLite or JSON 'state' file.
    *   **Benefit:** Eliminates the need to restart large batches from file #1.

*   **[ ] Concurrent Upload Schedulers:**
    *   **Objective:** Increase throughput for high-volume libraries.
    *   **Implementation:** Utilize Go `channels` and `sync.WaitGroup` to handle 3–5 concurrent uploads per batch.
    *   **Benefit:** Dramatically reduces the time required for massive library migrations.

*   **[ ] Automatic ID3 Tag Sync:**
    *   **Objective:** Propagate local ID3 tag changes discovered during the Crawler phase back to the source CSV manifest.
    *   **Benefit:** Simplifies the "cycle of truth" between the local CMS (spreadsheet) and the R2 cloud storage.

