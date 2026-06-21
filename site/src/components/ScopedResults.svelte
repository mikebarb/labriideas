<!-- src/components/ScopedResults.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCatalog } from '../lib/catalogStore.js';
  import { applyIntents, type SearchIntent } from '../lib/intentRouter.js';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import type { Track } from '../lib/types';

  let allTracks: Track[] = $state([]);
  let filteredTracks: Track[] = $state([]);
  let activeIntents: SearchIntent[] = $state([]);

  onMount(async () => {
    try {
      const catalog: Track[] = await getCatalog();
      allTracks = catalog;
    } catch (err) {
      console.error('Failed to load catalog:', err);
      allTracks = [];
    }

    // Initial URL check
    const params = new URLSearchParams(window.location.search);
    const paramData = params.get('filters');
    if (paramData) {
      try {
        activeIntents = JSON.parse(decodeURIComponent(paramData));
      } catch (e) {
        console.error('Failed to parse URL filters:', e);
      }
    }
    applyFilter();

    // Listen for filter updates
    window.addEventListener('scoped-search-updated', handleSearchUpdate as EventListener);
  });

  function handleSearchUpdate(event: Event): void {
    const customEvent = event as CustomEvent<{ intents: SearchIntent[] }>;
    activeIntents = customEvent.detail.intents;
    applyFilter();
  }

  function applyFilter(): void {
    filteredTracks = applyIntents(allTracks, activeIntents);
    filteredTracks.sort((a, b) => {
      const titleA = (a.title || a.filename).toLowerCase();
      const titleB = (b.title || b.filename).toLowerCase();
      return titleA.localeCompare(titleB);
    });
  }
</script>

<SearchResultsDisplay tracks={filteredTracks} />
