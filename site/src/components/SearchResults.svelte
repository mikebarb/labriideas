<!-- src/components/SearchResults.svelte -->
<script lang="ts">
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { 
    filterTracks, 
    createSearchParamsFromUrl 
  } from '../lib/searchEngine.js';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import type { Track, SearchParams } from '../lib/types';
  import { onMount } from 'svelte';

  interface Props {
    apiBase: string;
  }
  let { apiBase }: Props = $props();

  let allTracks: Track[] = $state([]);
  let filteredTracks: Track[] = $state([]);
  let isLoading = $state(true);

  onMount(async () => {
    try {
      const { tracks: catalog } = await getCachedCatalog();
      allTracks = catalog;
    } catch (err) {
      console.error("Failed to load catalog:", err);
      allTracks = [];
    }finally {
      isLoading = false;
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

{#if isLoading}
  <div class="text-sm text-gray-500 italic">Loading catalog...</div>
{:else}
  <SearchResultsDisplay tracks={filteredTracks} {apiBase} />
{/if}
