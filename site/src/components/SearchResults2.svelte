<!-- src/components/SearchResults2.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { rankedSearch } from '../lib/rankedEngine.js';
  import { sanitizeKeywords } from '../lib/dataUtils.js';
  import { isAdminStore } from '../lib/playerStore.js';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import type { Track } from '../lib/types';

  interface Props {
    // CHANGED: Added apiBase prop. This is passed down to TrackCard 
    // so the download button can call the download library.
    apiBase: string;
  }

  let { apiBase }: Props = $props();

  let allTracks: Track[] = $state([]);
  let isLoading = $state(true);
  let isStale = $state(false);
  let filteredTracks: Track[] = $state([]);
  let query: string = $state('');
  let selectedSpeakers: string[] = $state([]);
  let selectedCategories: string[] = $state([]);
  let selectedKeywords: string[] = $state([]);

  onMount(() => {
    loadCatalog();
    window.addEventListener('search2-updated', handleUpdate as EventListener);
  });

  function handleUpdate(event: Event) {
    const e = event as CustomEvent;
    query = e.detail.query;
    selectedSpeakers = e.detail.speakers;
    selectedCategories = e.detail.categories;
    selectedKeywords = e.detail.keywords;
    applyLogic();
  }

 async function loadCatalog() {
    try {
      const { tracks, isStale: stale } = await getCachedCatalog();
      allTracks = tracks;
      isStale = stale;
    } catch (e) {
      console.error('Failed to load catalog', e);
    } finally {
      isLoading = false;
      applyLogic(); // Ensure we apply the filter logic after loading the catalog
    }
  }

  function applyLogic() {
    // 1. THE SIEVE (Hard Filtering - OR within, AND between)
    let results = allTracks.filter(track => {
      // Speaker: OR logic
      const matchSpeaker = selectedSpeakers.length === 0 || 
                           selectedSpeakers.includes(track.speaker || '');
      
      // Category: OR logic
      const trackCats = Array.isArray(track.category) ? track.category : (track.category ? [track.category] : []);
      const matchCategory = selectedCategories.length === 0 || 
                            trackCats.some(c => selectedCategories.includes(c));
      
      // Keyword: OR logic
      const trackKw = sanitizeKeywords(track.keywords);
      const matchKeyword = selectedKeywords.length === 0 || 
                           selectedKeywords.some(kw => trackKw.includes(kw.toLowerCase()));

      // AND between the three filter types
      return matchSpeaker && matchCategory && matchKeyword;
    });

    // 2. THE RANKER (Weighted Search)
    if (query.trim()) {
      results = rankedSearch(results, query);
    } else {
      // Default alphabetical sort when no query
      results.sort((a, b) => (a.title || a.filename).localeCompare(b.title || b.filename));
    }

    filteredTracks = results;
  }

</script>
<!--
//  Pass apiBase and $isAdminStore to SearchResultsDisplay.
//  $isAdminStore comes from the player store (set by Player.svelte 
//  when the isAdmin prop is true). This means the download button 
//  automatically appears/hides based on the admin status passed to Player.
-->
{#if isLoading}
  <div class="text-center py-8">
    <div class="text-sm text-gray-500 italic">Loading catalog...</div>
  </div>
{:else}
  <SearchResultsDisplay tracks={filteredTracks} {apiBase} isAdmin={$isAdminStore} />
{/if}
