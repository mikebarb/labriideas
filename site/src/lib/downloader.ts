// src/lib/downloader.ts
// 
// NEW FILE: Centralized download functionality.
// 
// Why this exists:
// - The download button will be used in MULTIPLE places: the queue drawer AND the search results.
// - Both places need the same logic: get a presigned URL, fetch as blob, trigger browser download.
// - Previously, this logic lived only in Player.svelte, which made it inaccessible from search.
// 
// What it exports:
// - fetchPresignedUrl: gets a temporary URL from the API for downloading a specific file
// - downloadTrack: handles the full download flow with lifecycle callbacks
// 
// Memory management:
// - The blob URL is revoked immediately after the download is triggered.
// - This frees the in-memory blob so the browser's garbage collector can reclaim it.

import type { Track } from './types';

// ---------------------------------------------------------------------------
// Lifecycle callbacks for downloadTrack
// ---------------------------------------------------------------------------
// Components pass these to receive updates without needing to track the
// underlying XHR themselves. Each callback is optional.
export interface DownloadCallbacks {
  onStart?: () => void;       // Called immediately when download begins
  onComplete?: () => void;    // Called after the file is saved to disk
  onError?: (err: Error) => void;  // Called if the download fails at any stage
}

// ---------------------------------------------------------------------------
// Result of fetching a presigned URL
// ---------------------------------------------------------------------------
// expiresAt is included for future use (e.g., caching URLs until they expire).
export interface PresignedUrlResult {
  url: string;
  expiresAt: number;
}

// ---------------------------------------------------------------------------
// Fetch a presigned download URL from the API
// ---------------------------------------------------------------------------
// The API endpoint returns either:
//   - application/json body: { url: "..." }
//   - plain text body: "https://..."
// This function handles both response formats transparently.
//
// CHANGED: This used to live in Player.svelte. It was moved here so that
// any component (queue drawer, search page, etc.) can use it.
export async function fetchPresignedUrl(
  filename: string,
  apiBase: string
): Promise<PresignedUrlResult> {
  const res = await fetch(`${apiBase}/api/download?file=${encodeURIComponent(filename)}`);
  if (!res.ok) throw new Error(`Failed to get URL: ${res.status}`);

  const contentType = res.headers.get('content-type') ?? '';
  let url: string;

  if (contentType.includes('application/json')) {
    const data = await res.json();
    url = data.url;
  } else {
    url = (await res.text()).trim();
  }

  return { url, expiresAt: Date.now() + 3600 * 1000 };
}

// ---------------------------------------------------------------------------
// Download a track to the user's local Downloads folder
// ---------------------------------------------------------------------------
// Flow:
//   1. Call onStart callback (component can show a spinner)
//   2. Fetch the presigned URL from the API
//   3. Use XMLHttpRequest to download the file as a blob
//   4. Create a temporary <a> element with the download attribute
//   5. Click it to trigger the browser's download manager
//   6. Revoke the blob URL to free memory
//   7. Call onComplete or onError callback
//
// Why XHR instead of fetch():
//   - We use XHR for compatibility with the blob response type.
//   - fetch() + blob() also works, but XHR is the established pattern.
//
// CHANGED: This used to live in Player.svelte as `downloadTrack(track)`. 
// It was moved here so any component can use it. The function now takes 
// apiBase as a parameter (previously it was available via closure in Player).
export function downloadTrack(
  track: Track,
  apiBase: string,
  callbacks: DownloadCallbacks = {}
): void {
  callbacks.onStart?.();

  fetchPresignedUrl(track.filename, apiBase)
    .then((ticket) => {
      const xhr = new XMLHttpRequest();
      xhr.open('GET', ticket.url, true);
      xhr.responseType = 'blob';

      xhr.onload = () => {
        if (xhr.status === 200) {
          const blob = xhr.response;
          const blobUrl = URL.createObjectURL(blob);

          const a = document.createElement('a');
          a.href = blobUrl;
          a.download = track.filename;
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          URL.revokeObjectURL(blobUrl);

          callbacks.onComplete?.();
        } else {
          const err = new Error(`Download failed: ${xhr.status}`);
          callbacks.onError?.(err);
        }
      };

      xhr.onerror = () => {
        const err = new Error('Network error during download');
        callbacks.onError?.(err);
      };

      xhr.send();
    })
    .catch((err) => {
      const errObj = err instanceof Error ? err : new Error(String(err));
      callbacks.onError?.(errObj);
    });
}
