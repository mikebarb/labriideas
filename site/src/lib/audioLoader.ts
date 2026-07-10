// src/lib/audioLoader.ts

/**
 * Gets a blob from OPFS. If not found, returns null.
 */
export async function getAudioFromOpfs(hash: string): Promise<Blob | null> {
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle('labri-cache', { create: false });
    const fileHandle = await dir.getFileHandle(hash, { create: false });
    const file = await fileHandle.getFile();
    return file;
  } catch (err) {
    return null; // File doesn't exist
  }
}

/**
 * The "Resolver": Tries to get from OS, then falls back to Network
 */
export async function resolveAudioUrl(hash: string, remoteUrl: string): Promise<string> {
  const cachedBlob = await getAudioFromOpfs(hash);
  
  if (cachedBlob) {
    console.log(`[AudioResolver] Cache Hit: ${hash}`);
    return URL.createObjectURL(cachedBlob);
  }

  console.log(`[AudioResolver] Cache Miss: ${hash}. Fetching from network...`);
  return remoteUrl;
}
