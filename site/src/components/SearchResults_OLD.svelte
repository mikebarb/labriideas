<!-- src/components/SearchResults.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
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

  function applyFilters() {
    const params = new URLSearchParams(window.location.search);
    const title = (params.get('title') || '').toLowerCase().trim();
    const speaker = (params.get('speaker') || '').toLowerCase().trim();
    const category = (params.get('category') || '').toLowerCase().trim();
    const keywords = (params.get('keywords') || '').toLowerCase().trim().split(',').filter(Boolean);

    filteredTracks = allTracks.filter((track: any) => {
      if (title && !(track.title || '').toLowerCase().includes(title) && !(track.filename || '').toLowerCase().includes(title)) {
        return false;
      }

      if (speaker && !(track.speaker || '').toLowerCase().includes(speaker)) {
        return false;
      }

      if (category) {
        const trackCategory = (track.category || '').toLowerCase();
        const matches = Array.isArray(track.category) 
          ? track.category.map((c: string) => c.toLowerCase()).includes(category)
          : trackCategory === category;
        if (!matches) return false;
      }

      if (keywords.length > 0) {
        const trackKeywords = (track.keywords || []).map((k: string) => k.toLowerCase());
        for (const kw of keywords) {
          if (!trackKeywords.some((k: string) => k.includes(kw))) {
            return false;
          }
        }
      }

      return true;
    });

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
            <div class="flex items-center justify-between">
              <div class="flex-1">
                {#if track.category}
                  <span class="inline-block text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded mr-2">
                    {track.category}
                  </span>
                {/if}
              </div>
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
