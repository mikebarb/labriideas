<!-- src/components/MobileMenu.svelte -->
<script>
  import { navItems } from '../data/menu.js';
  export let isOpen = false;
</script>

{#if isOpen}
  <!-- 
    CHANGED: Added max-h, overflow-y-auto, overscroll-contain, and a shadow.
    - max-h-[calc(100dvh-5rem)]: leaves room for the navbar and the bottom audio player.
    - overflow-y-auto: enables vertical scrolling when content exceeds the height.
    - overscroll-contain: traps the scroll inside the menu so it doesn't scroll the background page.
  -->
  <div class="md:hidden border-t border-gray-200 bg-white pb-6 px-4 max-h-[calc(100dvh-5rem)] overflow-y-auto overscroll-contain shadow-xl">
    {#each navItems as item}
      <div class="py-2 border-b border-gray-100">
        <a href={item.href} class="block uppercase tracking-wider text-sm font-semibold text-gray-800 hover:text-orange-600">
          {item.label}
        </a>
        {#if item.dropdown}
          <div class="pl-4 mt-1">
            {#each item.dropdown as dropItem}
              <a href={dropItem.href} class="block text-sm text-gray-600 py-1 hover:text-orange-600">{dropItem.label}</a>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
    
    <!-- REMOVED the mobile search from here, as it is now in the main header -->
  </div>
{/if}
