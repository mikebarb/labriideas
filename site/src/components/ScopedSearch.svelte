<!-- src/components/ScopedSearch.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getCatalog } from '../lib/catalogStore.js';
  import { findIntents, type SearchIntent } from '../lib/intentRouter.js';
  import type { Track } from '../lib/types';

  let allTracks: Track[] = $state([]);
  let query: string = $state('');
  let suggestions: SearchIntent[] = $state([]);
  let activeIntents: SearchIntent[] = $state([]);

  onMount(async () => {
    try {
      const catalog: Track[] = await getCatalog();
      allTracks = catalog;
    } catch (err) {
      console.error('Failed to load catalog:', err);
    }
  });

  // Reactive: Suggest new intents based on query
  $effect(() => {
    if (query.length > 0 && allTracks.length > 0) {
      suggestions = findIntents(allTracks, query);
    } else {
      suggestions = [];
    }
  });

  // When user clicks a suggestion, add it to the active filters
  function selectIntent(intent: SearchIntent): void {
    const exists = activeIntents.some(i => 
      i.field === intent.field && i.value === intent.value
    );
    if (!exists) {
      activeIntents = [...activeIntents, intent];
    }
    query = '';
    suggestions = [];
    publishState();
  }

  function removeIntent(intent: SearchIntent): void {
    activeIntents = activeIntents.filter(i => 
      !(i.field === intent.field && i.value === intent.value)
    );
    publishState();
  }

  function clearAll(): void {
    activeIntents = [];
    publishState();
  }

  // The Bridge: Tell ScopedResults what to filter by
  function publishState(): void {
    // Update URL
    const params = new URLSearchParams();
    if (activeIntents.length > 0) {
      params.set('filters', encodeURIComponent(JSON.stringify(activeIntents)));
    }
    const newUrl = params.toString() 
      ? `${window.location.pathname}?${params.toString()}` 
      : window.location.pathname;
    window.history.replaceState({}, '', newUrl);
    
    // Dispatch event for ScopedResults
    window.dispatchEvent(new CustomEvent('scoped-search-updated', {
      detail: { intents: activeIntents }
    }));
  }

  function getFieldColor(field: string): string {
    switch (field) {
      case 'speaker': return 'bg-blue-100 text-blue-800';
      case 'category': return 'bg-purple-100 text-purple-800';
      case 'keyword': return 'bg-orange-100 text-orange-800';
      case 'title': return 'bg-green-100 text-green-800';
      default: return 'bg-gray-100 text-gray-800';
    }
  }
</script>

<!-- TEMPLATE -->
<div class="p-4">
  <h1 class="text-xl font-bold mb-4">Scoped Search</h1>
  
  <!-- Active Intent Pills -->
  {#if activeIntents.length > 0}
    <div class="flex flex-wrap gap-2 mb-3">
      {#each activeIntents as intent (intent.field + intent.value)}
        <span class="inline-flex items-center gap-1 px-3 py-1 rounded-full text-sm {getFieldColor(intent.field)}">
          <span class="font-semibold uppercase text-xs opacity-70">{intent.field}:</span>
          {intent.displayLabel}
          <button 
            type="button"
            onclick={() => removeIntent(intent)}
            class="ml-1 font-bold"
          >
            ✕
          </button>
        </span>
      {/each}
      <button 
        type="button"
        onclick={clearAll}
        class="text-sm text-gray-500 hover:text-gray-700 underline"
      >
        Clear all
      </button>
    </div>
  {/if}

  <!-- Search Input -->
  <div class="relative">
    <input 
      type="text" 
      bind:value={query}
      placeholder="Search speakers, categories, keywords, or titles..." 
      class="w-full border rounded p-2"
    />

    <!-- Suggestions Dropdown -->
    {#if suggestions.length > 0}
      <ul class="absolute z-50 top-full left-0 w-full bg-white border mt-1 shadow-lg max-h-80 overflow-y-auto rounded">
        {#each suggestions as intent (intent.field + intent.value)}
          <li>
            <button
              type="button"
              class="w-full text-left p-2 hover:bg-gray-100 flex items-center justify-between"
              onclick={() => selectIntent(intent)}
            >
              <span>
                <span class="inline-block text-xs font-semibold uppercase px-2 py-0.5 rounded mr-2 {getFieldColor(intent.field)}">
                  {intent.field}
                </span>
                {intent.displayLabel}
              </span>
              <span class="text-xs text-gray-500">
                {intent.matchCount} {intent.matchCount === 1 ? 'track' : 'tracks'}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
