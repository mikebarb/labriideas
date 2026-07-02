<!-- src/components/RankedSearch.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import { rankedSearch } from '../lib/rankedEngine.js';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import type { Track } from '../lib/types';

  interface Props {
    apiBase: string;
  }
  let { apiBase }: Props = $props();

  let allTracks: Track[] = $state([]);
  let rankedTracks: Track[] = $state([]);
  let query: string = $state('');
  let isLoading = $state(true);

  onMount(async () => {
    try {
      const { tracks } = await getCachedCatalog();
      allTracks = tracks;
      rankedTracks = tracks;
    } catch (err) {
      console.error('Failed to load catalog:', err);
      allTracks = [];
      rankedTracks = [];
    }finally {
      isLoading = false;
    }
  });

  function handleSearch(e: Event): void {
    const target = e.target as HTMLInputElement;
    query = target.value;
    rankedTracks = rankedSearch(allTracks, query);
  }
</script>
{#if isLoading}
  <div class="text-sm text-gray-500 italic">Loading catalog...</div>
{:else}
  <div class="p-4">
    <h1 class="text-xl font-bold mb-4">Spotlight Search (Ranked)</h1>
    <input 
      type="text" 
      oninput={handleSearch} 
      placeholder="Search anything..." 
      class="w-full border p-2 mb-4"
      value={query}
    />
    <SearchResultsDisplay tracks={rankedTracks} {apiBase}/>
  </div>
{/if}