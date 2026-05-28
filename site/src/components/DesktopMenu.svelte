<!-- src/components/DesktopMenu.svelte -->
<script>
  // Import the data from the central file
  import { navItems } from '../data/menu.js';

  let activeNavId = null; 
  let timer;

  const handleMouseEnter = (id) => {
    clearTimeout(timer);
    activeNavId = id; 
  };

  const handleMouseLeave = () => {
    timer = setTimeout(() => {
      activeNavId = null;
    }, 500);
  };
</script>

<!-- Hidden on mobile, flex on desktop -->
<div class="hidden md:flex items-center gap-x-6" on:mouseleave={handleMouseLeave}>
  {#each navItems as item}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div 
      class="relative py-2" 
      on:mouseenter={() => handleMouseEnter(item.id)}
    >
      <a 
        href={item.href} 
        class="hover:text-gray-500 cursor-pointer uppercase tracking-wider text-sm font-semibold {activeNavId === item.id ? 'text-gray-500' : ''}"
      >
        {item.label}{item.dropdown ? ' ▾' : ''}
      </a>
      
      {#if item.dropdown && activeNavId === item.id}
        <div class="absolute top-full left-0 bg-white shadow-lg border border-gray-100 min-w-max py-2">
          {#each item.dropdown as dropItem}
            <a href={dropItem.href} class="block px-4 py-2 hover:bg-gray-100">{dropItem.label}</a>
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>
