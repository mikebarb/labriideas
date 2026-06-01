# Player.svelte Documentation

## 1. Overview
The `Player.svelte` component is the high-performance playback engine for the streaming architecture. It acts as a bridge between the R2-hosted audio assets and the user interface, managing secure access, playback state, and network resilience.

## 2. Architecture
- **Stateless Operation:** Built for a stateless environment; no database connection is required at the client level.
- **Direct Edge Streaming:** Audio data streams directly from R2 Edge nodes to the browser using native `<audio>` Range requests, ensuring low-latency playback and minimal server-side overhead.
- **Proactive Security:** Instead of handling raw files, the player manages time-limited Presigned URLs obtained from the Go proxy.
- **Tailwind-First UI:** The component is fully styled using Tailwind CSS utility classes for maximum performance and design consistency.

## 3. Core Features
- **Seamless Stream Hand-off:** A non-intrusive `scheduleUrlRefresh` logic detects when a Presigned URL is nearing its TTL expiry and silently updates the source, preventing playback cutouts.
- **Robust Error Recovery:** Features an exponential backoff retry mechanism (2s, 4s, 8s) to handle network jitters or session-related access errors.
- **Range Request Native Support:** Because the browser interacts directly with R2 via the signed URL, seeking is instantaneous and does not require re-buffering the entire file.
- **Admin-Only Context:** The UI dynamically enables administrative tools (download, metadata editing) based on a reactive `isAdmin` boolean, supporting future JWT/RBAC integration.

## 4. Props Interface

| Prop | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `tracks` | `Track[]` | `[]` | Array of track objects fetched from `/api/catalog`. |
| `apiBase` | `string` | `''` | The origin/base path for the Go Proxy API. |
| `urlTtlSeconds` | `number` | `3600` | Expiration time for Presigned URLs (in seconds). |
| `refreshBufferSeconds` | `number` | `120` | Buffer time before expiry for automatic URL rotation. |
| `isAdmin` | `boolean` | `false` | Toggles visibility of administrative playback tools. |
| `autoplay` | `boolean` | `false` | If true, initiates playback of the first track on mount. |

## 5. Integration Guide
The component is designed to be utilized within an Astro-based build. Use client-side directives (`client:load`) to initialize the Svelte island.

    ---
    import Player from '../components/Player.svelte';
    const catalog = await fetch(`${Astro.site}api/catalog`).then(r => r.json());
    ---

    <Player 
        client:load 
        tracks={catalog.tracks} 
        isAdmin={true} 
    />

## 6. Security Flow
1. **Request:** The player calls the Go proxy endpoint `/api/download?file=...`.
2. **Authorization:** The Go proxy validates the user request and generates an R2-compatible Presigned URL.
3. **Delivery:** The URL is returned to the client and assigned to the `<audio>` element.
4. **Expiry Management:** The player tracks `expiresAt` and initiates a new fetch request before the URL is invalidated, preserving the current `currentTime` (playback position) across the swap.

## 7. Design Notes
- **Styling:** The component uses Tailwind utility classes for all styling. Custom scrollbar behavior is managed in a minimal block at the bottom of the component.
- **Preloading:** The engine uses `preload="auto"` to ensure the first few seconds of audio are cached at the edge.
- **CORS:** Ensure the R2 bucket has valid CORS headers configured to allow `GET` requests from your frontend origin.

## 11. Streaming Mechanics: The Range Request Flow

The system leverages R2’s native support for HTTP Range requests. This is the cornerstone of our "Zero-Overhead" streaming strategy, ensuring that audio playback is snappy and efficient.

### The Request Lifecycle
The browser handles playback and seeking by communicating directly with R2 Edge nodes:

    Browser <audio>           R2 Edge Node
         │                        │
         │  GET /presigned-url    │
         │  Range: bytes=0-1048575│  ← browser sends this automatically on seek
         │ ──────────────────────▶│
         │                        │
         │  206 Partial Content   │  ← R2 natively supports Range
         │  Content-Range: 0-...  │
         │◀──────────────────────│
         │                        │

### Critical Insights
*   **Protocol Transparency:** R2 (S3-compatible) handles `Range` headers natively. The Presigned URL signature covers the object path and query parameters, but the `Range` request is an independent HTTP-level header that passes through the signature verification without interference.
*   **No Proxy-Through:** The Go proxy acts strictly as a "gatekeeper" to generate access tickets. Once the ticket is issued, the audio data flows directly from the R2 Edge node to the browser, keeping your Go server stateless and light.
*   **Latency-Free Seeking:** Because the browser is talking directly to R2, seeking is instantaneous. The browser simply issues a new `Range` request for the specific byte-offset of the destination timestamp.
*   **Bandwidth Efficiency:** Only the specific byte ranges requested by the browser are transferred, significantly reducing egress costs and improving start times for end-users.



=================================================================================================
AFTER DEBUGGING
=================================================================================================

# Component Documentation: Player.svelte

## 1. Overview
The `Player` component is an isolated Svelte 5 "Island" designed to act as a persistent playback engine. It handles audio buffering, seamless URL rotating (to manage Presigned URL expiry), and error recovery via exponential backoff.

## 2. Technical Implementation
The player operates as a stateful engine using Svelte 5 runes (`$state`, `$effect`, `$derived`). It communicates with the UI layer via browser-native CustomEvents to ensure compatibility across Astro's isolated island architecture.

### Audio Engine Configuration
To ensure CORS compliance for byte-range streaming, the following configuration is required on the HTML element:

    <audio
      bind:this={audioElement}
      preload="auto"
      crossorigin="anonymous"
    />

## 3. Communication API
The component exposes its internal state to the rest of the application using the browser `window` object.

### Event Bus
- To Play a Track: 
    Broadcast a `play-track` event containing the track object.
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));

- To Initialize Catalog: 
    Broadcast a `catalog-loaded` event when the catalog is fetched.
    window.dispatchEvent(new CustomEvent('catalog-loaded', { detail: catalog }));

## 4. Key Logic Features
- Proactive URL Refresh: Monitors Presigned URL expiry and fetches a replacement before the current URL becomes invalid.
- Error Recovery: Implements exponential backoff when a streaming request fails (e.g., network jitter).
- Range Request Support: Relies on native browser and R2 integration to handle byte-range requests for instant seeking.
- State Persistence: Uses the `audioElement` lifecycle directly to maintain playback state without re-loading the track when the UI re-renders.

## 5. Component Props
| Prop | Default | Description |
| :--- | :--- | :--- |
| `apiBase` | `''` | The base URL of the Go Auth Proxy. |
| `urlTtlSeconds` | `3600` | TTL of the Presigned URL in seconds. |
| `isAdmin` | `false` | Toggles download capabilities and admin UI. |
| `autoplay` | `false` | If true, plays the first track upon catalog load. |
| `showTracklist` | `true` | Hides the internal playlist if rendered via a separate Catalog viewer. |

## 6. CSS Styling
The component utilizes Tailwind CSS for its primary interface. Custom scrollbar behaviors are handled via the following global style overrides:

    .tracklist::-webkit-scrollbar {
      width: 4px;
    }
    .tracklist::-webkit-scrollbar-track {
      background: transparent;
    }
    .tracklist::-webkit-scrollbar-thumb {
      background: #333;
      border-radius: 2px;
    }


