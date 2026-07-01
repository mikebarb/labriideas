<script lang="ts">
  import { onMount } from 'svelte';
  import { getCatalog } from '../lib/catalogStore.ts';
  import { buildTrack } from '../lib/buildTrack.ts';
  import type { Track } from '../lib/types.ts';

  let catalog: Track[] = $state([]);

  onMount(async () => {
    try {
      catalog = await getCatalog();
    } catch (e) {
      console.error('PlayTrackDispatcher: failed to load catalog', e);
    }

    // Listen for filename-based play requests
    window.addEventListener('play-track-by-filename', handlePlayByFilename);
  });

  function handlePlayByFilename(e: Event) {
    const filename = (e as CustomEvent<string>).detail;
    if (!filename) return;

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
