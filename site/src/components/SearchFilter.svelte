<!-- src/components/SearchFilter.svelte -->
<script lang="ts">
  import FilterCombobox from './FilterCombobox.svelte'; // ← The new generic component
  import { onMount } from 'svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { extractAllKeywords } from '../lib/dataUtils.js';

  let titleQuery = $state('');
  let speakerQuery = $state('');
  let categoryQuery = $state('');
  let keywordInput = $state('');
  let keywords: string[] = $state([]);
  let isFuzzy = $state(false);
  let allAvailableKeywords: string[] = $state([]);
  let isLoading = $state(true);

  onMount(async () => {
    try {
      // 1. Load catalog
      const { tracks: catalog } = await getCachedCatalog();

      // 2. Build the keyword universe using the shared utility
      allAvailableKeywords = extractAllKeywords(catalog);
    } catch(e) {
      console.error('SearchFilter: failed to load catalog', e);
    } finally {
      isLoading = false;
    }

    // 3. Read the current URL state
    const params = new URLSearchParams(window.location.search);
    titleQuery = params.get('title') || '';
    speakerQuery = params.get('speaker') || '';
    categoryQuery = params.get('category') || '';
    isFuzzy = params.get('fuzzy') === 'true';
    
    const kwParam = params.get('keywords');
    if (kwParam) {
      keywords = kwParam.split(',').map(k => k.trim().toLowerCase()).filter(Boolean);
    }
  });

  // New: Filter suggestions based on what the user types
  let keywordSuggestions = $derived(
    allAvailableKeywords.filter(k => 
      k.includes(keywordInput.toLowerCase()) && !keywords.includes(k)
    )
  );

  function selectKeyword(kw: string) {
    keywords = [...keywords, kw];
    keywordInput = '';
    updateParams();
  }

  function addKeyword() {
    const trimmed = keywordInput.trim().toLowerCase();
    if (trimmed && !keywords.includes(trimmed)) {
      keywords = [...keywords, trimmed];
      keywordInput = '';
      updateParams();
    }
  }

  function removeKeyword(kw: string) {
    keywords = keywords.filter(k => k !== kw);
    updateParams();
  }

  function handleKeywordKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      addKeyword();
    }
  }

  function updateParams() {
    const params = new URLSearchParams();
    if (titleQuery) params.set('title', titleQuery);
    if (speakerQuery) params.set('speaker', speakerQuery);
    if (categoryQuery) params.set('category', categoryQuery);
    if (keywords.length > 0) params.set('keywords', keywords.join(','));
    if (isFuzzy) params.set('fuzzy', 'true');
    
    const newQs = params.toString();
    const newUrl = newQs ? `${window.location.pathname}?${newQs}` : window.location.pathname;
    
    window.history.replaceState({}, '', newUrl);
    window.dispatchEvent(new Event('search-updated'));
  }

  function clearAll() {
    titleQuery = '';
    speakerQuery = '';
    categoryQuery = '';
    keywords = [];
    isFuzzy = false;
    keywordInput = '';
    
    window.history.replaceState({}, '', window.location.pathname);
    window.dispatchEvent(new Event('search-updated'));
  }
</script>

{#if isLoading}
  <div class="text-sm text-gray-500 italic">Loading catalog...</div>
{:else}
  <div class="space-y-4">
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <input 
        type="text" 
        placeholder="Search Titles..." 
        value={titleQuery}
        oninput={(e) => { titleQuery = e.currentTarget.value; setTimeout(updateParams, 0); }}
        class="border rounded p-2 w-full"
        inputmode="search"
      />
      
      <!-- 
        Speaker is now a dropdown, just like Category. 
        This gives users a curated list of valid speakers instead of free-text.
      -->
      <FilterCombobox 
        field="speaker"
        value={speakerQuery}
        placeholder="Search Speakers..."
        onUpdate={(val) => { speakerQuery = val; updateParams(); }}
      />
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <FilterCombobox 
        field="category"
        value={categoryQuery}
        placeholder="Search Category..."
        onUpdate={(val) => { categoryQuery = val; updateParams(); }}
      />
      
      <div class="relative flex gap-2">
          <input 
              type="text" 
              placeholder="Type keyword..." 
              value={keywordInput}
              oninput={(e) => { keywordInput = e.currentTarget.value; }}
              onkeydown={handleKeywordKeydown}
              class="border rounded p-2 flex-1 w-full"
          />
          
          <!-- Suggestions Dropdown -->
          {#if keywordInput && keywordSuggestions.length > 0}
              <ul class="absolute z-50 top-full left-0 w-full bg-white border mt-1 shadow-lg max-h-40 overflow-y-auto">
              {#each keywordSuggestions as kw}
                  <li>
                  <button 
                      type="button"
                      class="w-full text-left p-2 hover:bg-orange-100"
                      onclick={() => selectKeyword(kw)}
                  >
                      {kw}
                  </button>
                  </li>
              {/each}
              </ul>
          {/if}
      </div>
    </div>
    <!-- Active Keywords -->
    {#if keywords.length > 0}
      <div class="flex flex-wrap gap-2">
        {#each keywords as kw}
          <span class="inline-flex items-center gap-1 bg-orange-100 text-orange-800 px-3 py-1 rounded-full text-sm">
            {kw}
            <button 
              type="button"
              onclick={() => removeKeyword(kw)}
              class="text-orange-600 hover:text-orange-900"
            >
              ✕
            </button>
          </span>
        {/each}
      </div>
    {/if}

    <div class="flex items-center justify-between">
      <label class="flex items-center gap-2">
        <input 
          type="checkbox" 
          checked={isFuzzy} 
          onchange={(e) => { isFuzzy = e.currentTarget.checked; updateParams(); }} 
        />
        Enable Fuzzy Search (Title only)
      </label>
      
      {#if titleQuery || speakerQuery || categoryQuery || keywords.length > 0 || isFuzzy}
        <button 
          type="button"
          onclick={clearAll}
          class="text-sm text-gray-500 hover:text-gray-700 underline"
        >
          Clear all filters
        </button>
      {/if}
    </div>
  </div>
{/if}
