<!-- src/components/MegaMenu.svelte -->
<script lang="ts">
  // CHANGED (Step B preview): menu.json is no longer a build-time static
  // import. It flows from menuDataStore, which serves the deployed
  // MASTER to everyone and the admin's IndexedDB DRAFT when logged in —
  // so edits preview live in this menu before they're committed to Git.
  // See src/lib/menuDataStore.ts.
  import { menuData } from '../lib/menuDataStore';
  import { slugify } from '../lib/slugify.ts';

  // Catalog hydration for featured lectures (mirrors FeaturedGrid).
  // One shared pipeline: getCachedCatalog() deduplicates via its promise
  // lock, so MegaMenu and FeaturedGrid never double-fetch — whichever
  // mounts first wins, the other awaits the same result.
  import TopicFeaturedCard from './TopicFeaturedCard.svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';
  import { buildTrack } from '../lib/buildTrack.js';
  
  // === TYPE DEFINITIONS ===
  
  interface HierarchyItem {
    subtopic: string;
    altName?: string;
    category: string;
  }

  // CHANGED: `url` (a page link) → `filename` (the R2 audio file).
  // Featured items are playable/queueable cards, not navigation links.
  interface FeaturedItem {
    title: string;
    speaker: string;
    filename: string;
  }

  // NEW: a sub-category node. Migrated shape is { lectures, featured? };
  // the legacy bare-array shape is still accepted for gradual migration
  // (see getLectures() below).
  interface SubCategoryNode {
    lectures?: HierarchyItem[];
    featured?: FeaturedItem[];
  }
  
  // The exact shape of the Topics subMenu after extraction.
  // CHANGED: hierarchy values are now sub-category NODES (lectures +
  // per-category featured), not plain leaf arrays. `featured` lives one
  // level lower than before — inside each sub-category — so curation
  // and navigation share a single source of truth in menu.json.
  //interface TopicsData {
  //  hierarchy: Record<string, Record<string, SubCategoryNode | HierarchyItem[]>>;
  //}

 // === EXTRACT DATA INTERNALLY ===
  // CHANGED (Step B preview): the old fail-fast `throw`s were safe when
  // menu.json was a build constant, but a runtime DRAFT swap makes them
  // reachable — a bad draft would crash the public menu. The store
  // structurally validates drafts before applying them (first line of
  // defense); this $derived + empty-object fallback is the second. A
  // malformed menu renders an empty browse bar, never a crash.
  // $menuData auto-subscribes: hierarchy recomputes on every draft
  // apply/revert, and the template re-renders from the same name.
  //
  // CHANGED (menu.json format v2): Topics was promoted from a subMenus
  // array entry to a top-level "Topics" key — the find() becomes a
  // direct property access.
  const hierarchy = $derived(
    (($menuData as any).Topics?.hierarchy as Record<string, Record<string, any>>) ?? {}
  );

  // === REACTIVE STATE (Svelte 5 syntax) ===
  
  // Type is inferred as string | null from the initial value
  let activeRoot = $state<string | null>(null);
  let activeSub = $state<string | null>(null);
  // Close delay: keeps the panel open briefly after the mouse leaves
  // the nav, so moving down into the panel doesn't close it.
  let timer: ReturnType<typeof setTimeout> | undefined;

  // NEW: open/switch delay (hover intent). When the top-level bar wraps
  // to two rows, the panel sits below BOTH rows — so the mouse must
  // cross the second row to reach it, and every crossed root button
  // would instantly steal the panel. With this delay, a root only takes
  // over if the mouse LINGERS on it; a fast transit is cancelled before
  // it fires, and the original selection survives the trip to the panel.
  let openTimer: ReturnType<typeof setTimeout> | undefined;

  // How long the mouse must rest on a top-level button before it takes
  // over the panel. 250ms: fast enough to feel responsive as a deliberate
  // hover, slow enough that a drag-to-the-panel pass (~50–100ms per
  // button) never triggers.
  const OPEN_DELAY = 250;    

  // Props — apiBase is forwarded to TopicFeaturedCard (play/download
  // need it)
  interface Props {
    apiBase?: string;
  }
  let { apiBase = '' }: Props = $props();

  // Catalog tracks (null until loaded). Only THIS is $state —
  // the hydration result is computed per-item on demand (see hydrateItem),
  // so no mirrored state is needed; template expressions re-evaluate
  // automatically when catalogTracks populates.
  let catalogTracks: any[] | null = $state(null);

  // === DATA ACCESSORS (new schema) ===

  // CHANGED: sub-category names for Column 1 — skip the reserved
  // 'featured' key so it never renders as a clickable category.
  const getSubCategories = (root: string): string[] =>
    Object.keys(hierarchy[root] ?? {}).filter(k => k !== 'featured');

  // NEW: leaf list accessor. Accepts BOTH shapes:
  //   legacy bare array  → treated as the lectures list (gradual migration)
  //   migrated node      → reads .lectures, defaulting to []
  const getLectures = (node: SubCategoryNode | HierarchyItem[] | undefined): HierarchyItem[] => {
    if (Array.isArray(node)) return node;
    return node?.lectures ?? [];
  };

  // NEW: featured items for a specific sub-category (context-sensitive
  // Column 3). Missing/empty featured arrays default to [] — sub-categories
  // without curation simply show the empty state.
  const getFeaturedItems = (root: string, sub: string): FeaturedItem[] => {
    const node = hierarchy[root]?.[sub];
    if (Array.isArray(node) || !node) return []; // legacy shape / missing node
    return node.featured ?? [];
  };

  // NEW: per-item hydration classifier (same three-state contract as
  // FeaturedGrid / the previous MegaMenu implementation):
  //   1. catalogTracks === null  → pendingHydration: true (buttons disabled)
  //   2. filename found          → full Track; menu.json title/speaker WIN
  //                                via spread order (curation over data)
  //   3. filename missing        → unavailable: true ("Lecture not available.")
  // Reads catalogTracks ($state) so the template re-renders on hydration.
  function hydrateItem(item: FeaturedItem) {
          if (catalogTracks === null) {
            return { ...item, pendingHydration: true };
          }
          const catalogItem = catalogTracks.find((t: any) => t.filename === item.filename);
          if (catalogItem) {
            // Catalog hit: buildTrack supplies hash/duration/metadata;
            // menu.json curation fields (title, speaker) win via spread order.
            return { ...buildTrack(catalogItem), ...item };
          }
          // Confirmed miss: filename not in catalog — file does not exist
          return { ...item, unavailable: true };
  }

  // Background catalog fetch, once on mount. Mirrors FeaturedGrid.
  $effect(() => {
    let cancelled = false;

    getCachedCatalog()
      .then(({ tracks }) => {
        if (cancelled) return;
        catalogTracks = tracks;

        // Admin diagnosis: warn once per missing filename across the WHOLE
        // hierarchy (featured now lives per sub-category) so stale
        // menu.json entries are discoverable in the console.
        for (const root of Object.keys(hierarchy)) {
          for (const sub of getSubCategories(root)) {
            for (const item of getFeaturedItems(root, sub)) {
            if (!tracks.find((t: any) => t.filename === item.filename)) {
              console.warn(`[MegaMenu] Featured filename not found in catalog: "${item.filename}"`);
              }
            }
          }
        }
      })
      .catch(e => {
        console.warn('[MegaMenu] Catalog enrichment failed:', e);
      });

    return () => { cancelled = true; };
  });

  // === HELPER FUNCTIONS ===
  
  const getLabel = (item: HierarchyItem): string => 
    (item.altName && item.altName.trim() !== '') ? item.altName : item.subtopic;

  // CHANGED: switching the active root is now DELAYED (hover intent).
  //   - Same root re-entered: nothing to do except cancel any pending
  //     switch — the panel must not flicker.
  //   - Different root: cancel any pending switch, keep the panel open
  //     (cancel the close timer — the mouse is back on the bar), and
  //     schedule the takeover after OPEN_DELAY. Quick passes never fire.
  const handleMouseEnter = (root: string): void => {
    if (activeRoot === root) {
      if (openTimer) { 
        clearTimeout(openTimer); 
        openTimer = undefined; 
      }
    }
    // The pointer has returned to the nav, so cancel any pending close.
    if (timer) {
      clearTimeout(timer);
      timer = undefined;
    }

    // Already showing this root; don't change the selected submenu.
    if (activeRoot === root) return;

    // First hover: open immediately so the selectable behavior is clear.
    if (activeRoot === null) {
      activeRoot = root;
      activeSub = getSubCategories(root)[0] ?? null;
      return;
    }

    // A different root is already open. Switch only if the pointer
    // pauses here; quick movement across the wrapped row won't hijack it.
    openTimer = setTimeout(() => {
      openTimer = undefined;
      activeRoot = root;
      // Default the active sub-category to the FIRST real category
      // (featured key is skipped by getSubCategories).
      activeSub = getSubCategories(root)[0] ?? null;
    }, OPEN_DELAY);
  };

  const handleMouseLeave = (): void => {
    // NEW: leaving the nav also cancels any pending switch — a takeover
    // must never fire after the mouse has gone.
    if (openTimer) { 
      clearTimeout(openTimer); 
      openTimer = undefined; 
    }
    timer = setTimeout(() => {
      activeRoot = null;
      activeSub = null;
    }, 500);
  };

  const handleHeaderHover = (): void => {
    if (openTimer) { 
      clearTimeout(openTimer); 
      openTimer = undefined; 
    }
    if (timer) clearTimeout(timer);
    activeRoot = null;
    activeSub = null;
  };
</script>

<nav class="relative bg-gray-50 border-b border-gray-200 py-4" onmouseleave={handleMouseLeave}>
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
    <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
      <button
        type="button"
        class="flex items-center space-x-2 text-black font-bold uppercase tracking-wider text-sm cursor-default bg-transparent border-none p-0"
        onmouseenter={handleHeaderHover}
      >
        <span>BROWSE LECTURES</span>
        <span>➔</span>
      </button>

      {#each Object.keys(hierarchy) as root}
        <button 
          class="uppercase text-sm font-semibold hover:text-orange-600 {activeRoot === root ? 'text-orange-600' : 'text-gray-800'}"
          onmouseenter={() => handleMouseEnter(root)}
        >
          {root}
        </button>
      {/each}
    </div>

    {#if activeRoot}
      <div
        class="absolute left-0 right-0 top-full z-40 bg-white shadow-xl border-t border-gray-100 
                    grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 
                    max-h-[70vh] md:max-h-[80vh] overflow-y-auto"
        role="menu"
        tabindex="0"
        onmouseenter={() => {
          if (openTimer) { 
            clearTimeout(openTimer);
            openTimer = undefined; 
          }
          if (timer) { 
            clearTimeout(timer); 
            timer = undefined; 
          }
        }}
      >

        <!-- COLUMN 1: Major theme's sub-categories -->
        <!-- CHANGED: getSubCategories skips the reserved 'featured' key -->
        <div class="col-span-1 bg-gray-50 p-6 border-r border-b md:border-b-0">
          {#each getSubCategories(activeRoot) as category}
            <button 
              class="py-2 w-full text-left cursor-pointer hover:text-orange-600 text-sm font-semibold {activeSub === category ? 'text-orange-600' : 'text-gray-700'}"
              onmouseenter={() => activeSub = category}
            >
              {category}
            </button>
          {/each}
        </div>

        <!-- COLUMN 2: Leaf topics of the hovered sub-category -->
        <!-- CHANGED: reads through getLectures() — supports both the new
             { lectures, featured } node shape and the legacy bare array -->
        <div class="col-span-1 p-6 border-b md:border-b-0">
          {#if activeSub}
            <h3 class="text-xs font-bold uppercase mb-4 text-gray-500 tracking-wider">{activeSub}</h3>
            <div class="flex flex-col gap-1">
              {#each getLectures(hierarchy[activeRoot][activeSub]) as item}
                <a 
                  href={`/topics/${slugify(item.category)}?label=${encodeURIComponent(getLabel(item))}`}
                  class="text-sm text-gray-700 hover:text-orange-600 py-1"
                >
                  {getLabel(item)}
                </a>
              {/each}
            </div>
          {/if}
        </div>

        <!-- COLUMN 3: Featured Lectures — CONTEXT-SENSITIVE -->
        <!-- Featured now follows the sub-category hovered in
             Column 2 (activeSub), reading that sub-category's own
             `featured` array from menu.json. Empty/missing featured
             lists show the empty state. Cards receive the hydrated item
             and manage play/queue/download via useTrackActions. -->
        <div class="col-span-2 md:col-span-2 p-6 bg-white border-t md:border-t-0 md:border-l">
          <h4 class="text-xs font-bold uppercase text-orange-600 mb-4 tracking-wider">Featured Lectures</h4>
          <div class="flex flex-col gap-4">
            {#if activeSub && getFeaturedItems(activeRoot, activeSub).length > 0}
              <!-- Compound key (`${item.filename}-${i}`) prevents Svelte 
                   runtime crash (each_key_duplicate) when draft/placeholder items 
                   share identical filenames like "xxx.mp3". -->
              {#each getFeaturedItems(activeRoot, activeSub) as item, i (`${item.filename}-${i}`)}
                <TopicFeaturedCard item={hydrateItem(item)} {apiBase} />
              {/each}
            {:else}
              <p class="text-xs text-gray-400 italic">No featured lectures for this category.</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
</nav>
