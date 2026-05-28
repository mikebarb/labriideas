<!-- src/components/Navbar.svelte -->
<script>
  import DesktopMenu from './DesktopMenu.svelte';
  import MobileMenu from './MobileMenu.svelte';

  let mobileOpen = false;
</script>

<nav class="sticky top-0 z-50 w-full bg-white border-b border-gray-200">
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="flex items-center justify-between py-3">
      
      <!-- LEFT SIDE: Desktop Menu (Hidden on Mobile) -->
      <DesktopMenu />

      <!-- RIGHT SIDE: Search & Hamburger -->
      <!-- ml-auto ensures this stays pinned to the right, especially on mobile when DesktopMenu is hidden -->
      <div class="flex items-center gap-x-4 ml-auto">
        
        <!-- PERSISTENT SEARCH BAR -->
        <div class="relative flex items-center border border-gray-300 rounded-full h-10 w-10 sm:w-32 focus-within:w-48 transition-all duration-300 overflow-hidden">
          
          <!-- Absolute input covers the whole area, making it all clickable -->
          <input 
            type="text" 
            placeholder="Search..." 
            class="absolute inset-0 w-full h-full pl-4 pr-10 outline-none text-sm bg-transparent cursor-pointer" 
          />
          
          <!-- Absolute icon on the right -->
          <div class="absolute inset-y-0 right-0 flex items-center w-10 justify-center pointer-events-none">
            <svg class="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </div>
          
        </div>

        <!-- Hamburger Icon -->
        <!-- flex-shrink-0 ensures the search bar expansion can NEVER push this off the screen -->
        <button 
          class="md:hidden flex-shrink-0 p-2" 
          on:click={() => mobileOpen = !mobileOpen} 
          aria-label="Toggle Menu"
        >
           <svg class="w-6 h-6 text-gray-800" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </button>
      </div>
    </div>
  </div>

  <!-- MOBILE DRAWER -->
  <MobileMenu isOpen={mobileOpen} />
</nav>
