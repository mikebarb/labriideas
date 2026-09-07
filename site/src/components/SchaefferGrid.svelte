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
    apiBase?: string;    // NEW: forwarded to TopicFeaturedCard
    isAdmin?: boolean;   // NEW: gates the download button
  }

  let { people, apiBase = '', isAdmin = false }: Props = $props();

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
        for (const person of Object.values(people)) {
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

<div class="grid grid-cols-1 md:grid-cols-2 gap-8 max-w-5xl mx-auto">
  {#each Object.entries(people) as [slug, person]}
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
          <TopicFeaturedCard item={hydrateItem(item, person.speakerName)} {apiBase} {isAdmin} />
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
