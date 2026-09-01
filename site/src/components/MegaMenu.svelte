<!-- src/components/MegaMenu.svelte -->
<script lang="ts">
  // Direct JSON import - Vite will bundle this client-side
  import menuData from '../data/menu.json';
  import { slugify } from '../lib/slugify.ts';

  // NEW: Catalog hydration for featured lectures (mirrors FeaturedGrid).
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
  // Featured items are now playable/queueable cards, not navigation links.
  // `filename` is resolved against the catalog at runtime to gain the
  // genuine R2 `hash` (unlocking OPFS caching / offline playback).
  interface FeaturedItem {
    title: string;
    speaker: string;
    filename: string;
  }

  interface FeaturedConfig {
    [root: string]: {
      featured: FeaturedItem[];
    };
  }

  // The exact shape of the Topics subMenu after extraction
  interface TopicsData {
    hierarchy: Record<string, Record<string, HierarchyItem[]>>;
    featured?: Record<string, FeaturedItem[]>;
  }

 // === EXTRACT DATA INTERNALLY ===
  
  const topicsSubMenu = menuData.subMenus.find(s => s.subMenu === 'Topics');
  if (!topicsSubMenu) {
    throw new Error('Configuration error: "Topics" subMenu not found in menu.json');
  }

  // Cast to our explicit TopicsData shape
  // This tells TypeScript: "trust me, this is the Topics variant"
  const topicsData = topicsSubMenu as unknown as TopicsData;
  
  if (!topicsData.hierarchy) {
    throw new Error('Configuration error: "Topics" subMenu missing hierarchy property');
  }
  
  // Now both properties are guaranteed to exist
  const hierarchy = topicsData.hierarchy;
  const featured = topicsData.featured || {};

  // These are now locally available - no props needed!
  //const hierarchy = topicsSubMenu.hierarchy as Record<string, Record<string, HierarchyItem[]>>;
  //const featured = (topicsSubMenu.featured || {}) as FeaturedConfig;
  // === REACTIVE STATE (Svelte 5 syntax) ===
  
  // Type is inferred as string | null from the initial value
  let activeRoot = $state<string | null>(null);
  let activeSub = $state<string | null>(null);
  let timer: ReturnType<typeof setTimeout> | undefined;

  // NEW: props — apiBase is forwarded to TopicFeaturedCard (play/download
  // need it); isAdmin gates the download button.
  interface Props {
    apiBase?: string;
    isAdmin?: boolean;
  }
  let { apiBase = '', isAdmin = false }: Props = $props();

  // NEW: Catalog tracks (null until loaded). Only THIS is $state —
  // the merged featured lists are a $derived overlay (FeaturedGrid pattern).
  let catalogTracks: any[] | null = $state(null);

  // NEW: OPTIMISTIC + REACTIVE HYDRATION (derived overlay).
  // Renders instantly from raw menu.json on Frame 0; silently enriches
  // once catalogTracks populates (memory/localStorage read, typically <5ms).
  //
  // Three-state classification per item:
  //   1. catalogTracks === null           → pendingHydration: true
  //      (card renders, buttons disabled — Option A)
  //   2. found in catalog                 → full Track (hash, duration);
  //      menu.json title/speaker WIN via spread order (curation over data)
  //   3. catalog loaded, filename missing → unavailable: true
  //      (card shows "Lecture not available." — the file doesn't exist)
  const featuredTracks = $derived(
    Object.fromEntries(
      Object.keys(featured).map(root => [
        root,
        featured[root].map(item => {
          // Pre-hydration: catalog not loaded yet — optimistic render
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
        })
      ])
    )
  );

  // NEW: Background catalog fetch, once on mount. Mirrors FeaturedGrid.
  $effect(() => {
    let cancelled = false;

    getCachedCatalog()
      .then(({ tracks }) => {
        if (cancelled) return;
        catalogTracks = tracks;

        // Admin diagnosis: warn once per missing filename so stale
        // menu.json entries are discoverable in the console.
        for (const root of Object.keys(featured)) {
          for (const item of featured[root]) {
            if (!tracks.find((t: any) => t.filename === item.filename)) {
              console.warn(`[MegaMenu] Featured filename not found in catalog: "${item.filename}"`);
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

  const handleMouseEnter = (root: string): void => {
    if (timer) clearTimeout(timer);
    activeRoot = root;
    activeSub = Object.keys(hierarchy[root])[0];
  };

  const handleMouseLeave = (): void => {
    timer = setTimeout(() => {
      activeRoot = null;
      activeSub = null;
    }, 500);
  };

  const handleHeaderHover = (): void => {
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
      <div class="absolute left-0 right-0 top-full z-40 bg-white shadow-xl border-t border-gray-100 
                  grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 
                  max-h-[70vh] md:max-h-[80vh] overflow-y-auto" role="menu">
        
        <!-- COLUMN 1: Categories -->
        <div class="col-span-1 bg-gray-50 p-6 border-r border-b md:border-b-0">
          {#each Object.keys(hierarchy[activeRoot]) as category}
            <button 
              class="py-2 w-full text-left cursor-pointer hover:text-orange-600 text-sm font-semibold {activeSub === category ? 'text-orange-600' : 'text-gray-700'}"
              onmouseenter={() => activeSub = category}
            >
              {category}
            </button>
          {/each}
        </div>

        <!-- COLUMN 2: Sub-Categories -->
        <div class="col-span-1 p-6 border-b md:border-b-0">
          {#if activeSub}
            <h3 class="text-xs font-bold uppercase mb-4 text-gray-500 tracking-wider">{activeSub}</h3>
            <div class="flex flex-col gap-1">
              {#each hierarchy[activeRoot][activeSub] as item}
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

        <!-- COLUMN 3: Featured Lectures -->
        <!-- CHANGED: navigation links → playable TopicFeaturedCard components.
             Cards receive the hydrated Track (or the raw pre-hydration item)
             and manage play/queue/download via useTrackActions. Keyed by
             filename for stable DOM reuse across hydration. -->
        <div class="col-span-2 md:col-span-2 p-6 bg-white border-t md:border-t-0 md:border-l">
          <h4 class="text-xs font-bold uppercase text-orange-600 mb-4 tracking-wider">Featured Lectures</h4>
          <div class="flex flex-col gap-4">
            {#if featured[activeRoot]}
              {#each featuredTracks[activeRoot] as item (item.filename)}
                <TopicFeaturedCard {item} {apiBase} {isAdmin} />
              {/each}
            {:else}
              <p class="text-xs text-gray-400 italic">No featured lectures for this section yet.</p>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
</nav>
