<script>
  export let track = null; // The track object passed from the parent list
  
  let editableMetadata = {};
  let isSaving = false;
  let status = "";

  // Reactively reflect changes when the selected track changes
  $: if (track) {
    editableMetadata = { ...track };
    status = "";
  }

  async function handleSave() {
    if (!track) return;
    
    isSaving = true;
    status = "Saving...";

    // 1. Filter out the system fields, only send what R2 uses for metadata
    const metadataPayload = { ...editableMetadata };
    delete metadataPayload.id;
    delete metadataPayload.filename;
    delete metadataPayload.hash;
    // NOTE: We do NOT delete `audio-hash`. It must be sent back to R2 
    // so the Copy-Over-Self operation preserves the fingerprint!

    try {
      const response = await fetch('http://localhost:8080/api/update-metadata', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filename: track.filename,
          metadata: metadataPayload
        })
      });

      if (!response.ok) throw new Error('Save failed');
      
      status = "✅ Metadata updated!";
      setTimeout(() => status = "", 3000);
    } catch (error) {
      console.error(error);
      status = "❌ Save failed";
    } finally {
      isSaving = false;
    }
  }
</script>

{#if track}
  <div class="bg-slate-800 p-6 rounded-lg border border-cyan-500/50 shadow-xl">
    <h3 class="text-lg font-bold text-cyan-400 mb-4">Edit: {track.filename}</h3>
    
    <div class="grid gap-4">
      {#each Object.entries(editableMetadata) as [key, value]}
        <!-- UPDATED: Hide audio-hash from the UI so users can't edit it -->
        {#if key !== 'id' && key !== 'filename' && key !== 'hash' && key !== 'audio-hash'}
          <label class="block">
            <span class="text-xs text-slate-400 uppercase">{key}</span>
            <input 
              bind:value={editableMetadata[key]} 
              class="w-full bg-slate-900 border border-slate-600 rounded p-2 text-white"
            />
          </label>
        {/if}
      {/each}
    </div>

    <button 
      on:click={handleSave}
      disabled={isSaving}
      class="mt-6 w-full bg-cyan-500 hover:bg-cyan-400 text-slate-900 font-bold py-2 px-4 rounded transition-colors">
      {isSaving ? 'Saving...' : 'Save Metadata'}
    </button>

    {#if status}
      <p class="mt-4 text-center text-sm font-medium text-cyan-400">{status}</p>
    {/if}
  </div>
{:else}
  <div class="text-slate-500 text-sm italic">Select a track to edit.</div>
{/if}
