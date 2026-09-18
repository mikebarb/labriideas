<!-- src/components/TopicsTree.svelte -->
<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';
  // CHANGED (Step B preview): build-time static import → reactive store.
  // Serves the deployed master normally; the admin's draft when logged
  // in — so the tree previews draft hierarchy changes live.
  // See src/lib/menuDataStore.ts.
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';
  // PERMANENT (Option 2 hybrid): the deployed master, used as the
  // fallback when a broken draft omits the Topics section. Not legacy —
  // do not remove in a future "unused imports" sweep.
  import masterMenu from '../data/menu.json';
  import { slugify } from '../lib/slugify.ts';

  
  interface LeafItem {
    subtopic: string;
    altName?: string;
    category: string;
  }

  // NEW: sub-category node shape after the menu.json rework.
  // Migrated: { lectures, featured? }. Legacy bare arrays are still
  // accepted so the JSON can be migrated section by section.
  type SubCategoryNode = { lectures?: LeafItem[]; featured?: unknown[] } | LeafItem[];

  // NEW: leaf accessor — accepts both shapes, defaults to [].
  function getLeaves(node: SubCategoryNode | undefined): LeafItem[] {
    if (Array.isArray(node)) return node;
    return node?.lectures ?? [];
  }

  // CHANGED (Step B preview): old guard threw at component init — safe
  // for a build-time constant, but a runtime draft swap makes it
  // reachable. The store validates drafts before applying them; this
  // fallback renders an empty tree rather than crashing if something
  // still slips through. $derived: re-derives on every draft apply.
  //const hierarchy = $derived(
  //  (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Topics')
  //    ?.hierarchy as Record<string, Record<string, any>>) ?? {}
  //);

  // CHANGED (preview): draft-preview state + honest fallback.
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's Topics section — used ONLY for the banner/diagnostic
  // state (hierarchy below reads $menuData directly).
  const draftTopics = $derived(
    isPreviewing
      ? (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Topics') as any)
      : undefined
  );

  // Diagnostic twin of the banner state — the two can never disagree.
  $effect(() => {
    if (isPreviewing && !draftTopics) {
      console.warn(
        '[TopicsTree] Draft preview active but "Topics" subMenu not found in draft — rendering the deployed master. Check the subMenu key spelling in the editor.'
      );
    }
  });

    // CHANGED (preview): the old fallback was `?? {}` — an EMPTY tree when
  // the draft lacked Topics, which is a broken page, not a degraded one.
  // Now the fallback chain is: draft (when previewing) → deployed master
  // → empty object. $menuData IS the master when not previewing, so the
  // masterMenu fallback only engages in the broken-draft case.
  // TYPED explicitly (same reason as SchaefferGrid): the any-typed draft
  // branch would otherwise widen the ternary and lose the record typing.
  const hierarchy = $derived<Record<string, Record<string, any>>>(
    (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Topics')?.hierarchy
      ?? (masterMenu as any).subMenus.find((s: any) => s.subMenu === 'Topics')?.hierarchy) ?? {}
  );

  // ============================================================================
  // STATE PERSISTENCE (sessionStorage)
  // ----------------------------------------------------------------------------
  // This component lives on the /topics index page (Astro MPA). Navigating to a
  // topic page destroys this component, and the browser's back button rebuilds
  // it fresh — losing both the expansion state and scroll position.
  //
  // To fix this, we persist:
  //   - openSections   -> sessionStorage key below
  //   - scroll position -> sessionStorage scroll key below (saved when a leaf
  //     link is clicked, restored on mount)
  //
  // IMPORTANT: sessionStorage only exists in the browser. All reads/writes
  // MUST stay inside onMount / $effect / event handlers (client-side only).
  // Do NOT hoist any of these calls into the top level of this script, or the
  // Astro build will fail with "ReferenceError: sessionStorage is not defined".
  // ============================================================================

  const OPEN_SECTIONS_KEY = 'labri:topics:openSections';
  const SCROLL_Y_KEY = 'labri:topics:scrollY';

  // Ref to this component's root element (bound in the template below).
  // Used to locate the element that ACTUALLY scrolls the page — the layout
  // may scroll an inner container (e.g. a div with overflow-y-auto) rather
  // than the document, in which case window.scrollY is always 0.
  let treeEl: HTMLDivElement;

  /** Reads persisted expansion state. Returns null if absent or corrupt. */
  function loadOpenSections(): Record<string, boolean> | null {
    try {
      const raw = sessionStorage.getItem(OPEN_SECTIONS_KEY);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      // Sanity check: must be a plain object, not an array/null/primitive
      if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
        return parsed;
      }
      return null;
    } catch {
      // Corrupt JSON or storage unavailable (e.g. private mode) — fall back silently
      return null;
    }
  }

  /** Writes current expansion state to sessionStorage. */
  function saveOpenSections(state: Record<string, boolean>) {
    try {
      sessionStorage.setItem(OPEN_SECTIONS_KEY, JSON.stringify(state));
    } catch {
      // Storage full/unavailable — non-fatal, page still works
    }
  }

  /**
   * Finds the element that actually scrolls this page. Walks up from the
   * tree's root to the first ancestor with a scrollable vertical overflow.
   * Falls back to window if the document itself is the scroller.
   * (Client-side only — call from event handlers / onMount, never top level.)
   */
  function getScroller(): HTMLElement | Window {
    let el: HTMLElement | null = treeEl;
    while (el) {
      const style = getComputedStyle(el);
      if (/(auto|scroll)/.test(style.overflowY) && el.scrollHeight > el.clientHeight) {
        return el;
      }
      el = el.parentElement;
    }
    return window;
  }

  /** Current scroll position of whichever element scrolls the page. */
  function readScrollY(): number {
    const scroller = getScroller();
    return scroller === window ? window.scrollY : (scroller as HTMLElement).scrollTop;
  }

  /** Sets the scroll position on whichever element scrolls the page. */
  function applyScrollY(y: number) {
    const scroller = getScroller();
    if (scroller === window) {
      // 'instant' is critical: if global CSS sets scroll-behavior: smooth,
      // a plain scrollTo animates and gets canceled by layout shifts.
      window.scrollTo({ top: y, left: 0, behavior: 'instant' as ScrollBehavior });
    } else {
      // Direct property set on the container: always instant, never animated
      (scroller as HTMLElement).scrollTop = y;
    }
  }

  /**
   * Called when a leaf topic link is clicked, just before navigation.
   * Snapshots the scroll position so we can restore it if the user
   * returns via the browser back button.
   */
  function handleTopicClick() {
    try {
      sessionStorage.setItem(SCROLL_Y_KEY, String(readScrollY()));
    } catch {
      // Non-fatal
    }
  }
  // ============================================================================
  // END STATE PERSISTENCE
  // ============================================================================

  // Track open state for BOTH minor themes (under majors) and the major themes themselves
  // Define a helper to generate the default state object
  const initialOpenState = () => {
    const state: Record<string, boolean> = {};
    for (const majorTheme of Object.keys(hierarchy)) {
      state[majorTheme] = true;
    }
    return state;
  };
  // Initialize the state directly with the generated object
  let openSections = $state(initialOpenState());

  // NEW (Step B preview): when the hierarchy changes identity (a draft
  // applied or reverted — NOT a section toggle), rebuild the expansion
  // map for the new structure: sections that still exist keep their
  // open/closed state, new sections default open, removed sections drop.
  //
  // Mechanics notes:
  //  - lastHierarchy is a PLAIN variable (not $state) — an identity
  //    sentinel so routine re-derives of the same object don't trigger
  //    a rebuild.
  //  - openSections is read through untrack() — reading it as a
  //    dependency while writing it here would loop the effect.
  //  - Toggling a section mutates openSections but NOT hierarchy, so
  //    this effect correctly stays dormant during normal browsing.
  //  - On first run (mount) this simply reproduces initialOpenState(),
  //    so the pre-existing onMount sessionStorage restore behaves
  //    exactly as before.
  let lastHierarchy: unknown = null;
  $effect(() => {
    const h = hierarchy;
    if (untrack(() => lastHierarchy) === h) return;
    lastHierarchy = h;
    const prev = untrack(() => openSections);
    const next: Record<string, boolean> = {};
    for (const major of Object.keys(h)) {
      next[major] = prev[major] ?? true;
      for (const minor of Object.keys(h[major])) {
        if (minor === 'featured') continue;
        const key = `${major}-${minor}`;
        next[key] = prev[key] ?? true;
      }
    }
    openSections = next;
  });

  // ============================================================================
  // CLIENT-SIDE RESTORE
  // ----------------------------------------------------------------------------
  // onMount runs ONLY in the browser, AFTER hydration. Here we:
  //   1. Override openSections with any persisted state (falling back to the
  //      default "all majors open" from initialOpenState above).
  //   2. Restore the saved scroll position, if one exists.
  //      - We clear the key only AFTER the position sticks, so that a *fresh*
  //        visit to /topics (typed URL, nav menu link) starts at the top as
  //        normal. Only a return-via-back-button (which re-runs this mount on
  //        the page the user left from a topic click) sees a saved position.
  // ============================================================================
    onMount(() => {
    // Restore expansion state
    const saved = loadOpenSections();
    if (saved) {
      openSections = saved;
    }

    // Restore scroll position (if the user left via a topic link)
    const savedScroll = sessionStorage.getItem(SCROLL_Y_KEY);

    if (savedScroll === null) return;

    const target = Number(savedScroll);
    

    // 1. Take control away from the browser's built-in scroll restoration,
    //    which otherwise races with us on back-navigations and wins.
    if ('scrollRestoration' in history) {
      history.scrollRestoration = 'manual';
    }

    // 2. Retry loop: the page may still be growing (fonts, images, hydration),
    //    so a single scroll can land short or get overwritten. We keep
    //    retrying until the position sticks, and only THEN consume the key.
    let attempts = 0;
    const tryRestore = () => {
      attempts++;
      applyScrollY(target);
                  
      if (Math.abs(readScrollY() - target) > 2 && attempts < 15) {
        // Position didn't stick (page still too short, or browser overrode us)
        // — wait two frames and try again.
        requestAnimationFrame(() => requestAnimationFrame(tryRestore));
      } else {
        // Success (or gave up after ~15 frames ≈ 250ms): consume the key so a
        // fresh visit to /topics starts at the top as normal.
        sessionStorage.removeItem(SCROLL_Y_KEY);
      }
    };
    requestAnimationFrame(tryRestore);
  });

  // ============================================================================
  // AUTO-SAVE EXPANSION STATE
  // ----------------------------------------------------------------------------
  // $effect runs client-side only and re-runs whenever `openSections` changes.
  // Because openSections is a $state proxy, mutating a key (toggle) or
  // replacing the whole object (expandAll/collapseAll) both trigger this.
  // Note: this also fires once immediately after mount, persisting whatever
  // state is active (restored or default) — which is harmless and keeps the
  // stored value consistent.
  // ============================================================================
  $effect(() => {
    saveOpenSections($state.snapshot(openSections) as Record<string, boolean>);
  });
  // ============================================================================
  // END CLIENT-SIDE RESTORE / AUTO-SAVE
  // ============================================================================

  function toggle(id: string) {
    openSections[id] = !openSections[id];
  }

  /**
   * Generates the key used in openSections for a major/minor pair.
   * Example: "Theology & Worldview-L'Abri"
   */
  function getKey(major: string, minor: string) {
    return `${major}-${minor}`;
  }

  function expandAll() {
    const allOpen: Record<string, boolean> = {};
    for (const [majorTheme, minorMap] of Object.entries(hierarchy)) {
      allOpen[majorTheme] = true; // Open the major
      // CHANGED: skip 'featured' — it's curation data inside the node,
      // not a navigable sub-category, and must not gain a tree section.
      for (const minorTheme of Object.keys(minorMap)) {
        if (minorTheme === 'featured') continue;
        allOpen[getKey(majorTheme, minorTheme)] = true;
      }
    }
    openSections = allOpen;
  }

  function collapseAll() {
    openSections = {};
  }

  // Pure URL generator (no side effects) that encodes both category and label
  function getTopicUrl(item: LeafItem): string {
    return `/topics/${slugify(item.category)}`;
  }

</script>

<!-- bind:this gives the script a handle on this root element so it can walk
     up the DOM to find the real scrolling container (see getScroller above) -->
<div class="max-w-2xl mx-auto py-8 px-4" bind:this={treeEl}>

  {#if isPreviewing && draftTopics}
    <!-- Draft banner: the tree on screen IS the admin's uncommitted draft. -->
    <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
      ✎ Previewing your uncommitted draft — public visitors see the deployed version.
    </div>
  {:else if isPreviewing}
    <!-- Broken-preview banner: matches the console warn exactly. -->
    <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
      ⚠ A draft is active, but the "Topics" section was not found in it — this page is showing the deployed master. Check the subMenu key spelling in the editor.
    </div>
  {/if}
  
  <!-- GLOBAL EXPAND/COLLAPSE CONTROLS -->
  <div class="flex gap-2 mb-6 pb-4 border-b border-gray-200">
    <button
      type="button"
      onclick={expandAll}
      class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
      title="Expand all categories"
    >
      <Maximize2 size={14} />
      Expand All
    </button>
    <button
      type="button"
      onclick={collapseAll}
      class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
      title="Collapse all categories"
    >
      <Minimize2 size={14} />
      Collapse All
    </button>
  </div>

  {#each Object.entries(hierarchy) as [majorTheme, minorMap]}
    <div class="mb-6">
      
      <!-- MAJOR THEME HEADER (Now also collapsible) -->
      <button
        type="button"
        class="flex items-center gap-2 w-full text-left py-2 mb-1"
        onclick={() => toggle(majorTheme)}
      >
        {#if openSections[majorTheme]}
          <ChevronDown size={20} class="text-orange-600 shrink-0" />
        {:else}
          <ChevronRight size={20} class="text-gray-600 shrink-0" />
        {/if}
        <h2 class="text-2xl font-bold text-gray-900">
          {majorTheme}
        </h2>
      </button>
      
      <!-- MAJOR THEME CONTENT (Only shows when expanded) -->
      {#if openSections[majorTheme]}
        <div class="ml-6 border-l-2 border-gray-200 pl-4">
          {#each Object.entries(minorMap).filter(([k]) => k !== 'featured') as [minorTheme, node]}
            {@const sectionId = getKey(majorTheme, minorTheme)}
            <div class="mb-2">
              <button 
                class="flex items-center gap-2 py-1.5 font-semibold text-gray-800 hover:text-orange-600 transition-colors w-full text-left"
                onclick={() => toggle(sectionId)}
              >
                {#if openSections[sectionId]}
                  <ChevronDown size={16} class="text-orange-600" />
                {:else}
                  <ChevronRight size={16} />
                {/if}
                <span>{minorTheme}</span>
              </button>
              {#if openSections[sectionId]}
                <ul class="ml-6 border-l border-gray-200 pl-3 py-1">
                  {#each getLeaves(node as SubCategoryNode) as item}
                    <li>
                      <!-- onclick snapshot: saves the scroll position before the
                           browser navigates away. Does NOT preventDefault, so
                           the normal href navigation proceeds unchanged. -->
                      <a
                        class="flex items-center gap-3 py-1.5 px-2 text-sm text-gray-600 hover:text-orange-600 hover:bg-gray-50 rounded w-full text-left transition-colors no-underline"
                        href={getTopicUrl(item)}
                        onclick={handleTopicClick}
                        title={`Open track list for: ${item.subtopic}`}
                      >
                        <Search size={14} class="opacity-50 shrink-0" />
                        <span>{item.subtopic}</span>
                      </a>
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>
