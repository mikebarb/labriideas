// src/lib/catalogStore.ts

declare global {
  interface Window {
    __LABRI_CATALOG__: any[] | undefined;
    __LABRI_CATALOG_PROMISE__: Promise<any[]> | undefined;
  }
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

async function inflateCatalog_old(arrayBuffer: ArrayBuffer): Promise<any> {
  console.log("inflateCatalog - arrayBuffer: ", arrayBuffer);
  const ds = new DecompressionStream('gzip');
  console.log("inflateCatalog - ds: ", ds);
  const decompressedStream = new Response(arrayBuffer).body!.pipeThrough(ds);
  console.log("inflateCatalog - decompressedStream: ", decompressedStream);
  const blob = await new Response(decompressedStream).blob();
  console.log("inflateCatalog - blob: ", blob);
  
  const text = await blob.text();
  console.log("inflateCatalog - text: ", text);
  return JSON.parse(text);
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
    const res = await fetch(`http://localhost:8080/api/catalog?version=${clientVersion}`);
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
    localStorage.setItem('catalog_version', newETag);

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

// Call this if an admin updates the catalog and you need to force a refresh
export function clearCatalogMemoryCache() {
  window.__LABRI_CATALOG__ = undefined;
}
