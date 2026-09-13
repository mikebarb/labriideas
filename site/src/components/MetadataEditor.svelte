<script lang='ts'>
  import MetadataForm from './MetadataForm.svelte';
  // Authenticated transport: /api/update-metadata is behind the Go
  // server's authMiddleware — a bare fetch carries no Bearer token and
  // 401s at the gate. authClient.fetch attaches the token and
  // auto-clears it if the session has expired.
  import { authClient } from '../lib/authClient';
  // Catalog source for the editable-field whitelist (see below).
  import { getCachedCatalog } from '../lib/catalogStore';
 
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

  // ─── Editable-field whitelist (derived, not hardcoded) ───
  // The authoritative field list is CatalogSchema in
  // publisher/pkg/schema/schema.go. It propagates to the client through
  // the catalog itself: every field the schema defines appears as a key
  // on catalog tracks. So the whitelist is DERIVED from the served
  // catalog at save time — a schema change in Go flows here automatically
  // with no mirror to keep in sync.
  //
  // Two rules keep the derivation safe:
  //
  // 1. Derive from the CATALOG, never from the edited track. The track
  //    in the editor may carry runtime fields (position, isActive, url,
  //    localBlob, ...) added by the player/queue layers — using its keys
  //    would whitelist the exact pollution this filter exists to block.
  //    Catalog tracks come straight from the server and carry only
  //    schema-defined fields.
  //
  // 2. Union keys across ALL tracks — optional fields (e.g. 'topten')
  //    may be absent from any single track, and a first-track-only
  //    derivation would silently make them uneditable.
  //
  // 'filename' and 'hash' are removed after derivation: filename travels
  // in the request envelope (the server looks up the object by it), and
  // hash is server-managed identity (the OPFS content key). 'audio-hash'
  // stays: the Copy-Over-Self update must preserve the fingerprint or
  // duplicate detection breaks.

  const SERVER_MANAGED_FIELDS = new Set(['filename', 'hash']);

  function deriveEditableFields(catalogTracks: any[]): string[] {
    const fieldSet = new Set<string>();
    for (const t of catalogTracks) {
      if (t && typeof t === 'object') {
        for (const key of Object.keys(t)) fieldSet.add(key);
      }
    }
    return [...fieldSet].filter(key => !SERVER_MANAGED_FIELDS.has(key));
  }

  async function handleSave() {
    if (!track || isSaved) return;
    
    isSaving = true;
    isSaved = false;
    errorMessage = '';

    // 1. Derive the whitelist from the (cached) catalog. Fetched at save
    //    time rather than on mount: no state to go stale, no race between
    //    catalog load and the Save click — and getCachedCatalog resolves
    //    from memory when the catalog is already warm (AppShell preloads
    //    it), so this is effectively free.
    let editableFields: string[];
    try {
      const { tracks } = await getCachedCatalog();
      editableFields = deriveEditableFields(tracks ?? []);
    } catch (err) {
      console.error('MetadataEditor: catalog unavailable for field derivation:', err);
      errorMessage = 'Could not load the catalog to determine editable fields. Nothing was saved.';
      isSaving = false;
      return;
    }

    // BLOCK, never fall through: an empty whitelist would send an empty
    // metadata map and WIPE the track's metadata on R2.
    if (editableFields.length === 0) {
      errorMessage = 'No editable fields could be derived (empty catalog). Nothing was saved.';
      isSaving = false;
      return;
    }

    // 2. Whitelist: build the payload from derived fields ONLY, and only
    //    when the field exists on this track (optional fields absent from
    //    this track are simply not sent). Runtime fields on the incoming
    //    track are dropped by never being copied.
    const metadataPayload: Record<string, any> = {};
    for (const field of editableFields) {
      if (field in editableMetadata) {
        metadataPayload[field] = (editableMetadata as any)[field];
      }
    }

    try {
      const response = await authClient.fetch(`${apiBase}/api/update-metadata`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filename: track.filename,
          metadata: metadataPayload
        })
      });

      if (!response.ok) {
        // CHANGED: surface the status (and server message when present)
        // instead of discarding it — "401" vs "500" sends the admin down
        // completely different debugging paths.
        if (response.status === 401) {
          // Session expired (TTL or server restart). authClient.fetch has
          // already cleared the dead token. The form data is INTENTIONALLY
          // left intact — re-login and press Save again; nothing is lost.
          errorMessage = 'Session expired — sign in again, then press Save. Your edits are still here.';
        } else {
          const errorText = await response.text().catch(() => '');
          errorMessage = `Save failed (server returned ${response.status}${errorText ? `: ${errorText}` : ''})`;
        }
        isSaved = false;
        return;
      }
      isSaved = true;
    } catch (error: unknown) {
      // Network-level failure (offline, server unreachable) — also keeps
      // the form data intact for a later retry.
      console.error('Save failed:', error);
      errorMessage = error instanceof Error ? error.message : 'Save failed';
      isSaved = false;
    } finally {
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
