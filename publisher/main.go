package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/cache"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

//"io"

// Global storage client
var storageClient *storage.Client
var catalogCache *cache.CatalogCache

type MetadataUpdateRequest struct {
	Filename string            `json:"filename"`
	Metadata map[string]string `json:"metadata"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on system env vars")
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

	// Create the raw S3 Client
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + accountID + ".r2.cloudflarestorage.com")
	})

	// Initialize YOUR Library Client!
	storageClient = storage.NewClient(s3Client, bucketName)
	catalogCache = cache.NewCatalogCache() // Initialize empty cache

	// Setup HTTP Routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/download", corsMiddleware(downloadHandler))
	mux.HandleFunc("/api/upload", corsMiddleware(uploadHandler))
	mux.HandleFunc("/api/catalog", corsMiddleware(catalogHandler))
	mux.HandleFunc("/api/update-metadata", corsMiddleware(updateMetadataHandler))

	// BACKGROUND CACHE WARMUP
	go func() {
		log.Println("Warming up catalog cache...")
		ctx := context.Background()

		// 1. Get ETag from R2
		head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
		if err != nil {
			log.Printf("Cache warmup failed (metadata): %v", err)
			return
		}
		r2ETag := *head.ETag

		// 2. Get compressed bytes from R2
		bytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			log.Printf("Cache warmup failed (download): %v", err)
			return
		}

		// 3. Update RAM Cache
		catalogCache.Update(r2ETag, bytes)
		log.Println("✅ Catalog cache warmed up successfully!")
	}()

	log.Println("🚀 Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// --- CATALOG HANDLER ---
func catalogHandler(w http.ResponseWriter, r *http.Request) {
	clientVersion := r.URL.Query().Get("version")
	fmt.Printf("catalogHandler - clientVersion: %s\n", clientVersion)
	ctx := r.Context()

	// 1. Ask R2 for the current ETag (The Source of Truth)
	head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
	if err != nil {
		http.Error(w, "Failed to check storage metadata", http.StatusInternalServerError)
		log.Printf("Error heading catalog: %v", err)
		return
	}
	r2ETag := *head.ETag
	fmt.Printf("catalogHandler - r2ETag: %s\n", r2ETag)
	// 2. If client version matches R2 ETag, they are up to date!
	if clientVersion == r2ETag {
		fmt.Println("catalogHandler - client is up to date")
		w.WriteHeader(http.StatusNotModified) // 304
		return
	}

	// 3. Client needs an update. Let's check our Go Server Cache.
	cachedETag, cachedBytes := catalogCache.Get()
	fmt.Printf("catalogHandler - cachedETag: %s\n", cachedETag)
	// 4. If Go Cache is stale or empty, fetch fresh bytes from R2
	if cachedETag != r2ETag || len(cachedBytes) == 0 {
		log.Println("Go Server Cache Miss. Fetching catalog from R2...")
		fmt.Printf("catalogHandler - Go Server Cache Miss. Fetching catalog from R2...\n")
		freshBytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			http.Error(w, "Failed to fetch catalog", http.StatusInternalServerError)
			return
		}
		// Update the Go Server RAM Cache
		catalogCache.Update(r2ETag, freshBytes)
		cachedETag = r2ETag
		cachedBytes = freshBytes
		fmt.Printf("catalogHandler - Updated server cache\n")
	}

	// 5. Stream the compressed bytes to the client
	fmt.Printf("catalogHandler - Stream catalogue to client\n")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("ETag", cachedETag) // Send the ETag so the client can save it
	w.Write(cachedBytes)

}

// --- TRANSPORT LAYER (HTTP Handlers) ---

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
		log.Printf("Error signing URL: %v", err)
		return
	}

	response := map[string]string{"url": url}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Limit upload size to 50MB to protect your server
	r.ParseMultipartForm(50 << 20)

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

	metadata := map[string]string{
		"title":  title,
		"artist": artist,
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

	ctx := r.Context()

	// 1. Update the MP3's metadata in R2
	err := storageClient.UpdateMetadata(ctx, req.Filename, req.Metadata)
	if err != nil {
		log.Printf("Failed to update metadata for %s: %v", req.Filename, err)
		http.Error(w, "Failed to update R2 metadata", http.StatusInternalServerError)
		return
	}

	// 2. HOT PATCH: Ensure catalog.json stays in sync

	// A. Check the current R2 ETag vs our RAM Cache ETag
	r2Head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
	if err != nil {
		log.Printf("Warning: Could not check R2 catalog ETag: %v", err)
		catalogCache.Clear()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		return
	}
	r2Etag := *r2Head.ETag

	var catalogGzBytes []byte
	ramEtag, ramBytes := catalogCache.Get()

	if ramEtag == r2Etag && len(ramBytes) > 0 {
		// B. HASHES MATCH: RAM is fresh! Use the RAM bytes (saves downloading the whole file)
		catalogGzBytes = ramBytes
	} else {
		// C. HASHES DO NOT MATCH: RAM is stale. Fetch the absolute latest from R2.
		log.Println("Cache stale during admin edit. Fetching fresh catalog from R2.")
		freshBytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			log.Printf("Warning: Could not fetch fresh catalog: %v", err)
			catalogCache.Clear()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
			return
		}
		catalogGzBytes = freshBytes
	}

	// D. Decompress the chosen bytes
	gzReader, err := gzip.NewReader(bytes.NewReader(catalogGzBytes))
	if err == nil {
		jsonBytes, err := io.ReadAll(gzReader)
		gzReader.Close()

		if err == nil {
			// E. Unmarshal into dynamic map
			var catalogData map[string]interface{}
			if json.Unmarshal(jsonBytes, &catalogData) == nil {

				// F. Find the track and update its fields
				if tracks, ok := catalogData["tracks"].([]interface{}); ok {
					for _, t := range tracks {
						if trackMap, ok := t.(map[string]interface{}); ok {
							if trackMap["filename"] == req.Filename {
								for key, value := range req.Metadata {
									trackMap[key] = value
								}
								break
							}
						}
					}
				}

				// G. Re-marshal to JSON
				newJsonBytes, err := json.Marshal(catalogData)
				if err == nil {
					// H. Re-compress to Gzip
					var buf bytes.Buffer
					gzWriter := gzip.NewWriter(&buf)
					gzWriter.Write(newJsonBytes)
					gzWriter.Close()
					newGzBytes := buf.Bytes()

					// I. Upload the freshly patched catalog.json.gz back to R2
					err := storageClient.PutObjectBytes(ctx, "catalog.json.gz", newGzBytes)
					if err != nil {
						log.Printf("Warning: Failed to upload patched catalog to R2: %v", err)
					}

					// J. Update the RAM cache with the new bytes and fetch the NEW R2 ETag
					newHead, _ := storageClient.GetMetadata(ctx, "catalog.json.gz")
					newEtag := ""
					if newHead != nil {
						newEtag = *newHead.ETag
					}
					catalogCache.Update(newEtag, newGzBytes)
				}
			}
		}
	}

	// 3. Return Success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// --- MIDDLEWARE ---

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		w.Header().Set("Access-Control-Expose-Headers", "ETag")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}
