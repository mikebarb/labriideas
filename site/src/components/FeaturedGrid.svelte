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
  // NEW (preview): reactive menu source + admin state, per the Option 2
  // hybrid pattern. items prop remains the SSR fallback; the draft
  // overrides it only for a logged-in admin with a live draft.
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  interface Props {
    items: any[];       // Curation entries from menu.json (build-time
                        // extraction — the SSR seed and non-admin fallback)
    apiBase?: string;
  }

  let { items, apiBase = '' }: Props = $props();

  // ─── Hybrid draft preview (Option 2 pattern, same contract as
  //      ContactSection.svelte) ───
  // featuredLectures is a TOP-LEVEL key in menu.json (not a subMenus
  // entry), so the draft lookup reads $menuData.featuredLectures directly.
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's copy of the featuredLectures section, resolved once and
  // shared by the banner, the diagnostic effect, and the selection.
  // undefined when the draft lacks the key entirely (e.g. it was
  // accidentally renamed — a load-bearing identifier, same class of
  // error as the Contact subMenu key).
  const draftFeatured = $derived(
    isPreviewing ? ($menuData as any).featuredLectures : undefined
  );

  // Diagnostic twin of the banner state — the two can never disagree.
  $effect(() => {
    if (isPreviewing && !draftFeatured) {
      console.warn(
        '[FeaturedGrid] Draft preview active but "featuredLectures" key not found in draft — rendering the deployed master. Check the key spelling in the editor.'
      );
    }
  });

  // Single source-selection point: draft items when the draft section
  // resolved, else the static items prop.
  const activeItems = $derived(
    isPreviewing && draftFeatured ? (draftFeatured.items ?? []) : items
  );

  // Catalog tracks (null until the background lookup completes).
  // NOTE: only THIS is $state — we no longer snapshot merged items.
  let catalogTracks: any[] | null = $state(null);

  // OPTIMISTIC + REACTIVE MERGE:
  // Renders immediately from the ACTIVE source (zero loading flash,
  // zero layout shift). When catalogTracks populates (memory/localStorage
  // read, typically <5ms), this derived silently recomputes and enriches
  // each card with the genuine R2 hash — unlocking OPFS caching for Queue.
  // CHANGED: reads activeItems (hybrid) instead of items — the merge now
  // recomputes when a draft is applied/reverted, as well as when the
  // catalog loads.
  const hydratedItems = $derived(
    activeItems.map(item => {
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

{#if isPreviewing && draftFeatured}
  <!-- Draft banner: the grid on screen IS the admin's uncommitted draft. -->
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if isPreviewing}
  <!-- Broken-preview banner: a draft IS active, but this section could
       not be found in it — the page is showing the deployed MASTER.
       Matches the console warn exactly. -->
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "featuredLectures" section was not found in it — this page is showing the deployed master. Check the key spelling in the editor.
  </div>
{/if}

  <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
    <!-- Compound fallback key (`${item.filename}-${i}`) ensures 
         draft/placeholder lectures without distinct IDs never collide. -->
    {#each hydratedItems as item, i (item.id ?? `${item.filename}-${i}`)}
      <FeaturedCard {item} {apiBase} />
    {/each}
  </div>
