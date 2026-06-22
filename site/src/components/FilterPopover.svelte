<!-- src/components/FilterPopover.svelte -->
<script lang="ts">
  interface Props {
    label: string;
    options: string[];
    selected: string[];
    onToggle: (val: string) => void;
    color?: string;
  }

  let { label, options, selected, onToggle, color = 'bg-gray-100' }: Props = $props();
  let open = $state(false);
  let filterText = $state('');

  let filteredOptions: string[] = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    if (!q) return options;
    return options.filter(opt => opt.toLowerCase().includes(q));
  });

  let searchInput: HTMLInputElement | undefined = $state();

  function handleToggle(val: string) {
    onToggle(val);
  }

  function clearAll(event: MouseEvent) {
    // Prevent the click from bubbling up to the parent <button> 
    // which would also open the popover.
    event.stopPropagation();
    
    // Create a copy and pass it up. We empty the list.
    selected.forEach(val => onToggle(val));
  }

  function handleClickOutside(node: HTMLElement) {
    const handleClick = (event: MouseEvent) => {
      if (!node.contains(event.target as Node)) {
        open = false;
      }
    };
    document.addEventListener('click', handleClick, true);
    return {
      destroy() {
        document.removeEventListener('click', handleClick, true);
      }
    };
  }

  $effect(() => {
    if (open) {
      Promise.resolve().then(() => {
        searchInput?.focus();
      });
      filterText = '';
    }
  });
</script>

<div class="relative" use:handleClickOutside>
  <button
  type="button"
  onclick={() => (open = !open)}
  class="border px-3 py-1.5 rounded text-sm font-medium bg-white hover:bg-gray-50 flex items-center gap-2 min-w-32 justify-between"
>
  <!-- LEFT SIDE: Clear (X) and Label -->
  <span class="flex items-center gap-2 truncate">
    {#if selected.length > 0}
      <span
        role="button"
        tabindex="0"
        onclick={clearAll}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') clearAll(e as unknown as MouseEvent); }}
        title="Clear all selections"
        class="text-gray-400 hover:text-red-600 rounded-full w-5 h-5 flex items-center justify-center text-xs font-bold bg-gray-100 cursor-pointer"
      >
        ✕
      </span>
    {/if}
    
    <span class="truncate">{label}</span>
    
    {#if selected.length > 0}
      <span class="inline-block px-1.5 py-0.5 rounded-full {color} text-xs">
        {selected.length}
      </span>
    {/if}
  </span>
    
  <!-- RIGHT SIDE: Chevron (v) -->
  <span class="text-gray-400">▾</span>
</button>

  {#if open}
    <div class="absolute top-full left-0 mt-1 w-64 bg-white border rounded shadow-xl z-50 p-2 max-h-72 overflow-y-auto">
      <input
        type="text"
        bind:this={searchInput}
        bind:value={filterText}
        placeholder={`Search ${label.toLowerCase()}...`}
        class="w-full border rounded p-1.5 text-sm mb-2 focus:outline-none focus:ring-1 focus:ring-blue-400"
      />

      {#if filteredOptions.length === 0}
        <p class="text-xs text-gray-400 italic p-2">No matches found</p>
      {:else}
        {#each filteredOptions as opt (opt)}
          <label class="flex items-center gap-2 p-1.5 hover:bg-gray-50 cursor-pointer text-sm rounded">
            <input
              type="checkbox"
              checked={selected.includes(opt)}
              onchange={() => handleToggle(opt)}
              class="cursor-pointer"
            />
            <span class="truncate">{opt}</span>
          </label>
        {/each}
      {/if}
    </div>
  {/if}
</div>
