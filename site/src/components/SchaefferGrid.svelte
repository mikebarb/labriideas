<!-- src/components/SchaefferGrid.svelte -->
<!--
  REFACTORED: upgraded from pre-refactor raw play-track buttons to the
  standard catalog-hydration + TopicFeaturedCard architecture used by
  MegaMenu / FeaturedGrid.

  Gains (identical behavior contract to those surfaces):
  - Catalog hydration: filenames resolved against the catalog at runtime,
    gaining the genuine R2 `hash` → OPFS caching + offline hot-swap work
    for these curated picks.
  - "Lecture not available." card state when a filename misses the catalog
    (visible failure at the card + console warn naming the file), instead
    of a silent downstream "Playback unavailable" in the player bar.
  - Full button states via useTrackActions: loading spinner, play/pause
    toggle, queue tick, admin download spinner.
-->
<script lang="ts">
  import { getCachedCatalog } from '../lib/catalogStore.ts';
  import { buildTrack } from '../lib/buildTrack.ts';
  import TopicFeaturedCard from './TopicFeaturedCard.svelte';
  
  // NEW (preview): reactive menu source + admin state, Option 2 hybrid.
  // people prop remains the SSR fallback; the draft overrides it only
  // for a logged-in admin with a live draft.
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  
  interface Person {
    speakerName: string;
    intro: string;
    items: Array<{
      id: string;
      title: string;
      filename: string;
    }>;
    moreLink?: string;
  }

  interface Props {
    people: Record<string, Person>;
    apiBase?: string;    // forwarded to TopicFeaturedCard
  }

  let { people, apiBase = '' }: Props = $props();

  // ─── Hybrid draft preview (same contract as FeaturedGrid.svelte) ───
  // schaefferCollection is a TOP-LEVEL key in menu.json, so the draft
  // lookup reads $menuData.schaefferCollection directly.
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's copy of the collection, resolved once and shared by the
  // banner, the diagnostic effect, and the selection. undefined when the
  // draft lacks the key (e.g. accidentally renamed — load-bearing).
  const draftCollection = $derived(
    isPreviewing ? ($menuData as any).schaefferCollection : undefined
  );

  // Diagnostic twin of the banner state — the two can never disagree.
  $effect(() => {
    if (isPreviewing && !draftCollection) {
      console.warn(
        '[SchaefferGrid] Draft preview active but "schaefferCollection" key not found in draft — rendering the deployed master. Check the key spelling in the editor.'
      );
    }
  });

  // Single source-selection point: draft people when the draft section
  // resolved, else the static people prop.
   const activePeople = $derived<Record<string, Person>>(
    isPreviewing && draftCollection ? (draftCollection.people ?? {}) : people
  );

  // Catalog tracks (null until the background lookup completes).
  // Only THIS is $state — per-item hydration is computed on demand below.
  let catalogTracks: any[] | null = $state(null);

  // Per-item hydration classifier — same three-state contract as MegaMenu:
  //   1. catalogTracks === null  → pendingHydration (buttons disabled)
  //   2. filename found          → full Track; menu.json title/speaker WIN
  //                                via spread order (curation over data)
  //   3. filename missing        → unavailable ("Lecture not available.")
  function hydrateItem(item: { id: string; title: string; filename: string }, speaker: string) {
    if (catalogTracks === null) {
      return { ...item, speaker, pendingHydration: true };
    }
    const catalogItem = catalogTracks.find((t: any) => t.filename === item.filename);
    if (catalogItem) {
      // buildTrack supplies hash/duration/metadata; spread the menu.json
      // curation fields AFTER so title/id win.
      return { ...buildTrack(catalogItem), ...item, speaker };
    }
    return { ...item, speaker, unavailable: true };
  }

  // Background catalog fetch, once on mount. Shares the same deduplicated
  // pipeline as MegaMenu/FeaturedGrid (whichever mounts first wins).
  $effect(() => {
    let cancelled = false;

    getCachedCatalog()
      .then(({ tracks }) => {
        if (cancelled) return;
        catalogTracks = tracks;

        // Admin diagnosis: warn once per missing filename.
        // CHANGED: reads activePeople (not the people prop) so the
        // diagnosis covers the DRAFT when previewing — a draft item with
        // a bad filename warns here the same way a deployed one does.
        for (const person of Object.values(activePeople)) {
          for (const item of person.items) {
            if (!tracks.find((t: any) => t.filename === item.filename)) {
              console.warn(`[Schaeffer] Featured filename not found in catalog: "${item.filename}"`);
            }
          }
        }
      })
      .catch(e => {
        console.warn('[Schaeffer] Catalog enrichment failed:', e);
      });

    return () => { cancelled = true; };
  });
</script>

{#if isPreviewing && draftCollection}
  <!-- Draft banner: the grid on screen IS the admin's uncommitted draft. -->
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if isPreviewing}
  <!-- Broken-preview banner: matches the console warn exactly. -->
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "schaefferCollection" section was not found in it — this page is showing the deployed master. Check the key spelling in the editor.
  </div>
{/if}

<div class="grid grid-cols-1 md:grid-cols-2 gap-8 max-w-5xl mx-auto">
  <!-- CHANGED: iterates activePeople (hybrid) instead of the prop; keyed
       by slug so draft reordering/edits re-render correctly. -->

  {#each Object.entries(activePeople) as [slug, person] (slug)}
    <div class="space-y-4">
      <p>
        {person.intro}
      </p>
      <h2 class="text-2xl font-bold text-gray-900">{person.speakerName}</h2>
      
      <div class="space-y-2">
        <!-- CHANGED: raw play-track buttons → TopicFeaturedCard. Compound
             key (filename-index) guards against duplicate placeholder
             filenames in menu.json, consistent with MegaMenu/FeaturedGrid. -->
        {#each person.items as item, i (`${item.filename}-${i}`)}
          <TopicFeaturedCard item={hydrateItem(item, person.speakerName)} {apiBase} />
        {/each}
      </div>

      {#if person.moreLink}
        <!-- CHANGED: `moreLink` is now WIRED rather than decorative —
             'x' (the historical placeholder) maps to the person page;
             any other value is used as the href verbatim. -->
        <a 
          href={person.moreLink && person.moreLink !== 'x' ? person.moreLink : `/schaeffer/${slug}/`}
          class="inline-block bg-orange-600 hover:bg-orange-700 text-white font-semibold px-6 py-2 rounded-md transition"
        >
          Listen to More →
        </a>
      {/if}
    </div>
  {/each}
</div>
