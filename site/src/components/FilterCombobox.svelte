<!-- src/components/FilterCombobox.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { onMount } from 'svelte';

  interface Props {
    field: string;           // The catalog field to extract ('category', 'speaker', etc.)
    value: string;           // Current value (controlled by parent)
    placeholder?: string;    // Optional placeholder text
    onUpdate: (val: string) => void;
  }

  let { field, value, placeholder = 'Search...', onUpdate }: Props = $props();
  
  let options: string[] = $state([]);
  let filtered: string[] = $state([]);
  let showDropdown = $state(false);
  const listboxId = $derived(`filter-combobox-${field}-${Math.random().toString(36).substr(2, 9)}`);
  onMount(async () => {
    const catalog = await getCatalog();
    
    // Extract all values for this field, handling both strings and arrays
    const allValues = catalog.flatMap((track: any) => {
      const val = track[field];
      if (Array.isArray(val)) return val;
      return val ? [val] : [];
    });
    
    // Deduplicate, filter empty, sort alphabetically
    options = [...new Set(allValues.filter(Boolean))].sort();
    filtered = options;
  });

  function handleInput(e: Event) {
    const input = (e.target as HTMLInputElement).value;
    filtered = options.filter(opt => opt.toLowerCase().includes(input.toLowerCase()));
    showDropdown = true;
  }

  function select(opt: string) {
    onUpdate(opt);
    showDropdown = false;
  }

  function handleBlur(e: FocusEvent) {
    const input = e.currentTarget as HTMLInputElement;
    const currentInput = input.value;
    
    if (!options.includes(currentInput) && currentInput !== '') {
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
    placeholder={placeholder}
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
      {#each filtered as opt (opt)}
        <li 
          role="option" 
          aria-selected={value === opt}
        >
          <button
            type="button"
            class="w-full text-left p-2 hover:bg-orange-100 cursor-pointer focus:bg-orange-100 focus:outline-none {value === opt ? 'bg-orange-50' : ''}"
            onclick={() => select(opt)}
            onmousedown={(e) => e.preventDefault()}
          >
            {opt}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
