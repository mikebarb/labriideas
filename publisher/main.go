package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/cache"
	"github.com/mikebarb/labriideas-publisher/pkg/catalog"
	"github.com/mikebarb/labriideas-publisher/pkg/schema"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

// Global storage and cache singletons
var storageClient *storage.Client
var catalogCache *cache.CatalogCache

// The shared catalog store: ONE implementation of the catalog
// read-modify-write lifecycle, used by the update, delete and upload
// handlers (previously three near-identical copies lived inline).
var catalogStore *catalog.Store

// Timestamp of the last successful menu.json commit — the baseline
// deploy-status polling compares against, so a SUCCESSFUL OLD build
// (from before this admin's commit) never reads as "deployed".
var lastMenuCommitAt time.Time
var lastMenuCommitMu sync.Mutex

// In-memory store for background crawl tasks
var jobRegistry sync.Map

// Pre-cached schema bytes served to the frontend editor
var menuSchemaBytes []byte

// Auto-crawl guarding. crawlActive is the single-flight lock: many
// simultaneous failures coalesce into ONE crawl. lastAutoCrawl is the
// cooldown: a crawl that FAILED to produce the catalog must not be
// immediately re-triggered by every subsequent request.
var crawlMu sync.Mutex
var crawlActive bool
var lastAutoCrawl time.Time

// autoCrawlCooldown bounds how often a missing catalog can trigger a
// regeneration, so a persistently broken environment cannot spawn an
// endless sequence of crawls.
const autoCrawlCooldown = 10 * time.Minute

// Request Payloads
type MetadataUpdateRequest struct {
	Filename string            `json:"filename"`
	Metadata map[string]string `json:"metadata"`
}

// UploadTrackRequest defines the metadata payload sent alongside the file
type UploadTrackRequest struct {
	Filename string            `json:"filename"`
	Metadata map[string]string `json:"metadata"`
}

// maybeTriggerRegeneration starts an asynchronous catalog regeneration
// if none is running AND the cooldown has elapsed. It is the recovery
// path for a MISSING catalog: the catalog is a derivable artefact, so
// one regeneration restores both the hot-patch surface and the public
// /api/catalog read path. Returns the job ID, or "" when suppressed.
//
// LOCK DISCIPLINE: crawlMu is acquired exactly ONCE and released by a
// single deferred Unlock. Do NOT add an explicit Unlock anywhere in this
// function — mixing defer with a manual Unlock causes "sync: unlock of
// unlocked mutex", a fatal error that cannot be recovered. The spawned
// goroutine takes its OWN lock when it clears the flag; that is a
// separate acquisition, not a release of this one.
func maybeTriggerRegeneration(reason string) string {
	crawlMu.Lock()
	defer crawlMu.Unlock()

	if crawlActive {
		log.Printf("Regeneration already in progress; not starting another (%s)", reason)
		return ""
	}
	if !lastAutoCrawl.IsZero() && time.Since(lastAutoCrawl) < autoCrawlCooldown {
		log.Printf("Regeneration cooldown active (%.0f minute window); not starting another (%s)",
			autoCrawlCooldown.Minutes(), reason)
		return ""
	}
	crawlActive = true

	jobID := fmt.Sprintf("crawl-%d", time.Now().UnixNano())
	jobRegistry.Store(jobID, &CrawlJob{
		ID:       jobID,
		Status:   "running",
		Progress: 0,
		Message:  "Automatic regeneration: " + reason,
	})

	// The goroutine re-acquires crawlMu to clear the flag when the crawl
	// finishes — on success OR failure, because a failed crawl has still
	// "just run" and must not be immediately retried.
	go func() {
		defer func() {
			crawlMu.Lock()
			crawlActive = false
			lastAutoCrawl = time.Now()
			crawlMu.Unlock()
		}()
		startCrawlWorker(jobID)
	}()

	log.Printf("[ADMIN] Automatic catalog regeneration triggered (%s): job %s", reason, jobID)
	return jobID
}

// CrawlJob represents the state of an asynchronous crawl task
type CrawlJob struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // "running", "completed", "failed"
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

// Root-level validation contract derived from the schema — populated
// at startup, used by updateMenuHandler's structural gate. Single
// source of truth: the required root keys come from the schema's
// "required" array, so schema revisions automatically update the
// deploy gate with no Go code changes.
var menuSchemaRequiredKeys []string

func init() {
	var err error
	// Read and verify menu JSON schema at application startup
	menuSchemaBytes, err = os.ReadFile("pkg/schema/menu.schema.json")
	if err != nil {
		log.Printf("⚠️  WARNING: Could not load pkg/schema/menu.schema.json: %v", err)
		return
	}

	// Parse the schema's TOP-LEVEL contract only — we are not a full
	// JSON Schema validator (the browser editor already does that via
	// codemirror-json-schema). We extract the root "required" keys so
	// the deploy gate enforces exactly what the schema declares,
	// instead of a hard-coded key list that can drift from the schema.
	var schemaDoc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(menuSchemaBytes, &schemaDoc); err != nil {
		log.Printf("⚠️  WARNING: menu.schema.json is not valid JSON — root-key gate will use fallback: %v", err)
		return
	}
	menuSchemaRequiredKeys = schemaDoc.Required
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on system env vars")
	}
	// --- NEW: Initialize Auth system ---
	InitAuth()
	if len(adminPasswords()) == 0 {
		log.Println("⚠️  WARNING: ADMIN_PASSWORDS is empty — browser login will fail.")
	}
	if len(adminAPITokens()) == 0 {
		log.Println("⚠️  WARNING: ADMIN_API_TOKENS is empty — CLI tools will be rejected.")
	}

	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	accountID := os.Getenv("R2_ACCOUNT_ID")
	bucketName := os.Getenv("R2_BUCKET_NAME")

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 1. Get the Port from the environment (Render provides this)
	// If it's not set (like on your local machine), default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create the raw S3 Client
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + accountID + ".r2.cloudflarestorage.com")
	})

	// Initialize YOUR Library Client!
	storageClient = storage.NewClient(s3Client, bucketName)
	catalogCache = cache.NewCatalogCache() // Initialize empty cache

	// The shared catalog store. SetOnMissing installs the
	// auto-regeneration hook. The hook is a closure rather than the
	// function itself because maybeTriggerRegeneration returns the job
	// ID (useful to direct callers that want to log it), while the hook
	// contract is fire-and-report-nothing: Store.Load only needs the
	// side effect triggered.
	catalogStore = catalog.NewStore(storageClient, catalogCache)
	catalogStore.SetOnMissing(func(reason string) {
		maybeTriggerRegeneration(reason)
	})

	// Setup HTTP Routes
	mux := http.NewServeMux()

	// --- Public endpoints (no token required) ---
	mux.HandleFunc("/api/catalog", corsMiddleware(catalogHandler))
	mux.HandleFunc("/api/download", corsMiddleware(downloadHandler))
	mux.HandleFunc("/api/schema/menu", corsMiddleware(menuSchemaHandler))
	mux.HandleFunc("/api/schema/catalog", corsMiddleware(catalogSchemaHandler))

	// --- Auth endpoints (must be reachable without a token) ---
	mux.HandleFunc("/api/auth/login", corsMiddleware(loginHandler))
	mux.HandleFunc("/api/auth/logout", corsMiddleware(logoutHandler))
	mux.HandleFunc("/api/auth/status", corsMiddleware(authStatusHandler))

	// --- Admin endpoints (Protected: Bearer token or Cookie session required) ---
	// Middleware order: CORS runs first (handles headers/preflight), then Auth (gates access)
	mux.HandleFunc("/api/upload", corsMiddleware(authMiddleware(uploadHandler)))
	mux.HandleFunc("/api/upload-track", corsMiddleware(authMiddleware(uploadTrackHandler)))
	mux.HandleFunc("/api/update-metadata", corsMiddleware(authMiddleware(updateMetadataHandler)))
	mux.HandleFunc("/api/delete-track", corsMiddleware(authMiddleware(deleteTrackHandler)))
	mux.HandleFunc("/api/get-upload-url", corsMiddleware(authMiddleware(getSignedUploadURLHandler)))
	mux.HandleFunc("/api/start-crawl", corsMiddleware(authMiddleware(startCrawlHandler)))
	mux.HandleFunc("/api/crawl-status", corsMiddleware(authMiddleware(crawlStatusHandler)))
	mux.HandleFunc("/api/update-menu", corsMiddleware(authMiddleware(updateMenuHandler)))
	mux.HandleFunc("/api/deploy-status", corsMiddleware(authMiddleware(deployStatusHandler)))

	// BACKGROUND CACHE WARMUP
	go func() {
		log.Println("Warming up catalog cache...")
		ctx := context.Background()

		// 1. Get ETag from R2
		head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
		if err != nil {
			// A fresh environment (or deleted catalog) should self-heal
			// at boot rather than waiting for the first admin action —
			// this is arguably the most valuable trigger site, because
			// it covers "deployed to a new/empty bucket".
			var nf *types.NotFound
			if errors.As(err, &nf) {
				maybeTriggerRegeneration("startup warmup: catalog missing")
			} else {
				log.Printf("Cache warmup failed (metadata): %v", err)
			}
			return
		}
		r2ETag := *head.ETag

		// 2. Get compressed bytes from R2
		rawBytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			log.Printf("Cache warmup failed (download): %v", err)
			return
		}

		// 3. Validate the bytes before caching. A corrupt catalog (plain
		//    text, truncated, or bad gzip) would otherwise be cached and
		//    served until the first mutation detects the failure — the
		//    warmup is the only site that can catch it at boot.
		if err := validateCatalogBytes(rawBytes); err != nil {
			log.Printf("Catalog cache warmup failed (validation): %v", err)
			maybeTriggerRegeneration("startup warmup: catalog corrupt")
			return
		}

		// 4. Update RAM Cache
		catalogCache.Update(r2ETag, rawBytes)
		log.Println("✅ Catalog cache warmed up successfully!")
	}()

	// --- Statistics collector: periodic flush to R2, final flush on shutdown ---
	// NOTE: signal.NotifyContext CAPTURES SIGINT/SIGTERM — once registered,
	// Ctrl+C no longer terminates the process by default. The signal only
	// cancels statsCtx. Therefore main() must observe the shutdown and stop
	// the HTTP server itself (below), or the process hangs forever with the
	// port held open. This is the whole point of the select at the end.
	statsCtx, statsStop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer statsStop()

	// statsDone closes AFTER StartStatsLoop's final flush completes — main
	// waits on it so the server isn't killed mid-flush.
	//statsDone := make(chan struct{})
	//go func() {
	//	StartStatsLoop(statsCtx) // returns after final flush on shutdown
	//	close(statsDone)
	//}()

	// statsDone receives the final flush outcome (true = flushed, false =
	// requeued/failed) when StartStatsLoop returns on shutdown — main waits
	// on it so the server isn't torn down mid-flush, and uses the result
	// for an honest shutdown message.
	statsDone := make(chan bool, 1)
	go func() {
		ok := StartStatsLoop(statsCtx) // true if final flush succeeded
		statsDone <- ok
	}()

	// CHANGED: http.Server instead of http.ListenAndServe — Shutdown()
	// requires a server object; the package-level function can't be stopped.
	srv := &http.Server{Addr: ":" + port, Handler: mux}

	// Run the server in a goroutine so main can supervise both the server
	// and the shutdown signal.
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()

	log.Printf("🚀 Server running on http://localhost:%s", port)

	select {
	case err := <-serverErr:
		// The server failed on its own (port bind error, etc.) — fatal.
		// ErrServerClosed is the normal return from Shutdown() and is NOT
		// an error.
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listener failed: %v", err)
		}
	//case <-statsDone:
	// Signal received AND final stats flush complete. Now stop the
	// HTTP server: Shutdown closes the listener (main's select would
	// otherwise block on it) and waits up to 5s for in-flight requests
	// to finish before returning. Then main returns → process exits.
	//	log.Println("🛑 Shutting down (stats flushed)...")
	//	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//	defer cancel()
	//	if err := srv.Shutdown(shutdownCtx); err != nil {
	//		log.Printf("Forcing listener close: %v", err)
	//	}
	case flushed := <-statsDone:
		// Signal received AND final stats flush ATTEMPT completed. Now stop
		// the HTTP server: Shutdown closes the listener (main's select
		// would otherwise block on it) and waits up to 5s for in-flight
		// requests to finish before returning. Then main returns → exit.
		if flushed {
			log.Println("🛑 Shutting down (stats flushed cleanly)...")
		} else {
			log.Println("🛑 Shutting down (stats flush failed — events requeued, will be lost at exit)...")
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Forcing listener close: %v", err)
		}
	}
}

// =============================================================================
// HTTP HANDLERS
// =============================================================================

// --- MENU SCHEMA HANDLER ---
func menuSchemaHandler(w http.ResponseWriter, r *http.Request) {
	if len(menuSchemaBytes) == 0 {
		http.Error(w, "Menu schema not available on server", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(menuSchemaBytes)
}

// requiredMenuKeys returns the root keys the deploy gate enforces.
// Schema-derived when the schema loaded and declared a "required"
// array; hard-coded fallback when the schema file is missing or
// unreadable — the deploy path must never become ungated.
func requiredMenuKeys() []string {
	if len(menuSchemaRequiredKeys) > 0 {
		return menuSchemaRequiredKeys
	}
	// FALLBACK: mirrors the current schema's required roots. Only
	// engages if the schema failed to load at startup (deployment
	// misconfiguration); the startup warning makes that visible.
	return []string{"Topics", "featuredLectures", "Playlists", "schaefferCollection", "Contact L'Abri"}
}

// updateMenuHandler acts as the GitOps CI/CD orchestrator. It performs structural
// sanity checks on incoming menu data and commits it directly to the GitHub repository.
func updateMenuHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// CHANGED: read the RAW request body BEFORE decoding. The decoded map
	// is used for the structural validation gate below (key presence is
	// order-insensitive), but the COMMIT (step 2) now uses the raw bytes.
	// Rationale: unmarshal-then-marshal round-trips through a Go map,
	// which serializes keys ALPHABETICALLY — silently discarding the
	// admin's physical key ordering from the editor. The menu.json
	// consumers (MegaMenu, TopicsTree) render sections in file order, so
	// section reordering in the editor only survives deploy if the commit
	// preserves the original byte order. json.Indent (step 2) reformats
	// whitespace only and never re-sorts keys.
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Decode into a map for VALIDATION ONLY. Key order is lost here by
	// design — the gate only checks that required root keys exist, and
	// key-presence is identical regardless of ordering.
	var menuData map[string]interface{}
	if err := json.Unmarshal(rawBody, &menuData); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// ─── STRUCTURAL VALIDATION GATE (SCHEMA-DRIVEN) ───
	// Required root keys are read from menu.schema.json at startup —
	// the same file served to the browser editor via
	// menuSchemaHandler. Revising the schema (adding a section,
	// renaming a key) automatically revises this gate; the two can no
	// longer drift apart. Deep validation (nested objects, arrays,
	// item fields) remains the editor's job client-side via
	// codemirror-json-schema; this gate enforces only the root-level
	// contract before the payload is committed to GitHub.
	for _, key := range requiredMenuKeys() {
		if _, exists := menuData[key]; !exists {
			http.Error(w, fmt.Sprintf("Schema violation: missing root key '%s'", key), http.StatusBadRequest)
			return
		}
	}

	// ─── GITHUB API COMMIT PIPELINE ───
	owner := os.Getenv("GITHUB_OWNER")
	repo := os.Getenv("GITHUB_REPO")
	branch := os.Getenv("GITHUB_BRANCH")
	if branch == "" {
		branch = "main"
	}
	token := os.Getenv("GITHUB_PAT")
	path := "site/src/data/menu.json"

	if owner == "" || repo == "" || token == "" {
		log.Println("[ERROR] GitHub configuration missing (GITHUB_OWNER, GITHUB_REPO, or GITHUB_PAT)")
		http.Error(w, "Server GitHub integration not configured", http.StatusInternalServerError)
		return
	}

	// 1. Fetch current file SHA from GitHub
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	getReq, err := http.NewRequestWithContext(r.Context(), "GET", getURL, nil)
	if err != nil {
		http.Error(w, "Failed to create GitHub request", http.StatusInternalServerError)
		return
	}
	getReq.Header.Set("Authorization", "Bearer "+token)
	getReq.Header.Set("Accept", "application/vnd.github.v3+json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	getResp, err := httpClient.Do(getReq)
	if err != nil || getResp.StatusCode != http.StatusOK {
		log.Printf("GitHub fetch file info failed (status: %d): %v", getResp.StatusCode, err)
		http.Error(w, "Failed to retrieve current menu state from GitHub", http.StatusBadGateway)
		return
	}

	var getResult struct {
		Sha string `json:"sha"`
	}
	json.NewDecoder(getResp.Body).Decode(&getResult)
	getResp.Body.Close()

	// 2. Format JSON and encode to Base64
	// CHANGED: json.Indent on the RAW body instead of json.MarshalIndent
	// on the decoded map. MarshalIndent re-sorts all map keys
	// alphabetically (Go maps are unordered); json.Indent only
	// normalizes whitespace (indentation/newlines) and preserves key
	// order byte-for-byte — so the admin's section ordering from the
	// editor survives the commit. Note json.Indent also re-validates
	// the JSON (it errors on malformed input), a harmless second check
	// given Unmarshal already passed.
	var formattedBuffer bytes.Buffer
	if err := json.Indent(&formattedBuffer, rawBody, "", "  "); err != nil {
		http.Error(w, "Failed to serialize JSON for commit", http.StatusInternalServerError)
		return
	}
	// Append newline to match standard POSIX file conventions
	formattedJSON := append(formattedBuffer.Bytes(), '\n')
	encodedContent := base64.StdEncoding.EncodeToString(formattedJSON)

	// 3. Send commit PUT request to GitHub
	putURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)
	putPayload := map[string]string{
		"message": "chore(menu): update navigation configuration from admin dashboard",
		"content": encodedContent,
		"sha":     getResult.Sha,
		"branch":  branch,
	}
	putBody, _ := json.Marshal(putPayload)

	putReq, err := http.NewRequestWithContext(r.Context(), "PUT", putURL, bytes.NewBuffer(putBody))
	if err != nil {
		http.Error(w, "Failed to prepare commit request", http.StatusInternalServerError)
		return
	}
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Accept", "application/vnd.github.v3+json")
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := httpClient.Do(putReq)
	if err != nil || (putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated) {
		log.Printf("GitHub commit failed (status: %d): %v", putResp.StatusCode, err)
		http.Error(w, "Failed to commit changes to GitHub repository", http.StatusBadGateway)
		return
	}
	putResp.Body.Close()

	// Record the commit time — /api/deploy-status uses this to decide
	// whether the latest Cloudflare deployment includes this change.
	lastMenuCommitMu.Lock()
	lastMenuCommitAt = time.Now().UTC()
	lastMenuCommitMu.Unlock()

	log.Printf("[ADMIN] Successfully committed updated menu.json to GitHub on branch %s", branch)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Menu committed successfully. Build triggered on Cloudflare.",
	})
}

// deployStatusHandler reports whether the Cloudflare Pages build that
// includes the admin's last menu.json commit has finished. The editor
// polls this after "Save & Deploy" and reloads the page only once the
// NEW build is live — closing the gap where a reload fired before the
// build existed and served the previous (stale) bundle.
//
// Status lives one level deep in the Pages API: the deployment object
// carries created_on, and its latest_stage object carries the live
// status (name: queued/build/deploy, status: idle/active/success/
// failure/canceled). The deployment is complete only when
// latest_stage.status is a terminal state.
//
// Requires env vars: CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_PROJECT_NAME,
// CLOUDFLARE_API_TOKEN (Pages:Read scope is sufficient).
func deployStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lastMenuCommitMu.Lock()
	commitAt := lastMenuCommitAt
	lastMenuCommitMu.Unlock()

	if commitAt.IsZero() {
		// No menu commit has happened since the server booted.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"state": "idle", "message": "No menu deploy in progress."})
		return
	}

	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	project := os.Getenv("CLOUDFLARE_PROJECT_NAME")
	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if accountID == "" || project == "" || token == "" {
		// Misconfiguration must be visible in server logs, not just the
		// editor's "not configured" message.
		log.Println("[ADMIN] Cloudflare status check skipped: environment variables not fully configured.")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"state": "unknown", "message": "Cloudflare status not configured on server."})
		return
	}

	// Latest deployment for the project (list is newest-first).
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s/deployments?per_page=1", accountID, project)
	req, err := http.NewRequestWithContext(r.Context(), "GET", url, nil)
	if err != nil {
		http.Error(w, "Failed to build status request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("[ADMIN] Cloudflare API request failed (status: %d): %v", resp.StatusCode, err)
		http.Error(w, "Failed to query Cloudflare status", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// latest_stage is nested one level inside each deployment object.
	var cfResp struct {
		Result []struct {
			CreatedOn   time.Time `json:"created_on"`
			LatestStage struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"latest_stage"`
		} `json:"result"`
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &cfResp); err != nil || len(cfResp.Result) == 0 {
		log.Printf("[ADMIN] Failed to parse Cloudflare status response. Raw body: %s", string(bodyBytes))
		http.Error(w, "Failed to parse Cloudflare status", http.StatusBadGateway)
		return
	}

	latest := cfResp.Result[0]

	state := "building"
	message := "Cloudflare build in progress..."

	// latest_stage.status is the CURRENT stage's state. Terminal states:
	// success / failure / canceled. Anything else (active, idle, empty
	// while queued) means the build is still running — keep polling.
	switch strings.ToLower(latest.LatestStage.Status) {
	case "success":
		// The 15s buffer absorbs minor clock skew between Render and
		// Cloudflare so a SUCCESSFUL OLD build never reads as deployed
		// for a commit that just happened.
		if latest.CreatedOn.After(commitAt.Add(-15 * time.Second)) {
			state, message = "deployed", "New build is live."
			// One log line per OUTCOME — the deploy-half of the story the
			// commit log line started. Per-poll logging stays out: the
			// 6-second poll cadence is operational noise.
			log.Printf("[ADMIN] Cloudflare deployment live for menu commit at %s", commitAt.Format(time.RFC3339))
		} else {
			// SUCCESS, but from BEFORE this commit — the build for this
			// commit hasn't appeared as a deployment yet (or was skipped).
			state, message = "pending", "Waiting for the build to start..."
		}
	case "failure":
		state, message = "failed", "Cloudflare build failed — check the Pages dashboard."
		log.Printf("[ADMIN] Cloudflare build FAILED for menu commit at %s", commitAt.Format(time.RFC3339))
	case "canceled":
		state, message = "failed", "Cloudflare build was canceled — check the Pages dashboard."
		log.Printf("[ADMIN] Cloudflare build CANCELED for menu commit at %s", commitAt.Format(time.RFC3339))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"state": state, "message": message})
}

// catalogHandler streams the compressed catalog.json.gz with 304 ETag revalidation.
func catalogHandler(w http.ResponseWriter, r *http.Request) {
	//log.Println("called catalogHandler")
	clientVersion := r.URL.Query().Get("version")
	//fmt.Printf("catalogHandler - clientVersion: %s\n", clientVersion)
	ctx := r.Context()

	// 1. Ask R2 for the current ETag (The Source of Truth)
	head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
	var nf *types.NotFound
	if err != nil {
		if errors.As(err, &nf) {
			// The catalog is missing: a regeneration is already triggered
			// (or running). This is a rebuild-in-progress state, not a
			// server fault — the editor should retry, not treat it as a
			// crash. 503 + Retry-After tells the editor the difference.
			maybeTriggerRegeneration("public catalog request: catalog missing")
			w.Header().Set("Retry-After", "30")
			http.Error(w, "Catalog is being regenerated; try again shortly", http.StatusServiceUnavailable)
			return
		}
		log.Printf("Error heading catalog: %v", err)
		http.Error(w, "Failed to read catalog", http.StatusInternalServerError)
		return
	}

	r2ETag := *head.ETag
	//fmt.Printf("catalogHandler - r2ETag: %s\n", r2ETag)
	// 2. If client version matches R2 ETag, they are up to date!
	if clientVersion == r2ETag {
		//fmt.Println("catalogHandler - client is up to date")
		w.WriteHeader(http.StatusNotModified) // 304
		return
	}

	// 3. Client needs an update. Let's check our Go Server Cache.
	cachedETag, cachedBytes := catalogCache.Get()
	// 4. If Go Cache is stale or empty, fetch fresh bytes from R2
	if cachedETag != r2ETag || len(cachedBytes) == 0 {
		//log.Println("Go Server Cache Miss. Fetching catalog from R2...")
		freshBytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			http.Error(w, "Failed to fetch catalog from R2", http.StatusInternalServerError)
			return
		}
		// Update the Go Server RAM Cache
		catalogCache.Update(r2ETag, freshBytes)
		cachedETag = r2ETag
		cachedBytes = freshBytes
	}

	// 5. Stream the compressed bytes to the client
	// CHANGED: Tell the browser it's a binary blob, NOT auto-decompressing gzip
	w.Header().Set("Content-Type", "application/octet-stream")
	// REMOVED: w.Header().Set("Content-Encoding", "gzip")
	// Tell the browser it's JSON, but it's Gzipped (Browser will auto-decompress!)
	//w.Header().Set("Content-Type", "application/json")
	//w.Header().Set("Content-Encoding", "gzip")

	w.Header().Set("ETag", cachedETag) // Send the ETag so the client can save it
	w.Write(cachedBytes)

}

// catalogSchemaHandler serves the authoritative catalog schema: the full
// field list, its writable/derived classification, and a checksum over a
// canonical (sorted, deduplicated, newline-joined) form.
//
// It is the single source of truth for the CLI tools: they fetch this at
// startup rather than compiling a copy of schema.go, so a stale binary
// can never silently disagree with the server about which fields exist.
// The checksum is compared (not a version number): "same or different;
// rectify if different" is all the contract requires.
//
// Public by design: the field list is not sensitive, and an
// unauthenticated dry-run (diffing a CSV against the catalog) must be
// possible without admin credentials.
func catalogSchemaHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fields":   schema.CatalogSchema,
		"writable": schema.CatalogSchemaWritable,
		"derived":  schema.CatalogSchemaDerived,
		"checksum": schema.Checksum(),
	})
}

// --- TRANSPORT LAYER (HTTP Handlers) ---

// downloadHandler generates short-lived presigned URLs for track audio and logs stats.
func downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "Missing 'file' query parameter", http.StatusBadRequest)
		return
	}

	// Ask the library for the URL
	url, err := storageClient.GetDownloadURL(r.Context(), fileName, 5*time.Minute)
	if err != nil {
		// Note: The library automatically skips .json files and folders
		http.Error(w, "Failed to generate signed URL", http.StatusInternalServerError)
		log.Printf("Error signing downloadURL: %v", err)
		return
	}

	// Usage statistics: what was requested, from where, when. The primary
	// public-usage signal. Catalog fetches are deliberately NOT logged —
	// they fire on every page load and would be pure noise.
	stats.Record(Event{Kind: "track_download", File: fileName, IP: clientIP(r)})

	response := map[string]string{"url": url}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Limit upload size to 50MB to protect your server
	r.ParseMultipartForm(50 << 20) // 50MB limit

	// 2. Get the file from the form data (key name: "file")
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 3. Get metadata from form data
	title := r.FormValue("title")
	artist := r.FormValue("artist")
	audioHash := r.FormValue("audio-hash")

	metadata := map[string]string{
		"title":      title,
		"artist":     artist,
		"audio-hash": audioHash, // Saves the fingerprint to R2
	}

	// 4. Call the library to upload!
	err = storageClient.UploadFile(r.Context(), header.Filename, header.Size, file, metadata)
	if err != nil {
		http.Error(w, "Failed to upload file to R2", http.StatusInternalServerError)
		log.Printf("Error uploading: %v", err)
		return
	}

	//5. Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "file": header.Filename})
}

// updateMetadataHandler processes a request from the admin UI to overwrite
// the custom metadata for a specific track in R2.
// Because R2 does not allow patching metadata directly, this triggers a
// server-side "Copy-Over-Self" operation via the storage client.
// The catalog is then hot-patched through the shared store, in the same
// request: step 1 (R2 metadata) and step 2 (catalog) are both durable
// before the response is sent.

func updateMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MetadataUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	// Schema filtering boundary — unchanged behaviour (permissive drop).
	// This single filter covers both consumers of req.Metadata: the R2
	// metadata write AND the catalog hot-patch. The deletion set is
	// captured HERE, before the write: UpdateMetadata consumes the merged
	// map (where empty means delete), but the catalog needs the
	// operator's intent, so every empty-valued field is recorded as an
	// explicit deletion for PatchTrack.
	filtered := make(map[string]string, len(req.Metadata))
	var deletes []string
	for _, field := range schema.CatalogSchema {
		if v, ok := req.Metadata[field]; ok {
			filtered[field] = v
			if v == "" {
				deletes = append(deletes, field)
			}
		}
	}
	req.Metadata = filtered

	ctx := r.Context()

	// 1. Update the MP3's metadata in R2 (copy-based: the body never
	//    leaves R2). If this fails, NOTHING has changed — the caller
	//    should retry, so this is a hard 500, not a partial.
	if err := storageClient.UpdateMetadata(ctx, req.Filename, req.Metadata); err != nil {
		log.Printf("Failed to update metadata for %s: %v", req.Filename, err)
		http.Error(w, "Failed to update metadata", http.StatusInternalServerError)
		return
	}

	// 2. HOT PATCH — the step that was missing. R2 metadata is already
	//    updated (step 1 succeeded); the catalog must now reflect it in
	//    the same request, before the response is sent.
	found, err := catalogStore.PatchTrack(ctx, req.Filename, req.Metadata, deletes)
	if err != nil {
		detail := "R2 metadata updated; catalog sync failed — automatic regeneration triggered"
		if errors.Is(err, catalog.ErrCatalogMissing) {
			// Load already fired the onMissing hook; no explicit trigger needed.
			detail = "R2 metadata updated; catalog missing — automatic regeneration started"
		} else {
			// F3: transient write failure (corrupt gzip, R2 error). The
			// catalog is stale as of this moment; regeneration repairs it.
			maybeTriggerRegeneration("catalog sync failed after metadata update: " + req.Filename)
		}
		log.Printf("Metadata updated for %s but catalog sync issue: %v", req.Filename, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "partial",
			"detail": detail,
		})
		return
	}
	if !found {
		// R2 metadata was updated, but the rebuilt catalog has no entry
		// for this filename. Nothing to patch — the catalog's copy is
		// rebuilt from R2 metadata by the next regeneration, which is
		// triggered here so the entry appears automatically.
		log.Printf("[ADMIN] WARNING: R2 metadata updated for %q but catalog has NO matching entry — patch skipped. Regeneration triggered.", req.Filename)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "partial",
			"detail": "R2 metadata updated; track has no catalog entry — automatic regeneration triggered",
		})
		maybeTriggerRegeneration("catalog entry missing for " + req.Filename)
		return
	}

	// 3. Return Success — same shape as before.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// deleteTrackHandler deletes a single track from R2 and hot‑patches catalog.json.gz.
func deleteTrackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// 1. Delete the object from R2
	err := storageClient.DeleteObject(ctx, req.Filename)
	if err != nil {
		log.Printf("Failed to delete %s from R2: %v", req.Filename, err)
		http.Error(w, "Failed to delete from R2", http.StatusInternalServerError)
		return
	}

	// 2. Hot-patch: remove the entry through the shared store.
	found, err := catalogStore.RemoveTrack(ctx, req.Filename)
	if err != nil {
		detail := "R2 object deleted; catalog sync failed — automatic regeneration triggered"
		if errors.Is(err, catalog.ErrCatalogMissing) {
			detail = "R2 object deleted; catalog missing — automatic regeneration started"
		} else {
			// F3: R2 delete succeeded but the catalog write failed. The
			// catalog now references a deleted object — regeneration is
			// the repair, and it must not wait for the next scheduled crawl.
			maybeTriggerRegeneration("catalog sync failed after delete: " + req.Filename)
		}
		log.Printf("Track deleted from R2 but catalog sync issue %v: %v", req.Filename, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "partial",
			"detail": detail,
		})
		return
	}
	if !found {
		// Catalog already consistent — no entry, nothing to repair.
		log.Printf("Deleted %s from R2; catalog had no matching entry", req.Filename)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// UploadTrackRequest defines the metadata payload sent alongside the file - defined at the top for reuse in both the uploadTrackHandler and updateMetadataHandler
// This handler is designed for an admin tool that allows uploading a new track
//
//	along with its metadata in one request.
func uploadTrackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// 1. Parse the multipart form (max 50MB in memory)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	// 2. Retrieve the file from the form
	file, _, err := r.FormFile("audioFile")
	if err != nil {
		http.Error(w, "Missing audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read the file into a byte slice (for an admin tool, buffering up to 50MB in RAM is safe)
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	// 3. Retrieve the metadata JSON string from the form
	metadataStr := r.FormValue("metadata")
	var req UploadTrackRequest
	if err := json.Unmarshal([]byte(metadataStr), &req); err != nil {
		http.Error(w, "Invalid metadata JSON", http.StatusBadRequest)
		return
	}

	// ─── INSERT: SCHEMA ENFORCEMENT ───
	filtered := make(map[string]string, len(req.Metadata))
	for _, field := range schema.CatalogSchema {
		if v, ok := req.Metadata[field]; ok {
			filtered[field] = v
		}
	}
	req.Metadata = filtered
	// ─── END INSERT ───

	// 4. Upload the file to R2 with custom metadata
	err = storageClient.PutObjectWithMetadata(ctx, req.Filename, fileBytes, req.Metadata)
	if err != nil {
		log.Printf("Failed to upload file to R2: %v", err)
		http.Error(w, "Failed to upload to R2", http.StatusInternalServerError)
		return
	}

	// 5. Get the R2 ETag (MD5) of the newly uploaded file.
	head, err := storageClient.GetMetadata(ctx, req.Filename)
	if err != nil {
		log.Printf("Warning: Failed to get ETag for new file: %v", err)
	}
	r2Hash := ""
	if head != nil && head.ETag != nil {
		r2Hash = strings.Trim(*head.ETag, `"`)
	}

	// 6. Append to the catalog via the shared store.
	if err := catalogStore.AppendTrack(ctx, req.Filename, r2Hash, req.Metadata); err != nil {
		detail := "file uploaded; catalog sync failed — automatic regeneration triggered"
		if errors.Is(err, catalog.ErrCatalogMissing) {
			detail = "file uploaded; catalog missing — automatic regeneration started"
		} else {
			// F3: the file is in R2 but the catalog doesn't know it. The
			// regeneration will enumerate the new object and add the entry.
			maybeTriggerRegeneration("catalog sync failed after upload: " + req.Filename)
		}
		log.Printf("File uploaded to R2 but catalog sync issue %v: %v", req.Filename, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "partial",
			"hash":   r2Hash,
			"detail": detail,
		})
		return
	}

	// 7. Success — same shape as before.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "hash": r2Hash})
}

func getSignedUploadURLHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	// Generate a URL that expires in 15 minutes (plenty of time for large file uploads)
	url, err := storageClient.GetUploadURL(r.Context(), filename, 15*time.Minute)
	if err != nil {
		http.Error(w, "Failed to generate upload URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

// startCrawlWorker is the background Goroutine that does the heavy lifting
func startCrawlWorker(jobID string) {
	// 1. Setup the progress channel
	progressChan := make(chan storage.CrawlProgress, 10) // Buffered channel

	// 2. Goroutine to listen to progress and update the registry
	go func() {
		for progress := range progressChan {
			job, _ := jobRegistry.Load(jobID)
			j := job.(*CrawlJob)

			j.Progress = progress.Percent
			j.Message = progress.Message

			// Save updated job back to registry
			jobRegistry.Store(jobID, j)
		}
	}()

	// 3. Execute the actual crawl
	catalogData, err := storageClient.CrawlCatalog(context.Background(), progressChan)
	close(progressChan) // Signal the listener to stop

	// 4. Handle Completion / Failure
	job, _ := jobRegistry.Load(jobID)
	j := job.(*CrawlJob)

	if err != nil {
		j.Status = "failed"
		j.Message = err.Error()
	} else {
		// ==========================================
		// NEW: Compress and Upload to R2
		// ==========================================
		j.Message = "Compressing catalog..."
		jobRegistry.Store(jobID, j)

		jsonData, err := json.MarshalIndent(catalogData, "", "  ")
		if err != nil {
			j.Status = "failed"
			j.Message = "Failed to marshal JSON"
			jobRegistry.Store(jobID, j)
			return
		}

		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		gzWriter.Name = "catalog.json"
		gzWriter.Write(jsonData)
		gzWriter.Close()
		gzBytes := buf.Bytes()

		j.Message = "Uploading catalog to R2..."
		jobRegistry.Store(jobID, j)

		err = storageClient.PutObjectBytes(context.Background(), "catalog.json.gz", gzBytes)
		if err != nil {
			j.Status = "failed"
			j.Message = "Failed to upload catalog to R2"
			jobRegistry.Store(jobID, j)
			return
		}

		// ==========================================
		// SUCCESS: Clear cache and finalize
		// ==========================================
		catalogCache.Clear() // Force the server to download the new one on next request

		j.Status = "completed"
		j.Progress = 100
		j.Message = "Catalog updated successfully."

	}

	jobRegistry.Store(jobID, j)
}

// 1. Trigger the Crawl
func startCrawlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate a unique Job ID (simple timestamp + random for now)
	jobID := fmt.Sprintf("crawl-%d", time.Now().UnixNano())

	// Initialize the Job in the registry
	jobRegistry.Store(jobID, &CrawlJob{
		ID:       jobID,
		Status:   "running",
		Progress: 0,
		Message:  "Initializing crawl...",
	})

	// Spin up the background worker
	go startCrawlWorker(jobID)

	// Immediately return 202 Accepted + JobID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

// 2. Check the Status
func crawlStatusHandler(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "Missing job_id", http.StatusBadRequest)
		return
	}

	job, exists := jobRegistry.Load(jobID)
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// validateCatalogBytes checks that the compressed bytes are a valid
// gzip stream containing a JSON object with a tracks array. It mirrors
// the decode the catalog store performs, so the warmup can detect a
// corrupt object at boot instead of caching bytes that will fail on
// first use.
func validateCatalogBytes(rawBytes []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(rawBytes))
	if err != nil {
		return fmt.Errorf("catalog gunzip failed: %w", err)
	}
	defer gz.Close()
	jsonBytes, err := io.ReadAll(gz)
	if err != nil {
		return fmt.Errorf("catalog read after gunzip failed: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return fmt.Errorf("catalog unmarshal failed: %w", err)
	}
	if _, ok := data["tracks"]; !ok {
		return fmt.Errorf("catalog has no tracks array")
	}
	return nil
}

// =============================================================================
// CORS MIDDLEWARE
// =============================================================================

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Define your allowed origins
		allowedOrigins := []string{
			"http://localhost:4321",        // Local Development
			"https://labriideas.pages.dev", // Production Cloudflare Site
		}

		// 2. Get the origin from the incoming request; if it is in our
		//    allow-list, reflect it back exactly. Unlisted origins get
		//    no ACAO header at all — the browser then blocks the
		//    response, which is the CORS layer doing its (limited) job.
		//    NOTE: CORS is a browser convenience, NOT the security
		//    boundary — authMiddleware is. curl ignores CORS entirely.
		origin := r.Header.Get("Origin")
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		// 3. Allow the methods we use
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		// 4. Allow request headers. CHANGED: added Authorization —
		//    REQUIRED for the Bearer-token auth overlay. It is a
		//    non-simple header, so any authed request triggers a
		//    preflight (OPTIONS); without it listed here the browser
		//    kills the request client-side (ERR_FAILED) and it never
		//    reaches authMiddleware at all. Content-Type remains for
		//    JSON bodies and multipart/form-data uploads.
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 5. Expose the ETag header so the frontend can read it (for caching)
		w.Header().Set("Access-Control-Expose-Headers", "ETag")

		// 6. Cache the preflight result for a day — the browser stops
		//    re-sending OPTIONS for repeated uploads/fetches to the
		//    same route. Free performance win, no behavior change.
		w.Header().Set("Access-Control-Max-Age", "86400")

		// 7. Intercept the Preflight OPTIONS request and return 204.
		//    Must stay BEFORE next(w, r): preflights never carry
		//    credentials by design, so authMiddleware would 401 them.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 8. Pass normal requests to the actual handler
		next(w, r)
	}
}
