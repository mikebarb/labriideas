<!-- src/components/TopicsTree.svelte -->
<script lang="ts">
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';

  export interface LeafItem {
    subtopic: string;
    category: string;
  }

  export interface Props {
    data: Record<string, Record<string, LeafItem[]>>;
  }

  let { data }: Props = $props();

  // Track open state for BOTH minor themes (under majors) and the major themes themselves
  let openSections: Record<string, boolean> = $state({});

  function toggle(id: string) {
    openSections[id] = !openSections[id];
  }

  /**
   * Generates the key used in openSections for a major/minor pair.
   * Example: "Theology & Worldview-L'Abri"
   */
  function getKey(major: string, minor: string) {
    return `${major}-${minor}`;
  }

  function expandAll() {
    const allOpen: Record<string, boolean> = {};
    for (const [majorTheme, minorMap] of Object.entries(data)) {
      allOpen[majorTheme] = true; // Open the major
      for (const minorTheme of Object.keys(minorMap)) {
        allOpen[getKey(majorTheme, minorTheme)] = true; // Open the minor
      }
    }
    openSections = allOpen;
  }

  function collapseAll() {
    openSections = {};
  }

  const selectCategory = (item: LeafItem) => {
    window.open(
      `/topics/${item.category}?label=${encodeURIComponent(item.subtopic)}`,
      '_blank',
      'noopener,noreferrer'
    );
  };
</script>

<div class="max-w-2xl mx-auto py-8 px-4">
  
  <!-- GLOBAL EXPAND/COLLAPSE CONTROLS -->
  <div class="flex gap-2 mb-6 pb-4 border-b border-gray-200">
    <button
      type="button"
      onclick={expandAll}
      class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
      title="Expand all categories"
    >
      <Maximize2 size={14} />
      Expand All
    </button>
    <button
      type="button"
      onclick={collapseAll}
      class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
      title="Collapse all categories"
    >
      <Minimize2 size={14} />
      Collapse All
    </button>
  </div>

  {#each Object.entries(data) as [majorTheme, minorMap]}
    <div class="mb-6">
      
      <!-- MAJOR THEME HEADER (Now also collapsible) -->
      <button
        type="button"
        class="flex items-center gap-2 w-full text-left py-2 mb-1"
        onclick={() => toggle(majorTheme)}
      >
        {#if openSections[majorTheme]}
          <ChevronDown size={20} class="text-orange-600 flex-shrink-0" />
        {:else}
          <ChevronRight size={20} class="text-gray-600 flex-shrink-0" />
        {/if}
        <h2 class="text-2xl font-bold text-gray-900">
          {majorTheme}
        </h2>
      </button>
      
      <!-- MAJOR THEME CONTENT (Only shows when expanded) -->
      {#if openSections[majorTheme]}
        <div class="ml-6 border-l-2 border-gray-200 pl-4">
          {#each Object.entries(minorMap) as [minorTheme, leaves]}
            {@const sectionId = getKey(majorTheme, minorTheme)}
            <div class="mb-2">
              <button 
                class="flex items-center gap-2 py-1.5 font-semibold text-gray-800 hover:text-orange-600 transition-colors w-full text-left"
                onclick={() => toggle(sectionId)}
              >
                {#if openSections[sectionId]}
                  <ChevronDown size={16} class="text-orange-600" />
                {:else}
                  <ChevronRight size={16} />
                {/if}
                <span>{minorTheme}</span>
              </button>
              
              {#if openSections[sectionId]}
                <ul class="ml-6 border-l border-gray-200 pl-3 py-1">
                  {#each leaves as item}
                    <li>
                      <button 
                        class="flex items-center gap-3 py-1.5 px-2 text-sm text-gray-600 hover:text-orange-600 hover:bg-gray-50 rounded w-full text-left transition-colors"
                        onclick={() => selectCategory(item)}
                        title={`Open track list for: ${item.subtopic}`}
                      >
                        <Search size={14} class="opacity-50 flex-shrink-0" />
                        <span>{item.subtopic}</span>
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>
