<!-- src/components/MegaMenu.svelte -->
<script>
  import { menuStructure } from '../data/menu.js';
  
  let activeRoot = null; // Tracks which main tab is hovered (e.g., WORLDVIEW)
  let activeSub = null;  // Tracks which category is hovered (e.g., Bible)
  let timer;

  const handleMouseEnter = (root) => {
    clearTimeout(timer);
    activeRoot = root;
    // Default the sub-category to the first item in the list
    activeSub = Object.keys(menuStructure[root].categories)[0]; 
  };

  const handleMouseLeave = () => {
    // 2-second grace period before closing, as requested earlier
    timer = setTimeout(() => {
      activeRoot = null;
      activeSub = null;
    }, 500);
  };

  // Reset the menu when hovering over the main header
  const handleHeaderHover = () => {
    clearTimeout(timer);
    activeRoot = null;
    activeSub = null;
  };
</script>

<nav class="bg-gray-50 border-b border-gray-200 py-4" on:mouseleave={handleMouseLeave}>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    
    <!-- Top Row: Browse Lectures Header & Main Tabs -->
    <!-- Top Row: FIXED flex-wrap and gap to prevent truncation -->
    <!-- Changed from: flex items-center space-x-6 -->
    <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
    <!-- <div class="flex items-center space-x-6"> -->
      <!-- BROWSE LECTURES Header: Added on:mouseenter to reset menu -->
      <!-- class="flex items-center space-x-2 text-black font-bold uppercase tracking-wider text-sm cursor-default" -->
      <button
        type="button"
        class="flex items-center space-x-2 text-black font-bold uppercase tracking-wider text-sm cursor-default bg-transparent border-none p-0"
        on:mouseenter={handleHeaderHover}
      >
      <!-- <div class="flex items-center space-x-2 text-black font-bold uppercase tracking-wider text-sm"> -->
        <span>BROWSE LECTURES</span>
        <span>➔</span>
      </button>

      {#each Object.keys(menuStructure) as root}
        <button 
          class="uppercase text-sm font-semibold hover:text-orange-600 {activeRoot === root ? 'text-orange-600' : 'text-gray-800'}"
          on:mouseenter={() => handleMouseEnter(root)}
        >
          {root}
        </button>
      {/each}
    </div>

    <!-- Dropdown Panel: Only shows when a root tab is active -->
    {#if activeRoot}
      <!-- Grid: 2 columns on mobile, 4 columns on desktop -->
      <div class="absolute left-0 right-0 z-40 bg-white shadow-xl border-t border-gray-100 grid grid-cols-2 md:grid-cols-4 mt-4 -mx-4 sm:-mx-6 lg:-mx-8" role="menu">
        
        <!-- COLUMN 1: Categories (Left) -->
        <!-- col-span-1 takes up 1 unit. border-b on mobile, removed on md+ -->
        <div class="col-span-1 bg-gray-50 p-6 border-r border-b md:border-b-0">
          {#each Object.keys(menuStructure[activeRoot].categories) as category}
            <button 
              class="py-2 w-full text-left cursor-pointer hover:text-orange-600 text-sm font-semibold {activeSub === category ? 'text-orange-600' : 'text-gray-700'}"
              on:mouseenter={() => activeSub = category}
            >
              {category}
        </button>
          {/each}
        </div>

        <!-- COLUMN 2: Sub-Categories (Middle) -->
        <!-- col-span-1 takes up 1 unit. border-b on mobile, removed on md+ -->
        <div class="col-span-1 p-6 border-b md:border-b-0">
          {#if activeSub}
            <h3 class="text-xs font-bold uppercase mb-4 text-gray-500 tracking-wider">{activeSub}</h3>
            <div class="flex flex-col gap-1">
              {#each menuStructure[activeRoot].categories[activeSub] as sub}
                <a 
                  href="/topics/{sub.toLowerCase().replace(/ /g, '-')}" 
                  class="text-sm text-gray-700 hover:text-orange-600 py-1"
                >
                  {sub}
                </a>
              {/each}
            </div>
          {/if}
        </div>

        <!-- COLUMN 3: Featured Lectures (Right) -->
        <!-- col-span-2 spans the whole row on mobile. md:col-span-2 takes up 2/4 on desktop -->
        <!-- border-t on mobile to separate from row above, removed on md+. border-l added on md+ -->
        <div class="col-span-2 md:col-span-2 p-6 bg-white border-t md:border-t-0 md:border-l">

          <h4 class="text-xs font-bold uppercase text-orange-600 mb-4 tracking-wider">Featured Lectures</h4>
          <div class="flex flex-col gap-4">
            {#each menuStructure[activeRoot].featured as item}
              <a href={item.url} class="group block border-b border-gray-100 pb-3 last:border-0">
                <span class="block text-sm font-bold text-gray-900 group-hover:text-orange-600 transition-colors">{item.title}</span>
                <span class="block text-xs text-gray-500 mt-1">{item.speaker}</span>
              </a>
            {/each}
          </div>
        </div>

      </div>
    {/if}
  </div>
</nav>
