<script>
 // --- Security Placeholder ---
  // Later, Astro will pass true/false here based on the user's JWT cookie.
  // For now, we can default it to true just so we can see the UI while developing.
  export let isAdmin = true;

  import { onMount } from 'svelte';
  import MetadataEditor from '../components/MetadataEditor.svelte';
  
  let selectedTrack = null;

  // Catalog here is a list of tracks, not the json calalog.json content.
  let catalog = []; 
  let status = "Checking for updates...";  
  let isLoading = true;

  let currentPage = 1;
  const itemsPerPage = 5; 

  $: totalPages = Math.ceil(catalog.length / itemsPerPage);
  $: paginatedCatalog = catalog.slice(
    (currentPage - 1) * itemsPerPage,
    currentPage * itemsPerPage
  );

  function nextPage() { if (currentPage < totalPages) currentPage += 1; }
  function prevPage() { if (currentPage > 1) currentPage -= 1; }

 // Helper: Convert Base64 string back to ArrayBuffer
  function base64ToArrayBuffer(base64) {
    const binaryString = window.atob(base64);
    const bytes = new Uint8Array(binaryString.length);
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i);
    }
    return bytes.buffer;
  }
  
  // Helper: Convert ArrayBuffer to Base64 string for localStorage
  function arrayBufferToBase64(buffer) {
    let binary = '';
    const bytes = new Uint8Array(buffer);
    for (let i = 0; i < bytes.byteLength; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return window.btoa(binary);
  }

   // The "Inflator": Decompresses Gzip and Parses JSON
  async function inflateCatalog(arrayBuffer) {
    const ds = new DecompressionStream('gzip');
    const decompressedStream = new Response(arrayBuffer).body.pipeThrough(ds);
    const blob = await new Response(decompressedStream).blob();
    const text = await blob.text();
    return JSON.parse(text);
  }

  async function loadCatalog() {
    isLoading = true;
    status = "Checking for catalog updates...";
    
    // Get our current ETag/Version from cache
    const clientVersion = localStorage.getItem('catalog_version') || "";
    
    try {
      const res = await fetch(`http://localhost:8080/api/catalog?version=${clientVersion}`);
      
      // 304 means our cache is perfectly valid
      if (res.status === 304) {
        status = "Catalog is up to date (Loaded from cache).";
        // Inflate from local storage
        const cachedBase64 = localStorage.getItem('catalog_data');
        if (cachedBase64) {
          const buffer = base64ToArrayBuffer(cachedBase64);
          const data = await inflateCatalog(buffer);
          catalog = data.tracks || [];
          console.log("CatalogViewer - catalog: ", catalog);
        }
        isLoading = false;
        return;
      }
      
      if (!res.ok) throw new Error(`Server returned ${res.status}`);

      status = "New version found. Downloading compressed catalog...";
      
      // 1. Get the raw binary Gzip data and the new ETag
      const arrayBuffer = await res.arrayBuffer();
      const newETag = res.headers.get('ETag');

      // 2. Save compressed to localStorage (as Base64)
      const base64Data = arrayBufferToBase64(arrayBuffer);
      localStorage.setItem('catalog_data', base64Data);
      localStorage.setItem('catalog_version', newETag);

      // 3. Inflate into memory for Svelte to use
      status = "Decompressing catalog...";
      const data = await inflateCatalog(arrayBuffer);
      catalog = data.tracks || [];
      
      status = `Catalog updated to ETag ${newETag}! (${catalog.length} tracks)`;
      
    } catch (error) {
      console.error("Catalog load error:", error);
      status = "Error. Trying cache...";
      
      const cachedBase64 = localStorage.getItem('catalog_data');
      if (cachedBase64) {
        const buffer = base64ToArrayBuffer(cachedBase64);
        const data = await inflateCatalog(buffer);
        catalog = data.tracks || [];
        status = "Loaded from cache (Offline mode).";
      } else {
        status = "Failed to load catalog.";
      }
    } finally {
      isLoading = false;
    }
  }

  onMount(() => {
    loadCatalog();
  });
</script>

<div class="text-white p-6 bg-slate-800 rounded-lg border border-slate-700 shadow-xl max-w-2xl mx-auto">
  <h2 class="text-2xl font-bold text-cyan-400 mb-4">R2 Catalog</h2>
  
  <p class="text-sm text-slate-300 mb-6">{status}</p>
  
  <!-- Loading Spinner -->
  {#if isLoading}
    <div class="text-center py-8 text-slate-400">Loading...</div>
  {:else if catalog.length === 0}
    <div class="text-center py-8 text-slate-400">No tracks found.</div>
  {:else}
    <!-- We loop over 'paginatedCatalog' instead of 'catalog' -->
    <ul class="space-y-2 mb-6">
      {#each paginatedCatalog as track}
        <!-- <li class="bg-slate-900 p-3 rounded border border-slate-600 flex justify-between"> -->
        <div class="flex justify-between p-2 bg-slate-700 rounded">
          <span class="text-white">{track.filename}</span>
          <span class="text-slate-400">{track.title}</span>
          <span class="text-slate-400">{track.artist}</span>
          <!-- 
          <button 
            on:click={() => currentTrack = track} 
            class="bg-cyan-500 hover:bg-cyan-400 px-3 py-1 rounded text-black font-bold">
            ▶ Play
          </button>
          -->
          <button 
            on:click={() => window.dispatchEvent(new CustomEvent('play-track', { detail: track }))} 
            class="bg-cyan-500 hover:bg-cyan-400 px-3 py-1 rounded text-black font-bold">
              ▶ Play
          </button>
          {#if isAdmin}
            <button on:click={() => selectedTrack = track} class="bg-slate-600 hover:bg-slate-500 px-3 py-1 rounded text-white">
              edit
            </button>
          {/if}
        </div>
        <!-- </li> -->
      {/each}
    </ul>
  
    {#if isAdmin}
      <MetadataEditor track={selectedTrack} />
    {/if}

    <!-- PAGINATION CONTROLS -->
    <div class="flex items-center justify-between bg-slate-900 p-3 rounded border border-slate-600">
      <button 
        on:click={prevPage} 
        disabled={currentPage === 1}
        class="px-4 py-2 bg-slate-700 rounded disabled:opacity-40 hover:bg-slate-600 transition-colors text-sm font-semibold"
      >
        ← Previous
      </button>

      <span class="text-slate-300 text-sm">
        Page {currentPage} of {totalPages} ({catalog.length} total tracks)
      </span>

      <button 
        on:click={nextPage} 
        disabled={currentPage === totalPages}
        class="px-4 py-2 bg-slate-700 rounded disabled:opacity-40 hover:bg-slate-600 transition-colors text-sm font-semibold"
      >
        Next →
      </button>
    </div>
  {/if}
</div>
