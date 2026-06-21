<!-- src/components/SearchResults.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { fuzzyFilter } from '../lib/fuzzySearch.js';
  import { onMount } from 'svelte';

  let allTracks: any[] = $state([]);
  let filteredTracks: any[] = $state([]);
  let expandedFilename: string | null = $state(null);

  onMount(async () => {
    try {
      allTracks = await getCatalog();
    } catch (err) {
      console.error("Failed to load catalog:", err);
      allTracks = [];
    }
    
    applyFilters(); 
    window.addEventListener('search-updated', applyFilters);
  });

  // Helper: Normalize keywords to always be an array of strings
  function getKeywords(track: any): string[] {
    if (Array.isArray(track.keywords)) {
      return track.keywords.filter((k: any) => typeof k === 'string');
    }
    if (typeof track.keywords === 'string' && track.keywords.trim() !== '') {
      return track.keywords.split(',').map((k: string) => k.trim()).filter(Boolean);
    }
    return [];
  }

  function applyFilters() {
    const params = new URLSearchParams(window.location.search);
    const title = (params.get('title') || '').trim();
    const speaker = (params.get('speaker') || '').trim();
    const category = (params.get('category') || '').trim();
    const keywords = (params.get('keywords') || '').split(',').filter(Boolean);
    const isFuzzy = params.get('fuzzy') === 'true';

    // Stage 1: Apply fuzzy search to the Title field (if enabled)
    let workingSet = allTracks;
    if (isFuzzy && title) {
      workingSet = fuzzyFilter(workingSet, title, 'title');
    } else if (title) {
      workingSet = workingSet.filter((track: any) => 
        (track.title || '').toLowerCase().includes(title.toLowerCase()) ||
        (track.filename || '').toLowerCase().includes(title.toLowerCase())
      );
    }

    // Stage 2: Apply remaining filters (AND logic)
    filteredTracks = workingSet.filter((track: any) => {
      if (speaker && track.speaker !== speaker) {
        return false;
      }

      if (category) {
        const trackCategory = (track.category || '').toLowerCase();
        const matches = Array.isArray(track.category) 
          ? track.category.map((c: string) => c.toLowerCase()).includes(category.toLowerCase())
          : trackCategory === category.toLowerCase();
        if (!matches) return false;
      }

      if (keywords.length > 0) {
        const trackKeywords = getKeywords(track).map((k: string) => k.toLowerCase());
        const allMatch = keywords.every((kw: string) => 
          trackKeywords.some((tk: string) => tk.includes(kw.toLowerCase()))
        );
        if (!allMatch) return false;
      }

      return true;
    });

    // Stage 3: Sort alphabetically
    filteredTracks.sort((a: any, b: any) => {
      const titleA = (a.title || a.filename || '').toLowerCase();
      const titleB = (b.title || b.filename || '').toLowerCase();
      return titleA.localeCompare(titleB);
    });
  }

  function toggle(filename: string) {
    expandedFilename = expandedFilename === filename ? null : filename;
  }

  function play(track: any) {
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }
</script>

<div class="space-y-2">
  {#if allTracks.length === 0}
    <p class="text-gray-500 italic py-4">Loading catalog...</p>
  {:else if filteredTracks.length === 0}
    <div class="text-center py-8">
      <p class="text-gray-500 italic">No tracks match your filters.</p>
    </div>
  {:else}
    <p class="text-sm text-gray-500 mb-4">
      {filteredTracks.length} {filteredTracks.length === 1 ? 'track' : 'tracks'} found
    </p>
    
    {#each filteredTracks as track (track.filename)}
      <div class="border rounded transition-all {expandedFilename === track.filename ? 'bg-gray-50' : 'bg-white'}">
        <button 
          class="w-full text-left p-4 flex justify-between items-center min-h-11"
          onclick={() => toggle(track.filename)}
        >
          <div class="truncate flex-1">
            <p class="font-medium text-gray-900">{track.title || track.filename}</p>
            <p class="text-sm text-gray-500">{track.speaker}</p>
          </div>
          <span class="text-gray-400 ml-2">{expandedFilename === track.filename ? '▼' : '▶'}</span>
        </button>

        {#if expandedFilename === track.filename}
          <div class="px-4 pb-4 border-t border-gray-200 pt-4">
            <!-- Category tag -->
            {#if track.category}
              <div class="mb-3">
                <span class="inline-block text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded">
                  {track.category}
                </span>
              </div>
            {/if}

            <!-- Keyword pills (using normalized array) -->
            {#if getKeywords(track).length > 0}
              <div class="mb-3 flex flex-wrap gap-1">
                {#each getKeywords(track) as kw (kw)}
                  <span class="text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded border border-gray-200">
                    #{kw}
                  </span>
                {/each}
              </div>
            {/if}

            <!-- Play button -->
            <div class="flex justify-end">
              <button 
                onclick={() => play(track)}
                class="bg-green-600 hover:bg-green-700 text-white px-6 py-2 rounded-lg font-bold transition"
              >
                ▶ Play Track
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
