# Crawler Documentation

## 1. Overview
The Crawler is the "Orchestrator" of the asset pipeline. It performs an automated scan of the R2 bucket to reconcile the physical storage (MP3 files) with the logical manifest (`catalog.json.gz`).

## 2. Dynamic Schema Mapping
The Crawler is **Schema-Driven**. It does not use hardcoded data structures. Instead, it references `schema/schema.go`. When a new field (e.g., "album" or "bpm") is added to the system schema, the Crawler automatically detects the corresponding R2 metadata field and includes it in the catalog.

## 3. Logical Flow

### `Main.go`
- **Purpose**: Main.go is the root program for this tool.
- **Input**: Nil - Cloundflare R2 storage bucket is scanned.
- **Output**: catalog.json.gz output to local disk.
- **Internal Flow**: 
    - 1. Load R2 Credentials from Environment Variables
    - 2. Configure the AWS SDK to point to Cloudflare R2
    - 3. Create the raw S3 client
    - 4. Create YOUR storage client using the function you just showed me
    - 5. Run the Crawl => call CrawlCatalog
    - 6. Marshal the dynamic map to JSON
    - 7. Write the Gzip file to the local disk so we can inspect it

### `Crawler.go`
- **Purpose**: Crawler.go is a library of functional tools that does the heavy lifting scanning Cloudflare R2 storage and returning a catalog map from the stored mpeg files.
- **Function**: CrawlCatalog
- **Input**: *Client.
- **Output**: Catalog map
- **Internal Flow**: 
    - 1. Use a slice of dynamic maps instead of a rigid struct
    - 2. Initialize the Paginator - read back page at a time from R2
    - 3. Loop through pages of objects
    - 4. ..Loop through items on this specific page
    - 5. ....Head the object to get its custom metadata
    - 6. ....DYNAMIC MAPPING using the Global Schema
    - 7. Build the final dynamic Catalog map

## 4. Function Reference

### `CrawlCatalog(ctx context.Context)`
- **Purpose**: Performs a full bucket scan to generate a current-state manifest.
- **Input Parameters**: 
    - `ctx context.Context`: Standard Go context for timeout and cancellation control.
- **Returned Results**: 
    - `map[string]interface{}`: A dynamic JSON-compatible dictionary representing the catalog.
    - `error`: Error state if R2 communication fails.
- **Technical Description**:
  1. **Pagination**: Uses `ListObjectsV2Paginator` to safely handle buckets of any size.
  2. **Filtering**: Automatically ignores folders, manifest files, and non-MP3 assets.
  3. **Head Request**: Performs a `HeadObject` on every MP3 to extract custom R2 metadata.
  4. **Dynamic Mapping**: Iterates over `schema.CatalogSchema` to construct the track map.
     - `id/filename` are mapped to the R2 Object Key.
     - `hash` is mapped to the R2 ETag.
     - Other fields (e.g., `title`, `artist`) are mapped to R2 Metadata keys.
- **Design Considerations**: The dynamic switch-case loop ensures the crawler never requires recompilation when the data format changes.

### `getValue(row, columnName)` (CSV Tool)
- **Purpose**: Extracts values from spreadsheet input by column name rather than position. The spreadsheet must be exported to a comma delimited file which is read into the tool.
- **Technical Description**: Uses a header-map generated from the first row of the CSV. This makes the tool immune to column reordering or the addition of extra, non-indexed spreadsheet columns.
- **Usage Note**: The spreadsheet header titles must match the schema fild names exactly for correct data to be imported into the json file from the exported spreadsheet file. 
