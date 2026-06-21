<!-- src/components/SearchResults.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { 
    filterTracks, 
    createSearchParamsFromUrl 
  } from '../lib/searchEngine.js';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import type { Track, SearchParams } from '../lib/types';
  import { onMount } from 'svelte';

  let allTracks: Track[] = $state([]);
  let filteredTracks: Track[] = $state([]);

  onMount(async () => {
    try {
      const catalog: Track[] = await getCatalog();
      allTracks = catalog;
    } catch (err) {
      console.error("Failed to load catalog:", err);
      allTracks = [];
    }

    // Initial filter check (for bookmarked URLs)
    applyFilters();

    // Listen for filter updates
    window.addEventListener('search-updated', applyFilters);
  });

  function applyFilters(): void {
    const params: SearchParams = createSearchParamsFromUrl();
    filteredTracks = filterTracks(allTracks, params);
  }
</script>

<SearchResultsDisplay tracks={filteredTracks} />
