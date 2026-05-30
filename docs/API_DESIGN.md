# API Design Specification

## Overview
The system utilizes a "Data-Agnostic Proxy" model where the Go backend acts as an orchestrator between the Frontend (Svelte) and the Cloud Storage (R2).

## Metadata Update Workflow (`/api/update-metadata`)

### 1. The "Hot Patch" Pattern
Unlike standard databases, R2/S3 does not support partial metadata updates. To ensure the system remains performant and consistent, the Go backend implements the "Hot Patch" pattern:

1.  **Verification:** Upon receiving a `POST` request, the server fetches the object's `ETag` (hash) from R2.
2.  **Cache Validation:** The server compares the R2 `ETag` against the current RAM cache:
    *   **If hashes match:** The current RAM-resident `catalog.json.gz` is patched in-place.
    *   **If hashes mismatch:** The server downloads the latest copy from R2, then patches it.
3.  **Atomic Update:** The patched catalog is re-compressed (Gzip) and uploaded back to R2.
4.  **Metadata Synchronicity:** The specific MP3 file metadata is updated in R2 using the `CopyObject` (Copy-Over-Self) trick with `MetadataDirectiveReplace`.

### 2. Endpoint: `POST /api/update-metadata`
**Security:** Reserved for authenticated Administrators.

**Request Body:**
```json
{
  "filename": "path/to/song.mp3",
  "metadata": {
    "title": "New Title",
    "artist": "New Artist"
  }
}
