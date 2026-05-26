package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mikebarb/labriideas-publisher/pkg/schema"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CrawlCatalog scans the entire R2 bucket, reads metadata, and builds a dynamic Catalog map
func (c *Client) CrawlCatalog(ctx context.Context) (map[string]interface{}, error) {
	// 1. Use a slice of dynamic maps instead of a rigid struct
	var tracks []map[string]string

	// 2. Initialize the Paginator
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
	})

	// 3. Loop through pages of objects
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		// 4. Loop through items on this specific page
		for _, obj := range page.Contents {
			key := *obj.Key

			// A. Skip folders
			if strings.HasSuffix(key, "/") {
				continue
			}

			// B. Skip the manifest files
			if key == "catalog.json" || key == "catalog.json.gz" || key == "version.json" {
				continue
			}

			// C. Whitelist: ONLY process MP3 files
			if !strings.HasSuffix(strings.ToLower(key), ".mp3") {
				continue
			}

			// 5. Head the object to get its custom metadata
			headOutput, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(c.bucket),
				Key:    &key,
			})

			if err != nil {
				log.Printf("Warning: Failed to head object %s: %v", key, err)
				continue
			}

			// 6. DYNAMIC MAPPING using the Global Schema
			trackData := make(map[string]string)

			for _, fieldName := range schema.CatalogSchema {
				switch fieldName {
				case "id", "filename":
					// ID and Filename are derived from the R2 Key
					trackData[fieldName] = key
				case "hash":
					// Hash is derived from the R2 ETag (stripping the quotes AWS adds)
					trackData[fieldName] = strings.Trim(*headOutput.ETag, `"`)
				default:
					// For everything else (title, artist, future fields), grab from custom metadata
					// R2 normalizes metadata keys to lowercase, which perfectly matches our lowercase schema
					trackData[fieldName] = headOutput.Metadata[fieldName]
				}
			}

			tracks = append(tracks, trackData)
		}
	}

	// 7. Build the final dynamic Catalog map
	catalog := map[string]interface{}{
		"release": fmt.Sprintf("r2-crawl-%s", time.Now().Format("2006-01-02-15-04-05")),
		"count":   len(tracks),
		"tracks":  tracks,
	}

	return catalog, nil
}
