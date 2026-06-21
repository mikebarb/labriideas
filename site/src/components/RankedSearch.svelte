<!-- src/components/RankedSearch.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import { rankedSearch } from '../lib/rankedEngine.js';
  import { getCatalog } from '../lib/catalogStore.js';
  import type { Track } from '../lib/types';

  let allTracks: Track[] = $state([]);
  let rankedTracks: Track[] = $state([]);
  let query: string = $state('');

  onMount(async () => {
    try {
      const catalog: Track[] = await getCatalog();
      allTracks = catalog;
      rankedTracks = catalog;
    } catch (err) {
      console.error('Failed to load catalog:', err);
      allTracks = [];
      rankedTracks = [];
    }
  });

  function handleSearch(e: Event): void {
    const target = e.target as HTMLInputElement;
    query = target.value;
    rankedTracks = rankedSearch(allTracks, query);
  }
</script>

<div class="p-4">
  <h1 class="text-xl font-bold mb-4">Spotlight Search (Ranked)</h1>
  <input 
    type="text" 
    oninput={handleSearch} 
    placeholder="Search anything..." 
    class="w-full border p-2 mb-4"
    value={query}
  />
  <SearchResultsDisplay tracks={rankedTracks} />
</div>
