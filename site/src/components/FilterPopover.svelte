<!-- src/components/FilterPopover.svelte -->
<script lang="ts">
  interface Props {
    label: string;
    options: string[] | { value: string; isStale: boolean }[];
    selected: string[];
    onToggle: (val: string) => void;
    color?: string;
    stale?: string[];
  }

  let { label, options, selected, onToggle, color = 'bg-gray-100', stale = [] }: Props = $props();
  let open = $state(false);
  let filterText = $state('');

  // ─── V3-COMPATIBLE WRAPPER (added) ──────────────────────────────
  // Supports both the legacy `string[]` contract (used by V2) and the
  // new `{ value, isStale }[]` contract (used by V3).
  //
  // To revert to V2-only behavior:
  //   1. Remove this `displayOptions` derivation.
  //   2. Change the `{#each}` in the template back to:
  //        {#each filteredOptions as opt (opt)}
  //        ...selected.includes(opt)...
  //   3. Revert `filteredOptions` to operate on `options` directly.
  // ─────────────────────────────────────────────────────────────────
  let displayOptions = $derived(
    options.map(o => {
      const value = typeof o === 'string' ? o : o.value;
      const isStale = typeof o === 'string' 
        ? stale.includes(o) 
        : o.isStale;
      return { value, isStale };
    })
  );

  let filteredOptions: { value: string; isStale: boolean }[] = $derived.by(() => {
    const q = filterText.trim().toLowerCase();
    if (!q) return displayOptions;
    return displayOptions.filter(opt => opt.value.toLowerCase().includes(q));
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
  class="border px-3 py-1.5 rounded text-sm font-medium bg-white hover:bg-gray-50 flex items-center gap-2 min-w-26 justify-between"
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
        {#each filteredOptions as { value, isStale } (value)}
          <label 
            class="flex items-center gap-2 p-1.5 hover:bg-gray-50 cursor-pointer text-sm rounded transition-opacity"
            class:opacity-50={isStale}
            title={isStale ? 'No tracks match with current filters' : ''}
          >
            <input
              type="checkbox"
              checked={selected.includes(value)}
              onchange={() => handleToggle(value)}
              class="cursor-pointer"
            />
            <span class="truncate">{value}</span>
          </label>
        {/each}
      {/if}
    </div>
  {/if}
</div>
