<!-- src/components/SearchFilter2.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { sanitizeKeywords } from '../lib/dataUtils.js';  // ← ADD THIS
  import FilterPopover from './FilterPopover.svelte';
  import type { Track } from '../lib/types';

  let allTracks: Track[] = $state([]);
  let isStale = $state(false);
  let isLoading = $state(true);
  
  let query: string = $state('');
  let selectedSpeakers: string[] = $state([]);
  let selectedCategories: string[] = $state([]);
  let selectedKeywords: string[] = $state([]);

  // Derive unique options from the catalog
  let uniqueSpeakers: string[] = $derived.by(() => {
    const set = new Set<string>();
    allTracks.forEach((t: Track) => {
      if (t.speaker) set.add(t.speaker);
    });
    return Array.from(set).sort();
  });

  let uniqueCategories: string[] = $derived.by(() => {
    const set = new Set<string>();
    allTracks.forEach((t: Track) => {
      if (Array.isArray(t.category)) {
        t.category.forEach((c: string) => set.add(c));
      } else if (t.category) {
        set.add(t.category);
      }
    });
    return Array.from(set).sort();
  });

  // FIXED: Use sanitizeKeywords instead of manual string/array check
  let uniqueKeywords: string[] = $derived.by(() => {
    const set = new Set<string>();
    allTracks.forEach((t: Track) => {
      const cleanKeywords = sanitizeKeywords(t.keywords); // ← RETURNS string[]
      cleanKeywords.forEach((k: string) => set.add(k));
    });
    return Array.from(set).filter((k: string) => k.length >= 2).sort();
  });

  // loadCatalog function using getCachedCatalog
  // Returns immediately if cache exists (decompressing on the fly),
  // then triggers a background revalidation to check for updates.
  async function loadCatalog() {
    try {
      const { tracks, isStale: stale } = await getCachedCatalog();
      allTracks = tracks;
      isStale = stale;
    } catch (e) {
      console.error('Failed to load catalog', e);
    } finally {
      isLoading = false;
    }
  }

  onMount(() => {
    loadCatalog();
  });

  function publishState() {
    const params = new URLSearchParams();
    if (query) params.set('q', query);
    if (selectedSpeakers.length) params.set('speaker', selectedSpeakers.join(','));
    if (selectedCategories.length) params.set('cat', selectedCategories.join(','));
    if (selectedKeywords.length) params.set('kw', selectedKeywords.join(','));
    
    const newUrl = params.toString() 
      ? `${window.location.pathname}?${params.toString()}` 
      : window.location.pathname;
    window.history.replaceState({}, '', newUrl);

    window.dispatchEvent(new CustomEvent('search2-updated', {
      detail: {
        query,
        speakers: selectedSpeakers,
        categories: selectedCategories,
        keywords: selectedKeywords
      }
    }));
  }

  function clearAll() {
    query = '';
    selectedSpeakers = [];
    selectedCategories = [];
    selectedKeywords = [];
    publishState();
  }

  function toggleSpeaker(s: string) {
    selectedSpeakers = selectedSpeakers.includes(s) 
      ? selectedSpeakers.filter((x: string) => x !== s) 
      : [...selectedSpeakers, s];
    publishState();
  }

  function toggleCategory(c: string) {
    selectedCategories = selectedCategories.includes(c) 
      ? selectedCategories.filter((x: string) => x !== c) 
      : [...selectedCategories, c];
    publishState();
  }

  function toggleKeyword(k: string) {
    selectedKeywords = selectedKeywords.includes(k) 
      ? selectedKeywords.filter((x: string) => x !== k) 
      : [...selectedKeywords, k];
    publishState();
  }
</script>

<div class="space-y-3">
  <!--
    Loading state. The cached catalog decompresses in ~50ms,
    but we show a brief indicator so users with no cache see feedback
    during the initial fetch.
  -->
  {#if isLoading}
    <div class="text-sm text-gray-500 italic">Loading catalog...</div>
  {:else}
    <input
      type="text"
      bind:value={query}
      oninput={publishState}
      placeholder="Search anything... (then refine below)"
      class="w-full border-2 rounded p-2 text-base"
    />

    <div class="flex flex-wrap gap-2">
      <FilterPopover
        label="Speaker"
        options={uniqueSpeakers}
        selected={selectedSpeakers}
        onToggle={toggleSpeaker}
        color="bg-blue-100"
      />
      <FilterPopover
        label="Category"
        options={uniqueCategories}
        selected={selectedCategories}
        onToggle={toggleCategory}
        color="bg-purple-100"
      />
      <FilterPopover
        label="Keyword"
        options={uniqueKeywords}
        selected={selectedKeywords}
        onToggle={toggleKeyword}
        color="bg-orange-100"
      />
      
      {#if query || selectedSpeakers.length || selectedCategories.length || selectedKeywords.length}
        <button
          type="button"
          onclick={clearAll}
          class="text-sm text-gray-500 hover:text-red-600 underline self-center"
        >
          Clear all
        </button>
      {/if}
    </div>
    {#if isStale}
      <div class="text-xs text-gray-400">Updating catalog in background...</div>
    {/if}
  {/if}
</div>
