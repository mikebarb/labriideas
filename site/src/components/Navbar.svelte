<!-- src/components/Navbar.svelte -->
<script>
  // 1. Define navigation data structure.
  // Any item with a 'dropdown' array will automatically render a dropdown.
  const navItems = [
    { id: 'home', label: 'HOME', href: '/' },
    { 
      id: 'about', 
      label: 'ABOUT', 
      href: '/about', 
      dropdown: [
        { label: 'Using the ideas library', href: '/about/using' },
        { label: 'History of L\'Abri', href: '/about/history' }
      ]
    },
    { 
      id: 'learn', 
      label: 'LEARN', 
      href: '/learn', 
      // Example: If you add a dropdown to Learn later, it just goes here:
      // dropdown: [ { label: 'New Item', href: '/learn/new' } ] 
    },
    { id: 'contact', label: 'CONTACT', href: '/contact' },
    { id: 'external', label: 'L\'ABRI.ORG', href: 'https://labri.org' }
  ];

  let activeNavId = null; // Tracks which main menu item is hovered (e.g., 'about')
  let timer;

  const handleMouseEnter = (id) => {
    clearTimeout(timer);
    activeNavId = id; // Instantly switches state, closing any previous dropdown
  };

  const handleMouseLeave = () => {
    // 2-second grace period when mouse leaves the nav area entirely
    timer = setTimeout(() => {
      activeNavId = null;
    }, 500);
  };
</script>

<nav class="sticky top-0 z-50 w-full bg-white border-b border-gray-200" on:mouseleave={handleMouseLeave}>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="flex flex-wrap items-center py-3 gap-x-6 gap-y-1">
      <!-- Primary Links -->
      <!-- SINGLE FLEX CONTAINER for Menu AND Search -->
      <div class="flex flex-wrap gap-x-6 gap-y-2 items-center">
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

        <!-- Search Bar -->
        <!-- SEARCH BAR: Now lives inside the same flex container -->
        <div class="flex items-center border border-gray-300 rounded-full px-4 py-1 normal-case tracking-normal font-normal">
          <input type="text" placeholder="Search..." class="outline-none text-sm w-32 focus:w-48 transition-all" />
          <button aria-label="Search">
            <svg class="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </div>
</nav>
