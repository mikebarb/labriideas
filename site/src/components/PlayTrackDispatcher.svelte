<script lang="ts">
  import { onMount } from 'svelte';
 import { getCachedCatalog } from '../lib/catalogStore.ts';
  import { buildTrack } from '../lib/buildTrack.ts';
  import type { Track } from '../lib/types.ts';

  let catalog: Track[] = $state([]);
  let isCatalogReady = $state(false);

  onMount(async () => {
    try {
      const { tracks } = await getCachedCatalog();
      catalog = tracks;
    } catch (e) {
      console.error('PlayTrackDispatcher: failed to load catalog', e);
    } finally {
      isCatalogReady = true;
    }

    // Listen for filename-based play requests
    window.addEventListener('play-track-by-filename', handlePlayByFilename);
  });

  function handlePlayByFilename(e: Event) {
    const filename = (e as CustomEvent<string>).detail;
    if (!filename) return;

    // Guard against dispatching before catalog is loaded
    if (!isCatalogReady) {
      console.warn(`PlayTrackDispatcher: catalog not ready, ignoring: ${filename}`);
      return;
    }

    // Look up the track in the catalog
    const item = catalog.find((t: Track) => t.filename === filename);
    if (!item) {
      console.warn(`PlayTrackDispatcher: filename not found in catalog: ${filename}`);
      return;
    }

    // Build the track object and dispatch the real play event
    const track = buildTrack(item);
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }
</script>

<!-- This component renders nothing; it only wires up the dispatcher -->
