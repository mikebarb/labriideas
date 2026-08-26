<!-- src/components/FeaturedGrid.svelte -->
<!--
  NEW FILE: Svelte island for the featured lectures grid.

  Responsibilities:
  1. Catalog Hydration — resolves each menu.json item against the
     catalog (source of truth) by filename, gaining the genuine R2
     `hash`. This unlocks OPFS caching and offline hot-swap for
     featured tracks. Missing entries degrade gracefully to
     stream-only (hash: '') with a console warning for the admin.
  2. Rendering — maps hydrated items to FeaturedCard components.

  Hydration runs once on mount via $effect. getCachedCatalog() reads
  from memory/localStorage (sub-5ms) in the common case; the first-ever
  visit may briefly await a network fetch, in which case the cards
  appear when hydration completes.
-->
<script lang="ts">
  import FeaturedCard from './FeaturedCard.svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { buildTrack } from '../lib/buildTrack.js';

  interface Props {
    items: any[];       // Curation entries from menu.json
    apiBase?: string;
    isAdmin?: boolean;
  }

  let { items, apiBase = '', isAdmin = false }: Props = $props();

  // Catalog tracks (null until the background lookup completes).
  // NOTE: only THIS is $state — we no longer snapshot merged items.
  let catalogTracks: any[] | null = $state(null);

  // OPTIMISTIC + REACTIVE MERGE:
  // Renders immediately from menu.json (zero loading flash, zero layout
  // shift). When catalogTracks populates (memory/localStorage read,
  // typically <5ms), this derived silently recomputes and enriches each
  // card with the genuine R2 hash — unlocking OPFS caching for Queue.
  const hydratedItems = $derived(
    items.map(item => {
      const catalogItem = catalogTracks?.find((t: any) => t.filename === item.filename);
      if (catalogItem) {
        // Catalog hit: enrich with genuine hash + full metadata.
        // menu.json fields (image, titleBgColor, id) win via spread order.
        return { ...buildTrack(catalogItem), ...item };
      }
      // Catalog miss (or not loaded yet): stream-only fallback
      return {
      ...item,
      hash: item.hash ?? '',
      metadata: {
        title: item.title ?? '',
        artist: item.speaker ?? '',
        speaker: item.speaker ?? ''
      },
      playbackRate: 1.0
      };
    })
  );

  // Background enrichment: fetch catalog once on mount
  $effect(() => {
    let cancelled = false;

    getCachedCatalog()
      .then(({ tracks }) => {
        if (!cancelled) {
          catalogTracks = tracks;
        }
      })
      .catch(e => {
        console.warn('[Featured] Background catalog enrichment failed:', e);
      });

    return () => { cancelled = true; };
  });
</script>

  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
    {#each hydratedItems as item (item.id ?? item.filename)}
      <FeaturedCard {item} {apiBase} {isAdmin} />
    {/each}
  </div>
