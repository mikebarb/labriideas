<!-- src/components/TopicsTree.svelte -->
<script lang="ts">
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';
  import menuData from '../data/menu.json';

  interface LeafItem {
    subtopic: string;
    altName?: string;
    category: string;
  }

  // Find the Topics menu (this returns 'SubMenu | undefined')
  const topicsSubMenu = menuData.subMenus.find(s => s.subMenu === 'Topics');
  
  // Guard clause: if menu.json is malformed, throw a build error
  if (!topicsSubMenu || !topicsSubMenu.hierarchy) {
    throw new Error('Configuration error: "Topics" subMenu with hierarchy not found in menu.json');
  }
  
  // Now TypeScript knows hierarchy exists
  const hierarchy = topicsSubMenu.hierarchy;

  // Track open state for BOTH minor themes (under majors) and the major themes themselves
  let openSections: Record<string, boolean> = $state({});

  // Open all major themes on initial load
  for (const majorTheme of Object.keys(hierarchy)) {
    openSections[majorTheme] = true;
  }

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
    for (const [majorTheme, minorMap] of Object.entries(hierarchy)) {
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

  // NEW: Pure URL generator (no side effects)
  function getTopicUrl(item: LeafItem): string {
    return `/topics/${item.category}?label=${encodeURIComponent(item.subtopic)}`;
  }

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

  {#each Object.entries(hierarchy) as [majorTheme, minorMap]}
    <div class="mb-6">
      
      <!-- MAJOR THEME HEADER (Now also collapsible) -->
      <button
        type="button"
        class="flex items-center gap-2 w-full text-left py-2 mb-1"
        onclick={() => toggle(majorTheme)}
      >
        {#if openSections[majorTheme]}
          <ChevronDown size={20} class="text-orange-600 shrink-0" />
        {:else}
          <ChevronRight size={20} class="text-gray-600 shrink-0" />
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
                      <a
                        class="flex items-center gap-3 py-1.5 px-2 text-sm text-gray-600 hover:text-orange-600 hover:bg-gray-50 rounded w-full text-left transition-colors no-underline"
                        href={getTopicUrl(item)}
                        title={`Open track list for: ${item.subtopic}`}
                      >
                        <Search size={14} class="opacity-50 shrink-0" />
                        <span>{item.subtopic}</span>
                    </a>
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
