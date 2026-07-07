<!-- src/components/SearchFilter3.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { getNarrowedPool, mergeOptions, type MultiSearchParams } from '../lib/searchEngine.js';
  import FilterPopover from './FilterPopover.svelte';
  import type { Track } from '../lib/types';

  let allTracks: Track[] = $state([]);
  let isStale = $state(false);
  let isLoading = $state(true);
  
  let query: string = $state('');
  let selectedSpeakers: string[] = $state([]);
  let selectedCategories: string[] = $state([]);
  let selectedKeywords: string[] = $state([]);

  // ─── V3 Configuration Toggle ───────────────────────────────────
  // Switch this to 'stale-at-top' later if user feedback indicates 
  // they want stale items separated from available ones.
  const SORT_MODE = 'alphabetical'; 
  // ───────────────────────────────────────────────────────────────

  // Reactive parent constraint object
  let currentParams: MultiSearchParams = $derived.by(() => {
    return {
      query,
      speakers: selectedSpeakers,
      categories: selectedCategories,
      keywords: selectedKeywords
    };
  });

  // V3 Display Logic: Build the { value, isStale }[] lists inline
  // We don't need to hold the raw pools in the top-level scope.
  let speakerOptions: { value: string; isStale: boolean }[] = $derived.by(() => {
    return mergeOptions(
      getNarrowedPool(allTracks, currentParams, 'speakers'), 
      selectedSpeakers
    );
  });

  let categoryOptions: { value: string; isStale: boolean }[] = $derived.by(() => {
    return mergeOptions(
      getNarrowedPool(allTracks, currentParams, 'categories'), 
      selectedCategories
    );
  });

  let keywordOptions: { value: string; isStale: boolean }[] = $derived.by(() => {
    return mergeOptions(
      getNarrowedPool(allTracks, currentParams, 'keywords'), 
      selectedKeywords
    );
  });

  async function loadCatalog() {
    try {
      const { tracks, isStale: stale } = await getCachedCatalog();
      allTracks = tracks;
      isStale = stale;
    } catch (e) {
      console.error('Failed to load catalog', e);
    } finally {
      isLoading = false;
      hydrateFromUrl();
      publishState();
    }
  }

  function hydrateFromUrl() {
    if (typeof window === 'undefined') return;
    const params = new URLSearchParams(window.location.search);
    
    const q = (params.get('q') || '').trim();
    if (q) query = q;
    
    const sp = params.get('speaker');
    if (sp) selectedSpeakers = sp.split(',').map(s => s.trim()).filter(Boolean);
    
    const cat = params.get('cat');
    if (cat) selectedCategories = cat.split(',').map(s => s.trim()).filter(Boolean);
    
    const kw = params.get('kw');
    if (kw) selectedKeywords = kw.split(',').map(s => s.trim()).filter(Boolean);
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

    window.dispatchEvent(new CustomEvent('search3-updated', {
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
      ? selectedSpeakers.filter(x => x !== s) 
      : [...selectedSpeakers, s];
    publishState();
  }

  function toggleCategory(c: string) {
    selectedCategories = selectedCategories.includes(c) 
      ? selectedCategories.filter(x => x !== c) 
      : [...selectedCategories, c];
    publishState();
  }

  function toggleKeyword(k: string) {
    selectedKeywords = selectedKeywords.includes(k) 
      ? selectedKeywords.filter(x => x !== k) 
      : [...selectedKeywords, k];
    publishState();
  }
</script>

<div class="space-y-3">
  {#if isLoading}
    <div class="text-sm text-gray-500 italic">Loading catalog...</div>
  {:else}
    <input
      type="text"
      value={query}
      oninput={(e) => {
        query = e.currentTarget.value;
        publishState();
      }}
      placeholder="Search anything... (then refine below)"
      class="w-full border-2 rounded p-2 text-base"
    />

    <div class="flex flex-wrap gap-2">
      <FilterPopover
        label="Speaker"
        options={speakerOptions}
        selected={selectedSpeakers}
        onToggle={toggleSpeaker}
        color="bg-blue-100"
      />
      <FilterPopover
        label="Category"
        options={categoryOptions}
        selected={selectedCategories}
        onToggle={toggleCategory}
        color="bg-purple-100"
      />
      <FilterPopover
        label="Keyword"
        options={keywordOptions}
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
