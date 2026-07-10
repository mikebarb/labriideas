// src/lib/opfsStore.ts

/**
 * Checks if the browser supports the Origin Private File System.
 */
export function isOpfsSupported(): boolean {
  return typeof navigator !== 'undefined' && 
         !!navigator.storage && 
         typeof navigator.storage.getDirectory === 'function';
}

/**
 * Saves a track's blob to OPFS. 
 * Creates the 'labri-cache' directory if it doesn't exist.
 */
export async function saveTrackToOpfs(hash: string, blob: Blob): Promise<void> {
  if (!isOpfsSupported()) {
    throw new Error('OPFS is not supported in this browser');
  }

  const root = await navigator.storage.getDirectory();
  const dir = await root.getDirectoryHandle('labri-cache', { create: true });
  const fileHandle = await dir.getFileHandle(hash, { create: true });
  const writable = await fileHandle.createWritable();
  await writable.write(blob);
  await writable.close();
}

/**
 * Retrieves a track's blob from OPFS.
 */
export async function getTrackBlob(hash: string): Promise<File | null> {
  if (!isOpfsSupported()) return null;
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle('labri-cache', { create: false });
    const fileHandle = await dir.getFileHandle(hash, { create: false });
    const file = await fileHandle.getFile();
    return file;
  } catch (e) {
    return null;
  }
}

/**
 * Removes a specific track from OPFS.
 */
export async function removeTrackFromOpfs(hash: string): Promise<void> {
  if (!isOpfsSupported()) return;
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle('labri-cache', { create: false });
    await dir.removeEntry(hash);
  } catch (e) {
    console.warn(`[OPFS] Could not remove ${hash}:`, e);
  }
}

/**
 * Checks if a track exists in OPFS.
 */
export async function hasTrack(hash: string): Promise<boolean> {
  if (!isOpfsSupported()) return false;
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle('labri-cache', { create: false });
    await dir.getFileHandle(hash, { create: false });
    return true;
  } catch (e) {
    return false;
  }
}

/**
 * Lists the names (hashes) of all downloaded tracks.
 */
export async function listDownloadedHashes(): Promise<string[]> {
  if (!isOpfsSupported()) return [];
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle('labri-cache', { create: false });
    const hashes: string[] = [];
    // @ts-ignore - async iterator on FileSystemDirectoryHandle
    for await (const [name, handle] of dir.entries()) {
      if (handle.kind === 'file') {
        hashes.push(name);
      }
    }
    return hashes;
  } catch (e) {
    return [];
  }
}

/**
 * Gets an estimate of the storage usage and quota.
 */
export async function getStorageEstimate(): Promise<{ usage: number; quota: number }> {
  if (!isOpfsSupported() || !navigator.storage.estimate) {
    return { usage: 0, quota: 0 };
  }
  const estimate = await navigator.storage.estimate();
  return {
    usage: estimate.usage ?? 0,
    quota: estimate.quota ?? 0,
  };
}

/**
 * Formats bytes into a human-readable string.
 */
export function formatBytes(bytes: number, decimals: number = 2): string {
  if (!+bytes) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}
