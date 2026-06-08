<script>
  export let metadata = {};
  export let hiddenFields = ['id', 'hash']; // Fields to completely hide
  export let readonlyFields = [];           // Fields to show but lock
</script>

<div class="grid gap-4 bg-slate-900 p-4 rounded">
  {#each Object.entries(metadata) as [key, value]}
    {#if !hiddenFields.includes(key)}
      <label class="block">
        <span class="text-xs text-slate-400 uppercase">{key}</span>
        <input 
          type="text" 
          bind:value={metadata[key]} 
          readonly={readonlyFields.includes(key)}
          class="w-full border rounded p-2 {readonlyFields.includes(key) 
            ? 'bg-slate-950 border-slate-700 text-slate-400 cursor-default' 
            : 'bg-slate-800 border-slate-600 text-white'}"
        />
      </label>
    {/if}
  {/each}
</div>
