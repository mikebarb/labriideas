<!-- src/components/GenreTree.svelte -->
<script lang="ts">
  import { onMount, untrack, tick } from 'svelte';
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';

  // The genre tree is DERIVED DATA, not menu.json content — there is no
  // admin draft-preview machinery here (no menuDataStore, no isAdmin).
  // Visitors and admins see the same catalogue-derived structure.
  //
  // CHANGED (full-page track view): leaf clicks now NAVIGATE to
  // /genres/view/?group=…&sub=… — the same UX as TopicsTree → topic
  // pages — instead of expanding a TrackList in place. That restored
  // the two problems TopicsTree already solved, so its state machinery
  // returns below: expansion persistence + scroll restore across the
  // navigate-away/back cycle.
  import { getGenreTree, type GenreTree } from '../lib/genreIndex';
  import { slugify } from '../lib/slugify.ts';

  // ─── TREE STATE ───────────────────────────────────────────────────
  // Undefined until getGenreTree() resolves; empty groups afterwards
  // means "catalogue had no parsable genres" — an empty page, not an
  // error (rule 6 of the genreIndex contract).
  let tree = $state<GenreTree | undefined>(undefined);
  let isLoading = $state(true);

  // ─── OPEN/CLOSE STATE ─────────────────────────────────────────────
  // MAJORS ONLY, multi-select: any number of groups open at once,
  // independently. Initial state: all closed — the page opens as a
  // pure outline of group headers (agreed default).
  //
  // PERSISTED (restored): because leaf links navigate away and the
  // browser's back button rebuilds this component fresh, expansion
  // state lives in sessionStorage — same pattern as TopicsTree.
  let openSections = $state<Record<string, boolean>>({});

  function toggle(id: string) {
    openSections[id] = !openSections[id];
  }

  /** EXPAND: every major open (subtopics visible, panels are on the view page). */
  function expandAll() {
    if (!tree) return;
    const allOpen: Record<string, boolean> = {};
    for (const group of tree.groups) {
      allOpen[group.name] = true;
    }
    openSections = allOpen;
  }

  /** COLLAPSE: back to the pure outline — all majors closed. */
  function collapseAll() {
    openSections = {};
  }

  // ============================================================================
  // STATE PERSISTENCE (sessionStorage)
  // ----------------------------------------------------------------------------
  // Same rationale as TopicsTree: this island lives on the /genres index
  // page (Astro MPA). Clicking a leaf navigates to /genres/view/…, which
  // DESTROYS this component; the browser's back button rebuilds it fresh,
  // losing both expansion state and scroll position.
  //
  // Persisted:
  //   - openSections  -> OPEN_SECTIONS_KEY
  //   - scroll position -> SCROLL_Y_KEY (saved on leaf click, restored on mount)
  //
  // IMPORTANT: sessionStorage only exists in the browser. All reads/writes
  // MUST stay inside onMount / $effect / event handlers (client-side only).
  // Do NOT hoist any of these calls into the top level of this script, or
  // the Astro build will fail with "ReferenceError: sessionStorage is not
  // defined".
  // ============================================================================

  const OPEN_SECTIONS_KEY = 'labri:genres:openSections';
  const SCROLL_Y_KEY = 'labri:genres:scrollY';

  // Ref to this component's root element (bound in the template below).
  // Used to locate the element that ACTUALLY scrolls the page — the layout
  // may scroll an inner container rather than the document.
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
   * Called when a genre leaf link is clicked, just before navigation.
   * Snapshots the scroll position so we can restore it if the user
   * returns via the browser back button.
   */
  function handleLeafClick() {
    try {
      sessionStorage.setItem(SCROLL_Y_KEY, String(readScrollY()));
    } catch {
      // Non-fatal
    }
  }
  // ============================================================================
  // END STATE PERSISTENCE
  // ============================================================================

  // ─── STRUCTURE-CHANGE REBUILD ─────────────────────────────────────
  // When `tree` is REPLACED (a future re-read after revalidation),
  // groups that still exist keep their open/closed state, new ones
  // default CLOSED (the outline default), removed ones drop. Same
  // sentinel pattern as TopicsTree — a plain identity check so routine
  // re-derivations don't trigger it, and untrack() so writing
  // openSections here doesn't loop the effect.
  let lastTree: unknown = null;
  $effect(() => {
    const t = tree;
    if (untrack(() => lastTree) === t) return;
    lastTree = t;
    if (!t) return;
    const prev = untrack(() => openSections);
    const next: Record<string, boolean> = {};
    for (const group of t.groups) {
      next[group.name] = prev[group.name] ?? false;
    }
    openSections = next;
  });

  // ─── AUTO-SAVE EXPANSION STATE ────────────────────────────────────
  // Gated by persistenceActive: this effect is declared BEFORE onMount
  // and therefore runs first, while openSections is still the empty
  // initial state. Without the gate it writes {} over the saved value
  // before onMount can read it — which is exactly why the expansion
  // state was coming back collapsed. Plain (non-$state) variable: it is
  // written once, after the restore, and we save explicitly at that
  // point anyway.
  let persistenceActive = false;

  $effect(() => {
    const snapshot = $state.snapshot(openSections) as Record<string, boolean>;
    if (!persistenceActive) return;
    saveOpenSections(snapshot);
  });

  // ─── LOAD + CLIENT-SIDE RESTORE ───────────────────────────────────
  onMount(async () => {
    // Take scroll restoration away from the browser IMMEDIATELY, before
    // any await. If this is deferred until after the catalogue resolves,
    // the browser's own back-navigation restoration has already run and
    // fights ours.
    if ('scrollRestoration' in history) {
      history.scrollRestoration = 'manual';
    }

    // Restore expansion state before the tree arrives, so the outline
    // renders already in the user's previous shape. The structure-change
    // effect below preserves these values when `tree` lands (it reads
    // them as `prev`).
    const saved = loadOpenSections();
    if (saved) {
      openSections = saved;
    }

    // Load the derived tree (lazy, version-keyed memo inside genreIndex).
    try {
      tree = await getGenreTree();
    } catch (err) {
      console.error('[GenreTree] Failed to build genre tree:', err);
      // tree stays undefined → error state below
    } finally {
      isLoading = false;
    }

    // From here on, mutations of openSections are persisted.
    persistenceActive = true;
    saveOpenSections($state.snapshot(openSections) as Record<string, boolean>);

    // Scroll restore LAST, and only after the outline has actually been
    // written to the DOM — otherwise the page is still short and the
    // target position is unreachable (the retry loop gives up and eats
    // the key).
    await tick();
    restoreScrollIfAny();
  });

  /**
   * Restores the saved scroll position, if the user left via a leaf
   * link. Called AFTER the tree has rendered, so the page is at its
   * real height.
   */
  function restoreScrollIfAny() {
    const savedScroll = sessionStorage.getItem(SCROLL_Y_KEY);
    if (savedScroll === null) return;

    const target = Number(savedScroll);

    let attempts = 0;
    const tryRestore = () => {
      attempts++;
      applyScrollY(target);

      // 30 attempts (~500ms) rather than 15: fonts and images can still
      // shift the page after the tree renders.
      if (Math.abs(readScrollY() - target) > 2 && attempts < 30) {
        requestAnimationFrame(() => requestAnimationFrame(tryRestore));
      } else {
        sessionStorage.removeItem(SCROLL_Y_KEY);
      }
    };
    requestAnimationFrame(tryRestore);
  }

  /**
   * Pure URL generator (no side effects) for a genre leaf:
   * /genres/view/?group=<group-slug>&sub=<subtopic-slug>
   * The view page re-derives the tree at runtime and resolves these
   * slugs against it — so new genres work with no rebuild.
   */
  function getGenreUrl(group: string, subtopic: string): string {
    return `/genres/view/?group=${encodeURIComponent(slugify(group))}&sub=${encodeURIComponent(slugify(subtopic))}`;
  }
</script>

<!-- bind:this gives the script a handle on this root element so it can walk
     up the DOM to find the real scrolling container (see getScroller above) -->
<div class="max-w-2xl mx-auto py-8 px-4" bind:this={treeEl}>

  {#if isLoading}
    <p class="text-gray-400 italic py-4">Loading genres…</p>
  {:else if !tree}
    <p class="text-gray-400 italic py-4">Genres could not be loaded.</p>
  {:else if tree.groups.length === 0}
    <p class="text-gray-400 italic py-4">No tracks found.</p>
  {:else}

    <!-- GLOBAL EXPAND/COLLAPSE CONTROLS (outline depth, same chrome as TopicsTree) -->
    <div class="flex gap-2 mb-6 pb-4 border-b border-gray-200">
      <button
        type="button"
        onclick={expandAll}
        class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
        title="Show all subtopic lists"
      >
        <Maximize2 size={14} />
        Expand
      </button>
      <button
        type="button"
        onclick={collapseAll}
        class="flex items-center gap-1 px-3 py-1.5 text-sm bg-gray-100 hover:bg-gray-200 text-gray-700 rounded transition-colors"
        title="Collapse to the group outline"
      >
        <Minimize2 size={14} />
        Collapse
      </button>
    </div>

    {#each tree.groups as group (group.name)}
      <div class="mb-6">

        <!-- GROUP HEADER (collapsible, multi-select, like TopicsTree majors) -->
        <button
          type="button"
          class="flex items-center gap-2 w-full text-left py-2 mb-1"
          onclick={() => toggle(group.name)}
        >
          {#if openSections[group.name]}
            <ChevronDown size={20} class="text-orange-600 shrink-0" />
          {:else}
            <ChevronRight size={20} class="text-gray-600 shrink-0" />
          {/if}
          <h2 class="text-2xl font-bold text-gray-900">
            {group.name}
          </h2>
        </button>

        <!-- GROUP CONTENT (only when expanded) -->
        {#if openSections[group.name]}
          <div class="ml-6 border-l-2 border-gray-200 pl-4">
            <ul class="ml-6 border-l border-gray-200 pl-3 py-1">
              {#each group.subtopics as sub (`${group.name}|${sub.displayName}`)}
                <li>
                  <!-- Leaf link: navigates to the full-page track view,
                       same UX as TopicsTree → topic pages. onclick
                       snapshots the scroll position before the browser
                       navigates away; does NOT preventDefault, so the
                       normal href navigation proceeds unchanged. -->
                  <a
                    class="flex items-center gap-3 py-1.5 px-2 text-sm text-gray-600 hover:text-orange-600 hover:bg-gray-50 rounded w-full text-left transition-colors no-underline"
                    href={getGenreUrl(group.name, sub.displayName)}
                    onclick={handleLeafClick}
                    title={`Open track list for: ${sub.displayName}`}
                  >
                    <Search size={14} class="opacity-50 shrink-0" />
                    <span>{sub.displayName}</span>
                  </a>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
