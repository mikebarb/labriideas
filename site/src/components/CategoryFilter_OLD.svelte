<!-- src/components/CategoryFilter.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { onMount } from 'svelte';

  interface Props {
    value: string;
    onUpdate: (val: string) => void;
  }

  let { value, onUpdate }: Props = $props();
  let categories: string[] = $state([]);
  let filtered: string[] = $state([]);
  let showDropdown = $state(false);
  const listboxId = `category-listbox-${Math.random().toString(36).substr(2, 9)}`;

  onMount(async () => {
    const catalog = await getCatalog();
    const allCats = catalog.map((t: any) => t.category).filter(Boolean).flat();
    categories = [...new Set(allCats)].sort();
    filtered = categories;
  });

  function handleInput(e: Event) {
    const input = (e.target as HTMLInputElement).value;
    filtered = categories.filter(c => c.toLowerCase().includes(input.toLowerCase()));
    showDropdown = true;
  }

  function select(cat: string) {
    onUpdate(cat);
    showDropdown = false;
  }

  function handleBlur(e: FocusEvent) {
      // Read from the actual input element that just lost focus
      const input = e.currentTarget as HTMLInputElement;
      const currentInput = input.value;
      
      if (!categories.includes(currentInput) && currentInput !== '') {
        onUpdate('');
      }
      setTimeout(() => showDropdown = false, 200);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      showDropdown = false;
    } else if (e.key === 'Enter' && showDropdown && filtered.length > 0) {
      e.preventDefault();
      select(filtered[0]);
    }
  }
</script>

<div class="relative w-full">
  <input 
    type="text"
    {value}
    placeholder="Search Category..."
    oninput={handleInput}
    onblur={handleBlur}
    onfocus={() => showDropdown = true}
    onkeydown={handleKeydown}
    class="border rounded p-2 w-full"
    inputmode="search"
    role="combobox"
    aria-expanded={showDropdown}
    aria-controls={listboxId}
    aria-autocomplete="list"
  />

  {#if showDropdown && filtered.length > 0}
    <ul 
      id={listboxId}
      class="absolute z-50 bg-white border mt-1 w-full max-h-60 overflow-y-auto shadow-lg rounded"
      role="listbox"
    >
      {#each filtered as cat (cat)}
        <li 
          role="option" 
          aria-selected={value === cat}
        >
          <button
            type="button"
            class="w-full text-left p-2 hover:bg-orange-100 cursor-pointer focus:bg-orange-100 focus:outline-none {value === cat ? 'bg-orange-50' : ''}"
            onclick={() => select(cat)}
            onmousedown={(e) => e.preventDefault()}
          >
            {cat}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
