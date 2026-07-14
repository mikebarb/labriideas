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
// 
// OPFS Integration:
// - downloadTrack now checks OPFS FIRST. If the track is already cached locally
//   (e.g., the user has played it before), the file is pulled from disk.
// - This means the Download button becomes a "local export" for already-played
//   tracks, saving bandwidth and working even on a flaky network.
// - If we have to fetch from the network, we save the resulting blob to OPFS
//   so subsequent downloads/plays are instant.

import type { Track } from './types';
import { getTrackBlob, saveTrackToOpfs } from './opfsStore.ts';

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
//   2. CHECK OPFS: If the blob is already cached locally, use it.
//   3. FETCH: If not in OPFS, fetch the presigned URL from the API.
//   4. CACHE: Save the fetched blob to OPFS for next time.
//   5. TRIGGER: Create a temporary <a> element with the download attribute.
//   6. Click it to trigger the browser's download manager.
//   7. Revoke the blob URL to free memory.
//   8. Call onComplete or onError callback
//
// Why OPFS first?
//   - If the user has already played the track, the file is on their disk.
//   - The Download button becomes an instant, zero-bandwidth "export to OS".
//   - The app degrades gracefully on bad networks.
//
// CHANGED: Now async. Was previously a sync void return with XHR.
export async function downloadTrack(
  track: Track,
  apiBase: string,
  callbacks: DownloadCallbacks = {}
): Promise<void> {
  callbacks.onStart?.();

  try {
    let blob: Blob | null = null;

    // 1. TRY OPFS FIRST
    if (track.hash) {
      blob = await getTrackBlob(track.hash);
    }

    // 2. IF NOT IN OPFS, FETCH FROM NETWORK AND CACHE IT
    if (!blob) {
      const ticket = await fetchPresignedUrl(track.filename, apiBase);
      const response = await fetch(ticket.url);
      if (!response.ok) throw new Error(`Fetch failed: ${response.status}`);
      blob = await response.blob();
      
      // Save to OPFS for the next download/play
      if (track.hash) {
        try {
          await saveTrackToOpfs(track.hash, blob);
        } catch (cacheErr) {
          // If caching fails, we still proceed with the download.
          console.warn('[Downloader] OPFS cache failed, proceeding anyway:', cacheErr);
        }
      }
    }

    // 3. TRIGGER BROWSER DOWNLOAD
    const blobUrl = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = blobUrl;
    a.download = track.filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(blobUrl);

    callbacks.onComplete?.();
  } catch (err) {
    const errObj = err instanceof Error ? err : new Error(String(err));
    callbacks.onError?.(errObj);
  }
}


/**
 * Background pre-caching. Fetches a track and saves it to OPFS 
 * without playing it. Used for pre-warming the cache.
 */
export async function ensureTrackCached(track: Track): Promise<void> {
  if (!track.hash) return;
  const { getTrackBlob, saveTrackToOpfs } = await import('./opfsStore');
  
  const alreadyCached = await getTrackBlob(track.hash);
  if (alreadyCached) return;

  try {
    const ticket = await fetchPresignedUrl(track.filename, '');
    const response = await fetch(ticket.url);
    if (!response.ok) throw new Error('Fetch failed');
    const blob = await response.blob();
    await saveTrackToOpfs(track.hash, blob);
  } catch (err) {
    console.warn(`[CacheManager] Failed to cache ${track.filename}:`, err);
  }
}
