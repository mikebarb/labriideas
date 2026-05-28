<script>
  import { menuStructure } from '../data/menu.js';
  let isOpen = false;
</script>

<!-- The Hamburger Button -->
<button class="md:hidden p-4" on:click={() => isOpen = !isOpen}>
  <div class="space-y-1.5">
    <span class="block w-6 h-0.5 bg-black"></span>
    <span class="block w-6 h-0.5 bg-black"></span>
    <span class="block w-6 h-0.5 bg-black"></span>
  </div>
</button>

<!-- The Mobile Drawer -->
{#if isOpen}
  <div class="fixed inset-0 z-100 bg-white pt-20 px-6 overflow-y-auto">
    <button class="absolute top-4 right-4 font-bold" on:click={() => isOpen = false}>CLOSE ✕</button>
    
    <div class="flex flex-col gap-6">
      {#each Object.keys(menuStructure) as root}
        <div>
          <h3 class="font-bold uppercase text-orange-600 mb-2">{root}</h3>
          {#each Object.keys(menuStructure[root].categories) as cat}
             <div class="pl-4 py-1 text-sm">{cat}</div>
          {/each}
        </div>
      {/each}
      <a href="/about" class="font-bold uppercase">ABOUT</a>
      <a href="/contact" class="font-bold uppercase">CONTACT</a>
    </div>
  </div>
{/if}
