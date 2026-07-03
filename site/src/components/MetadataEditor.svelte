<script lang='ts'>
  import MetadataForm from './MetadataForm.svelte'; // Update path if needed
  
  // export let track = null; // The track object passed from the parent list
  // export let onClose = () => {}; // Callback to close the editor
 
   type Props = {
    track: any | null;
    onClose?: () => void;
    apiBase?: string;
  };
  let { track = null, onClose = () => {}, apiBase = '' }: Props = $props();

  let editableMetadata = $state({});
  let isSaving = $state(false);
  let isSaved = $state(false);
  let errorMessage = $state('');

  $effect(() => {
    if (track) {
      editableMetadata = { ...track };
      isSaved = false;
      errorMessage = '';
    }
  });
 
 // Cancel function to revert changes
  function handleCancel() {
    editableMetadata = { ...track };
    isSaved = false;
    errorMessage = '';
    onClose();
  }

  async function handleSave() {
    if (!track || isSaved) return;
    
    isSaving = true;
    isSaved = false;
    errorMessage = '';

    // 1. Filter out the system fields, only send what R2 uses for metadata
    const metadataPayload = { ...editableMetadata } as any;
    delete metadataPayload.id;
    delete metadataPayload.filename;
    delete metadataPayload.hash;
    // NOTE: We do NOT delete `audio-hash`. It must be sent back to R2 
    // so the Copy-Over-Self operation preserves the fingerprint!

    try {
      //const response = await fetch('${import.meta.env.PUBLIC_API_BASE_URL}/api/update-metadata', {
      const response = await fetch(`${apiBase}/api/update-metadata`, {  
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filename: track.filename,
          metadata: metadataPayload
        })
      });

      if (!response.ok){
        const errorText = await response.text();
        throw new Error('Save failed');
      }
      isSaved = true;
    }catch (error: unknown) {
      console.error('Save failed:', error);
      if (error instanceof Error) {
        errorMessage = error.message || 'Save failed';
      } else if (typeof error === 'string') {
        errorMessage = error;
      } else {
        errorMessage = 'Save failed';
      }
      isSaved = false;
    }finally {
      isSaving = false;
    }
  }
</script>

{#if track}
  <div class="bg-slate-800 p-6 rounded-lg border border-cyan-500/50 shadow-xl flex flex-col min-h-0 overflow-hidden">
    <h3 class="text-lg font-bold text-cyan-400 mb-4">Edit: {track.filename}</h3>
    
    <!-- SHARED COMPONENT -->
      <div class="flex-1 overflow-y-auto pr-2">
      <MetadataForm 
        bind:metadata={editableMetadata} 
        hiddenFields={['id', 'hash', 'audio-hash']}
        readonlyFields={['filename']}
      />
    </div>

    <div class="mt-6 flex gap-4 shrink-0">
      <button 
        onclick={handleCancel}
        disabled={isSaving}
        class="flex-1 bg-slate-700 hover:bg-slate-600 text-slate-200 font-bold py-2 px-4 rounded transition-colors">
        Close
      </button>

      <button 
        onclick={handleSave}
        disabled={isSaving || isSaved}
        class="flex-1 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
        {#if isSaving}
          Saving...
        {:else if isSaved}
          ✅ Saved
        {:else}
          Save Metadata
        {/if}
      </button>
    </div>

    {#if errorMessage}
      <p class="mt-4 text-center text-sm font-medium text-red-400">
        ❌ {errorMessage}
      </p>
    {/if}
  </div>
{:else}
  <div class="text-slate-500 text-sm italic">Select a track to edit.</div>
{/if}
