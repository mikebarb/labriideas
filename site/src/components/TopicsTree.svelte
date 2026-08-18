<!-- src/components/TopicsTree.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';
  import menuData from '../data/menu.json';
  import { slugify } from '../lib/slugify.ts';

  interface LeafItem {
    subtopic: string;
    altName?: string;
    category: string;
  }

  // Find the Topics menu (this returns 'SubMenu | undefined')
  const topicsSubMenu = menuData.subMenus.find(s => s.subMenu === 'Topics');
  
  // Guard clause: if menu.json is malformed, throw a build error
  if (!topicsSubMenu || !topicsSubMenu.hierarchy) {
    throw new Error('Configuration error: "Topics" subMenu with hierarchy not found in menu.json');
  }

  // Now TypeScript knows hierarchy exists
  const hierarchy = topicsSubMenu.hierarchy;

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
      for (const minorTheme of Object.keys(minorMap)) {
        allOpen[getKey(majorTheme, minorTheme)] = true; // Open the minor
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
          {#each Object.entries(minorMap) as [minorTheme, leaves]}
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
                  {#each leaves as item}
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
