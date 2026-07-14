// src/lib/catalogStore.ts

declare global {
  interface Window {
    __LABRI_CATALOG__: any[] | undefined;
    __LABRI_CATALOG_PROMISE__: Promise<any[]> | undefined;
  }
}

// ─── Online/Offline Revalidation ───
// When the browser comes back online, check if we have a cached
// catalog and trigger a revalidation. This ensures the catalog
// is fresh after a period of being offline.
if (typeof window !== 'undefined') {
  window.addEventListener('online', () => {
    // Only revalidate if we have a cached catalog to revalidate
    if (localStorage.getItem('catalog_data')) {
      backgroundRevalidate();
    }
  });
}

// --- Your Exact Helper Functions ---
function base64ToArrayBuffer(base64: string): ArrayBuffer {
  const binaryString = window.atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return bytes.buffer;
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  let binary = '';
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return window.btoa(binary);
}

async function inflateCatalog(arrayBuffer: ArrayBuffer): Promise<any> {
  // 1. Try parsing as raw JSON first (in case browser auto-decompressed via Content-Encoding)
  try {
    const text = new TextDecoder().decode(arrayBuffer);
    const data = JSON.parse(text);
    console.log("inflateCatalog: Data was already raw JSON (browser auto-decompressed).");
    return data;
  } catch (e) {
    // It's not valid JSON, so it must be zipped. Continue to step 2.
  }

  // 2. Try to decompress as Gzip
  try {
    const ds = new DecompressionStream('gzip');
    const decompressedStream = new Response(arrayBuffer).body!.pipeThrough(ds);
    const blob = await new Response(decompressedStream).blob();
    const text = await blob.text();
    return JSON.parse(text);
  } catch (gzipError) {
    console.error("inflateCatalog: Data is neither raw JSON nor valid Gzip.", gzipError);
    
    // 3. DEBUGGING: Log the first 4 bytes to see what the data actually is
    const bytes = new Uint8Array(arrayBuffer.slice(0, 4));
    const hex = Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join(' ');
    console.error("inflateCatalog: First 4 bytes (hex):", hex); 
    // If it's gzip, it will start with "1f 8b 08"
    // If it's an HTML error, it might start with "<!DO" (3c 21 44 4f)
    
    throw new Error("Catalog data is corrupted or in an unexpected format.");
  }
}

// ─── Encapsulated Cache I/O ───
// These helpers wrap localStorage so we can migrate to IndexedDB
// later (for PWA / Service Worker compatibility) without changing
// any callers.

async function readCatalogCache(): Promise<{ data: string; version: string } | null> {
  const data = localStorage.getItem('catalog_data');
  const version = localStorage.getItem('catalog_version');
  if (data && version) return { data, version };
  return null;
}

async function writeCatalogCache(data: string, version: string): Promise<void> {
  localStorage.setItem('catalog_data', data);
  localStorage.setItem('catalog_version', version);
}

async function clearCatalogCache(): Promise<void> {
  localStorage.removeItem('catalog_data');
  localStorage.removeItem('catalog_version');
}

// Public getter so the admin page or future SW can read the current version
export function getCatalogVersion(): string {
  return localStorage.getItem('catalog_version') || '';
}


// Inflates a Base64 gzipped string into the catalog array and stores it in memory
async function inflateBase64ToCatalog(base64: string): Promise<any[]> {
  const buffer = base64ToArrayBuffer(base64);
  const data = await inflateCatalog(buffer);
  const catalog = data.tracks || [];
  window.__LABRI_CATALOG__ = catalog; 
  return catalog;
}

/*
   async function inflateCatalog(arrayBuffer) {
    const ds = new DecompressionStream('gzip');
    const decompressedStream = new Response(arrayBuffer).body.pipeThrough(ds);
    const blob = await new Response(decompressedStream).blob();
    const text = await blob.text();
    return JSON.parse(text);
  }
*/

// -----------------------------------

/**
 * Gets the catalog.
 * 1. If already in memory (window), returns it instantly.
 * 2. If currently being fetched/decompressed by another component, waits for that to finish.
 * 3. Otherwise, executes the fetch/decompress pipeline.
 */
export async function getCatalog(): Promise<any[]> {
  // 1. Already inflated and in memory! Return instantly.
  if (window.__LABRI_CATALOG__ && window.__LABRI_CATALOG__.length > 0) {
    return window.__LABRI_CATALOG__;
  }

  // 2. Another component is already fetching/decompressing. Wait for it.
  if (window.__LABRI_CATALOG_PROMISE__) {
    return window.__LABRI_CATALOG_PROMISE__;
  }

  // 3. We are the first to call this. Start the pipeline.
  window.__LABRI_CATALOG_PROMISE__ = loadCatalogInternal();
  
  try {
    //const catalog = await window.__LABRI_CATALOG_PROMISE__;
    //return catalog;
    return await window.__LABRI_CATALOG_PROMISE__;
  } finally {
    // Clear the promise lock whether it succeeded or failed
    window.__LABRI_CATALOG_PROMISE__ = undefined;
  }
}

// ─── getCachedCatalog() ───
// Returns the catalog as fast as possible — from memory, then local
// cache, then server. Unlike getCatalog(), it does NOT block on the
// server check. Instead, it returns the cached version immediately
// and triggers a background revalidation to check for updates.
//
// Callers: user-facing search/display components that prioritize
// perceived performance over absolute freshness.
//
// Admin components (CatalogViewer, UploadManager) should continue
// to use getCatalog() which waits for a fresh server response.

export async function getCachedCatalog(): Promise<{ tracks: any[]; isStale: boolean }> {
  console.log("getCachedCatalog called");
  // 1. Already in memory? Return instantly.
  if (window.__LABRI_CATALOG__ && window.__LABRI_CATALOG__.length > 0) {
    // Fire-and-forget revalidation in the background
    backgroundRevalidate();
    return { tracks: window.__LABRI_CATALOG__, isStale: false };
  }

  // 2. Another component is already fetching. Wait for it.
  if (window.__LABRI_CATALOG_PROMISE__) {
    const tracks = await window.__LABRI_CATALOG_PROMISE__;
    backgroundRevalidate();
    return { tracks, isStale: false };
  }

  // 3. Try local cache first (fast path)
  const cached = await readCatalogCache();
  if (cached) {
    try {
      // Start a new promise for the decompression, but don't assign it
      // to the global promise — we want to return immediately with the
      // cached data, not block other callers.
      const tracks = await inflateBase64ToCatalog(cached.data);
      // Fire-and-forget revalidation
      backgroundRevalidate();
      return { tracks, isStale: true };
    } catch (e) {
      // Cache corrupted, clear it and fall through to server
      console.warn('Local cache corrupted, clearing', e);
      await clearCatalogCache();
    }
  }

  // 4. No cache at all — fall back to the full server fetch (blocking)
  // This is the same path as getCatalog() for cold starts.
  if (!window.__LABRI_CATALOG_PROMISE__) {
    window.__LABRI_CATALOG_PROMISE__ = loadCatalogInternal();
  }
  const tracks = await window.__LABRI_CATALOG_PROMISE__;
  window.__LABRI_CATALOG_PROMISE__ = undefined;
  return { tracks, isStale: false };
}

// ==================================================================================
// loadCatalogInternal()  
// load the catalog into memory. 
// Get it from the closest source that is not stale.
// Input: 
//    No parameters
//    Scans local storage, server cache and R2 storage in that order for a non-stale copy
//
// Output: No return
//    Update "catalog" in memory
//
// Logical Flow
//    Get our stored ETag/Version from local cache / local storage
//    Asks the server if our version is stale - sends our version
//    Response 304 = local version is not stale
//      -> process the local copy
//    Respone !304 = server sends valid compressed copy of catalog with ETag
//      -> saved compressed version & ETAG/Version to local cache / local storage
//      -> process downloaded copy
//    Inflate compress catalog into memory for Svelte to use
// ----------------------------------------------------------------------------------     
// Your exact pipeline, adapted to return data instead of setting a Svelte variable
async function loadCatalogInternal(): Promise<any[]> {
  const clientVersion = localStorage.getItem('catalog_version') || "";
  console.log("catalogStore - clientVersion: ", clientVersion);
  
  try {
    const res = await fetch(`${import.meta.env.PUBLIC_API_BASE_URL}/api/catalog?version=${clientVersion}`);
    console.log("catalogStore - response: ", res  );
    // 304 means our cache is perfectly valid
    if (res.status === 304) {
      const cachedBase64 = localStorage.getItem('catalog_data');
      if (cachedBase64) {
        try {
            //console.log("catalogStore - cachedBase64: ", cachedBase64);
            return await inflateBase64ToCatalog(cachedBase64);
            //const buffer = base64ToArrayBuffer(cachedBase64);
            //console.log("catalogStore - buffer: ", buffer);
            //const data = await inflateCatalog(buffer);
            //console.log("catalogStore - data: ", data);
            //const catalog = data.tracks || [];
            //console.log("catalogStore - catalog: ", catalog);
            //window.__LABRI_CATALOG__ = catalog; // Save to memory
            //return catalog;
        } catch (inflateError){
            // SELF-HEALING: If the cached zip is corrupted, we catch the error here,
            // delete the bad cache, and fall through to fetch a fresh copy.
            console.warn("Local cache corrupted, clearing and re-fetching.", inflateError);
            localStorage.removeItem('catalog_data');
            localStorage.removeItem('catalog_version');
            // go through the process again but now we have
            // an empty cache, so will force a fresh download.
            return await loadCatalogInternal();
        }
      }
    }
    
    if (!res.ok) throw new Error(`Server returned ${res.status}`);

    // New version found (Status 200)
    const arrayBuffer = await res.arrayBuffer();
    const newETag = res.headers.get('ETag');
    // Save to localStorage
    const base64Data = arrayBufferToBase64(arrayBuffer);
    localStorage.setItem('catalog_data', base64Data);
    const versionToStore = newETag || `local-${Date.now()}`;
    localStorage.setItem('catalog_version', versionToStore);

    // USE HELPER (We pass the base64Data we just saved so we
    // don't have to read it back out of localStorage)
    return await inflateBase64ToCatalog(base64Data);
    
  } catch (error) {
    console.error("Catalog load error:", error);
    
    // Network failed, try offline cache
    const cachedBase64 = localStorage.getItem('catalog_data');
    if (cachedBase64) {
      try {
        return await inflateBase64ToCatalog(cachedBase64);
      } catch (e) {
        return []; // Absolute fallback
      }
    }
    return[];
  }
}

// ─── Background Revalidation ───
// Checks the server for a newer catalog version without blocking
// the UI. If a new version is found, it updates the memory cache
// and localStorage. This is fire-and-forget — the caller doesn't
// wait for the result.

let revalidationInProgress = false;

async function backgroundRevalidate(): Promise<void> {
  // Prevent concurrent revalidations
  if (revalidationInProgress) return;
  revalidationInProgress = true;

  try {
    const clientVersion = getCatalogVersion();
    const res = await fetch(
      `${import.meta.env.PUBLIC_API_BASE_URL}/api/catalog?version=${clientVersion}`
    );

    // 304 = our cache is still current
    if (res.status === 304) return;

    // 200 = new version available
    if (res.ok) {
      const arrayBuffer = await res.arrayBuffer();
      const newETag = res.headers.get('ETag') || '';
      const base64Data = arrayBufferToBase64(arrayBuffer);

      // Update localStorage
      await writeCatalogCache(base64Data, newETag);

      // Update in-memory cache so the next call to getCachedCatalog
      // returns the fresh data. We inflate it here but don't return it.
      try {
        await inflateBase64ToCatalog(base64Data);
      } catch (e) {
        console.warn('Background revalidation: failed to inflate new catalog', e);
      }
    }
  } catch (err) {
    // Network error is expected if offline — ignore silently
    // The cached version remains available.
    if (err instanceof TypeError && err.message === 'Failed to fetch') {
      // Offline — do nothing
    } else {
      console.warn('Background revalidation failed:', err);
    }
  } finally {
    revalidationInProgress = false;
  }
}

// Call this if an admin updates the catalog and you need to force a refresh
export function clearCatalogMemoryCache() {
  window.__LABRI_CATALOG__ = undefined;
}
