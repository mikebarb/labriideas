<!-- src/components/SearchFilter.svelte -->
<script lang="ts">
  import CategoryFilter from './CategoryFilter_OLD.svelte'; // ← NEW IMPORT
  import { onMount } from 'svelte';

  let titleQuery = $state('');
  let speakerQuery = $state('');
  let categoryQuery = $state('');
  let keywordInput = $state('');
  let keywords: string[] = $state([]);
  let isFuzzy = $state(false);

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    titleQuery = params.get('title') || '';
    speakerQuery = params.get('speaker') || '';
    categoryQuery = params.get('category') || '';
    isFuzzy = params.get('fuzzy') === 'true';
    
    const kwParam = params.get('keywords');
    if (kwParam) {
      keywords = kwParam.split(',').filter(Boolean);
    }
  });

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
    
    const newUrl = `${window.location.pathname}?${params.toString()}`;
    window.history.replaceState({}, '', newUrl);
    window.dispatchEvent(new Event('search-updated'));
  }

  function clearAll() {
    titleQuery = '';
    speakerQuery = '';
    categoryQuery = '';
    keywords = [];
    isFuzzy = false;
    updateParams();
  }
</script>

<div class="space-y-4">
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <input 
      type="text" 
      placeholder="Search Titles..." 
      value={titleQuery}
      oninput={(e) => {
        titleQuery = e.currentTarget.value;
        // Small delay ensures the deletion is registered before URL update
        setTimeout(updateParams, 0); 
      }}
      class="border rounded p-2 w-full"
      inputmode="search"
    />
    <input 
      type="text" 
      placeholder="Search Speakers..." 
      value={speakerQuery}
      oninput={(e) => {
        speakerQuery = e.currentTarget.value;
        setTimeout(updateParams, 0);
      }}
      class="border rounded p-2 w-full"
      inputmode="search"
    />
  </div>

  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <!-- 
      REPLACEMENT: The Category input is now the CategoryFilter component.
      It sits in the same grid spot as the old free-text input.
    -->
    <CategoryFilter 
      value={categoryQuery} 
      onUpdate={(val) => { categoryQuery = val; updateParams(); }} 
    />
    
    <div class="flex gap-2">
      <input 
        type="text" 
        placeholder="Add keyword and press Enter..." 
        value={keywordInput}
        oninput={(e) => { keywordInput = e.currentTarget.value; }}
        onkeydown={handleKeywordKeydown}
        class="border rounded p-2 flex-1 w-full"
        inputmode="search"
      />
      <button 
        type="button"
        onclick={addKeyword}
        class="bg-gray-200 hover:bg-gray-300 px-4 rounded"
      >
        +
      </button>
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
      Enable Fuzzy Search
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
