<!-- src/components/SearchResults3.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { filterTracksMulti } from '../lib/searchEngine.ts';
  import { isAdminStore } from '../lib/playerStore.js';
  import SearchResultsDisplay from './SearchResultsDisplay.svelte';
  import type { Track } from '../lib/types';

  interface Props {
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
    window.addEventListener('search3-updated', handleUpdate as EventListener);
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
      hydrateFromUrl();
      applyLogic();
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


  function applyLogic() {
    filteredTracks = filterTracksMulti(allTracks, {
      query,
      speakers: selectedSpeakers,
      categories: selectedCategories,
      keywords: selectedKeywords
    });
  }
</script>

{#if isLoading}
  <div class="text-center py-8">
    <div class="text-sm text-gray-500 italic">Loading catalog...</div>
  </div>
{:else}
  <SearchResultsDisplay tracks={filteredTracks} {apiBase} isAdmin={$isAdminStore} />
{/if}
