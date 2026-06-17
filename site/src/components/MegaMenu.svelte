<!-- src/components/MegaMenu.svelte -->
<script lang="ts">
  // Direct JSON import - Vite will bundle this client-side
  import menuData from '../data/menu.json';
  
  // === TYPE DEFINITIONS ===
  
  interface HierarchyItem {
    subtopic: string;
    altName?: string;
    category: string;
  }

  interface FeaturedItem {
    title: string;
    speaker: string;
    url: string;
  }

  interface FeaturedItem {
    title: string;
    speaker: string;
    url: string;
  }

  interface FeaturedConfig {
    [root: string]: {
      featured: FeaturedItem[];
    };
  }

  // The exact shape of the Topics subMenu after extraction
  interface TopicsData {
    hierarchy: Record<string, Record<string, HierarchyItem[]>>;
    featured?: Record<string, FeaturedItem[]>;
  }

 // === EXTRACT DATA INTERNALLY ===
  
  const topicsSubMenu = menuData.subMenus.find(s => s.subMenu === 'Topics');
  //if (!topicsSubMenu || !topicsSubMenu.hierarchy) {
  if (!topicsSubMenu) {
    throw new Error('Configuration error: "Topics" subMenu not found in menu.json');
  }
  // Type guard: explicitly check the subMenu name
  //if (topicsSubMenu.subMenu !== 'Topics') {
  //  throw new Error('Expected Topics subMenu');
  //}


  // Cast to our explicit TopicsData shape
  // This tells TypeScript: "trust me, this is the Topics variant"
  const topicsData = topicsSubMenu as unknown as TopicsData;
  
  if (!topicsData.hierarchy) {
    throw new Error('Configuration error: "Topics" subMenu missing hierarchy property');
  }
  
  // Now both properties are guaranteed to exist
  const hierarchy = topicsData.hierarchy;
  const featured = topicsData.featured || {};

  // These are now locally available - no props needed!
  //const hierarchy = topicsSubMenu.hierarchy as Record<string, Record<string, HierarchyItem[]>>;
  //const featured = (topicsSubMenu.featured || {}) as FeaturedConfig;
  // === REACTIVE STATE (Svelte 5 syntax) ===
  
  // Type is inferred as string | null from the initial value
  let activeRoot = $state<string | null>(null);
  let activeSub = $state<string | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  // === HELPER FUNCTIONS ===
  
  const getLabel = (item: HierarchyItem): string => 
    (item.altName && item.altName.trim() !== '') ? item.altName : item.subtopic;

  const handleMouseEnter = (root: string): void => {
    if (timer) clearTimeout(timer);
    activeRoot = root;
    activeSub = Object.keys(hierarchy[root])[0];
  };

  const handleMouseLeave = (): void => {
    timer = setTimeout(() => {
      activeRoot = null;
      activeSub = null;
    }, 500);
  };

  const handleHeaderHover = (): void => {
    if (timer) clearTimeout(timer);
    activeRoot = null;
    activeSub = null;
  };
</script>

<nav class="bg-gray-50 border-b border-gray-200 py-4" onmouseleave={handleMouseLeave}>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
      <button
        type="button"
        class="flex items-center space-x-2 text-black font-bold uppercase tracking-wider text-sm cursor-default bg-transparent border-none p-0"
        onmouseenter={handleHeaderHover}
      >
        <span>BROWSE LECTURES</span>
        <span>➔</span>
      </button>

      {#each Object.keys(hierarchy) as root}
        <button 
          class="uppercase text-sm font-semibold hover:text-orange-600 {activeRoot === root ? 'text-orange-600' : 'text-gray-800'}"
          onmouseenter={() => handleMouseEnter(root)}
        >
          {root}
        </button>
      {/each}
    </div>

    {#if activeRoot}
      <div class="absolute left-0 right-0 z-40 bg-white shadow-xl border-t border-gray-100 grid grid-cols-2 md:grid-cols-4 mt-4 -mx-4 sm:-mx-6 lg:-mx-8" role="menu">
        
        <!-- COLUMN 1: Categories -->
        <div class="col-span-1 bg-gray-50 p-6 border-r border-b md:border-b-0">
          {#each Object.keys(hierarchy[activeRoot]) as category}
            <button 
              class="py-2 w-full text-left cursor-pointer hover:text-orange-600 text-sm font-semibold {activeSub === category ? 'text-orange-600' : 'text-gray-700'}"
              onmouseenter={() => activeSub = category}
            >
              {category}
            </button>
          {/each}
        </div>

        <!-- COLUMN 2: Sub-Categories -->
        <div class="col-span-1 p-6 border-b md:border-b-0">
          {#if activeSub}
            <h3 class="text-xs font-bold uppercase mb-4 text-gray-500 tracking-wider">{activeSub}</h3>
            <div class="flex flex-col gap-1">
              {#each hierarchy[activeRoot][activeSub] as item}
                <a 
                  href="/topics/{item.category}?label={encodeURIComponent(getLabel(item))}" 
                  class="text-sm text-gray-700 hover:text-orange-600 py-1"
                >
                  {getLabel(item)}
                </a>
              {/each}
            </div>
          {/if}
        </div>

        <!-- COLUMN 3: Featured Lectures -->
        <div class="col-span-2 md:col-span-2 p-6 bg-white border-t md:border-t-0 md:border-l">
          <h4 class="text-xs font-bold uppercase text-orange-600 mb-4 tracking-wider">Featured Lectures</h4>
          <div class="flex flex-col gap-4">
            {#if featured[activeRoot]}
              {#each featured[activeRoot] as item}
                <a href={item.url} class="group block border-b border-gray-100 pb-3 last:border-0">
                  <span class="block text-sm font-bold text-gray-900 group-hover:text-orange-600 transition-colors">{item.title}</span>
                  <span class="block text-xs text-gray-500 mt-1">{item.speaker}</span>
                </a>
              {/each}
            {:else}
              <p class="text-xs text-gray-400 italic">No featured lectures for this section yet.</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
</nav>
