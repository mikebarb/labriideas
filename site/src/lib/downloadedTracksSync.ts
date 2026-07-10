// src/lib/downloadedTracksSync.ts

import { listDownloadedHashes, type DownloadedTrack } from './opfsStore.ts';
import { downloadedTracksStore } from './playerStore.ts';
import { getCachedCatalog } from './catalogStore.ts';
import { getStorageEstimate } from './opfsStore.ts';

/**
 * Reconcile the in-memory downloaded tracks list with OPFS.
 * Call this:
 *   - On app startup
 *   - After a download completes
 *   - After a track is removed
 *   - When the user comes back online (in case of sync issues)
 *
 * The function reads all hashes from OPFS, looks up the corresponding
 * track metadata in the catalogue, and updates the reactive store.
 */
export async function syncDownloadedTracks(): Promise<DownloadedTrack[]> {
  try {
    const hashes = await listDownloadedHashes();
    if (hashes.length === 0) {
      downloadedTracksStore.set([]);
      return [];
    }

    // Get catalogue for metadata lookup
    const { tracks } = await getCachedCatalog();
    const trackByHash = new Map<string, any>();
    for (const t of tracks) {
      if (t.hash) trackByHash.set(t.hash, t);
    }

    // Build the synced list
    const synced: DownloadedTrack[] = [];
    for (const hash of hashes) {
      const meta = trackByHash.get(hash);
      synced.push({
        hash,
        title: meta?.title ?? 'Unknown Title',
        filename: meta?.filename ?? hash,
        downloadedAt: 0,  // OPFS doesn't track this; could store in a sidecar
        sizeBytes: 0,     // Could be calculated by reading the file
        extension: 'mp3'
      });
    }

    downloadedTracksStore.set(synced);
    return synced;
  } catch (err) {
    console.error('[syncDownloadedTracks] failed:', err);
    return [];
  }
}

/**
 * Check if a specific track is downloaded (reactive query).
 * Used by components that need to show a "downloaded" indicator.
 */
export function isTrackDownloaded(hash: string | undefined): boolean {
  if (!hash) return false;
  let result = false;
  downloadedTracksStore.subscribe(list => {
    result = list.some(t => t.hash === hash);
  })();
  return result;
}
