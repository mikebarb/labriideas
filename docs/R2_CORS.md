# Infrastructure Documentation: R2 Audio Streaming Pipeline

## 1. Overview
The system utilizes a stateless architecture where the Go backend generates time-limited Presigned URLs for static assets stored in Cloudflare R2. Playback is handled by the browser's native HTML5 <audio> element, which streams directly from R2's edge nodes.

## 2. Browser-to-R2 CORS Policy
To enable streaming features such as seeking (via Range requests) and native browser playback security, the R2 bucket must be configured to permit cross-origin requests from your application's deployment domain.

### Bucket CORS Policy
Apply the following JSON configuration in the Cloudflare R2 dashboard:

    [
      {
        "AllowedOrigins": [
          "http://localhost:4321",
          "https://your-production-domain.com"
        ],
        "AllowedMethods": [
          "GET",
          "HEAD"
        ],
        "AllowedHeaders": [
          "Range"
        ],
        "ExposeHeaders": [
          "Content-Length",
          "Content-Range",
          "Accept-Ranges"
        ],
        "MaxAgeSeconds": 86400
      }
    ]

### Configuration Breakdown
- AllowedOrigins: Whitelists your development (localhost) and production domains.
- AllowedMethods: GET allows stream retrieval; HEAD allows the player to fetch file metadata (duration/size) without downloading the full object.
- AllowedHeaders (Range): Essential for VBR (Variable Bit Rate) audio and allowing users to skip to any point in a track. This tells R2 to treat the file as a stream rather than a static download.
- ExposeHeaders: Allows the browser to access file metadata necessary for correct UI progress bar rendering.
- MaxAgeSeconds: Caches the CORS pre-flight handshake for 24 hours.

---

## 3. Player Integration
The Player component expects the crossorigin="anonymous" attribute to trigger the CORS handshake.

### Implementation Pattern
<audio
  bind:this={audioElement}
  preload="auto"
  crossorigin="anonymous"
/>

### Communication Protocol
Because the Player and CatalogViewer are isolated Astro Islands, they communicate via browser CustomEvents on the window object:

- To Play a Track:
  window.dispatchEvent(new CustomEvent('play-track', { detail: track }));

- To Update Catalog:
  window.dispatchEvent(new CustomEvent('catalog-loaded', { detail: catalog }));

### Presigned URL Strategy
To prevent mid-song cutouts, the Player implements a proactive URL refresh:
1. The backend issues a Presigned URL with a fixed TTL.
2. The Player schedules a refresh timer at (TTL - buffer).
3. Before the URL expires, the component requests a new one and swaps the audioElement.src seamlessly. The browser maintains the playback position via the Range header and native caching.
