# API Documentation

## 1. Go Backend (Publisher)

### `catalogHandler`
- **Purpose**: Orchestrates the version handshake and streaming of the catalog data.
- **Input Parameters**: 
    - `w http.ResponseWriter`: The interface used to construct the HTTP response.
    - `r *http.Request`: The incoming browser request containing the `version` query param.
- **Returned Results**: None (writes directly to response body).
- **Technical Description**: Checks client `version` against R2 `ETag`. If they match, returns `304`. If they differ, it streams the compressed binary blob from the RAM cache.
- **Design Considerations**: Uses `HeadObject` to minimize R2 bandwidth costs; uses `sync.RWMutex` to ensure thread-safety during cache updates.

### `catalogCache.Update(etag, data)`
- **Purpose**: Safely replaces the in-memory catalog cache.
- **Input Parameters**: 
    - `etag (string)`: The R2-generated hash.
    - `data ([]byte)`: The compressed Gzip file content.
- **Returned Results**: None.
- **Technical Description**: Uses a `Write Lock` to stop other requests briefly while memory is updated.
- **Design Considerations**: Must remain thread-safe to avoid memory corruption under concurrent loads.

## 2. Frontend (Svelte)

### `loadCatalog()`
- **Purpose**: The primary entry point for catalog data in the browser.
- **Technical Description**:
    1. Fetches from the Go proxy passing the current local `ETag`.
    2. If the server returns `304`, the app loads existing data from `localStorage`. 
    3. If the server sends a new catalog, it saves the new `ETag` and data.
    4. Runs `inflateCatalog()` to turn binary data into a usable JavaScript object.
