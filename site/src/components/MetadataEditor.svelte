<script>
  import MetadataForm from './MetadataForm.svelte'; // Update path if needed
  
  export let track = null; // The track object passed from the parent list
  export let onClose = () => {}; // Callback to close the editor
  
  let editableMetadata = {};
  let isSaving = false;
  let status = "";

  // Reactively reflect changes when the selected track changes
  $: if (track) {
    editableMetadata = { ...track };
    status = "";
  }

 // Cancel function to revert changes
  function handleCancel() {
   // 1. Revert changes locally
    editableMetadata = { ...track };
    status = "";
    // 2. Tell the parent to close the editor
    onClose(); 
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
      const response = await fetch('${import.meta.env.PUBLIC_API_BASE_URL}/api/update-metadata', {
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
    
    <!-- SHARED COMPONENT -->
    <MetadataForm 
      bind:metadata={editableMetadata} 
      hiddenFields={['id', 'filename', 'hash', 'audio-hash']} 
    />

    <div class="mt-6 flex gap-4">
      <button 
        on:click={handleCancel}
        disabled={isSaving}
        class="flex-1 bg-slate-700 hover:bg-slate-600 text-slate-200 font-bold py-2 px-4 rounded transition-colors">
        Cancel
      </button>

      <button 
        on:click={handleSave}
        disabled={isSaving}
        class="mt-6 w-full bg-cyan-500 hover:bg-cyan-400 text-slate-900 font-bold py-2 px-4 rounded transition-colors">
        {isSaving ? 'Saving...' : 'Save Metadata'}
      </button>
    </div>

    {#if status}
      <p class="mt-4 text-center text-sm font-medium text-cyan-400">{status}</p>
    {/if}
  </div>
{:else}
  <div class="text-slate-500 text-sm italic">Select a track to edit.</div>
{/if}
