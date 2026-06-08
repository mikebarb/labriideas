<script>
 // --- Security Placeholder ---
  // Later, Astro will pass true/false here based on the user's JWT cookie.
  // For now, we can default it to true just so we can see the UI while developing.
  export let isAdmin = true;

  import { onMount } from 'svelte';
  import MetadataEditor from '../components/MetadataEditor.svelte';
  import { getCatalog } from '../lib/catalogStore';

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

  // Function to close the metadata editor
  function closeEditor() {
    selectedTrack = null;
  }

  // ================================== loadCatalog ======================================
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
  async function loadCatalog() {
    isLoading = true;
    status = "Checking for catalog updates...";
    
    // Get our stored ETag/Version from local storage / cache
    const clientVersion = localStorage.getItem('catalog_version') || "";
    try{
      // Ask the service for the catalog. 
      // It will either return the memory cache instantly, or run the pipeline.
      const loadedCatalog = await getCatalog();

      catalog = loadedCatalog;
      status = `Catalog loaded (${catalog.length} tracks)`;

    } catch (err) {
      status = "Failed to load catalog.";
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
      <MetadataEditor track={selectedTrack} onClose={closeEditor} />
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
