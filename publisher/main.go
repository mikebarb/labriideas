package main

import (
	"context"
	"encoding/json"
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
	mux.HandleFunc("/api/upload", corsMiddleware(uploadHandler)) // NEW!
	mux.HandleFunc("/api/catalog", corsMiddleware(catalogHandler))

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
	ctx := r.Context()

	// 1. Ask R2 for the current ETag (The Source of Truth)
	head, err := storageClient.GetMetadata(ctx, "catalog.json.gz")
	if err != nil {
		http.Error(w, "Failed to check storage metadata", http.StatusInternalServerError)
		log.Printf("Error heading catalog: %v", err)
		return
	}
	r2ETag := *head.ETag

	// 2. If client version matches R2 ETag, they are up to date!
	if clientVersion == r2ETag {
		w.WriteHeader(http.StatusNotModified) // 304
		return
	}

	// 3. Client needs an update. Let's check our Go Server Cache.
	cachedETag, cachedBytes := catalogCache.Get()

	// 4. If Go Cache is stale or empty, fetch fresh bytes from R2
	if cachedETag != r2ETag || len(cachedBytes) == 0 {
		log.Println("Go Server Cache Miss. Fetching catalog from R2...")
		freshBytes, err := storageClient.GetObjectBytes(ctx, "catalog.json.gz")
		if err != nil {
			http.Error(w, "Failed to fetch catalog", http.StatusInternalServerError)
			return
		}
		// Update the Go Server RAM Cache
		catalogCache.Update(r2ETag, freshBytes)
		cachedETag = r2ETag
		cachedBytes = freshBytes
	}

	// 5. Stream the compressed bytes to the client
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
