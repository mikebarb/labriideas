<!-- src/components/GenreTree.svelte -->
<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ChevronDown, ChevronRight, Search, Maximize2, Minimize2 } from 'lucide-svelte';

  // The genre tree is DERIVED DATA, not menu.json content — there is no
  // admin draft-preview machinery here (no menuDataStore, no isAdmin).
  // Visitors and admins see the same catalogue-derived structure.
  import { getGenreTree, type GenreTree, type GenreLeaf } from '../lib/genreIndex';
  import TrackList from './TrackList.svelte';

  interface Props {
    apiBase?: string;
  }
  let { apiBase = '' }: Props = $props();

  // ─── TREE STATE ───────────────────────────────────────────────────
  // Undefined until getGenreTree() resolves; empty groups afterwards
  // means "catalogue had no parsable genres" — an empty page, not an
  // error (rule 6 of the genreIndex contract).
  let tree = $state<GenreTree | undefined>(undefined);
  let isLoading = $state(true);

  // NOTE: no re-read on catalogue revalidation. getGenreTree() is
  // version-keyed INSIDE the module, but this component reads it once
  // on mount. A background revalidation that changes the catalogue
  // invalidates the module's memo, so any REMOUNT (page revisit) picks
  // up the new structure — while an open page keeps the snapshot it
  // loaded with, exactly like every topic page today. Keeping this
  // simple beats wiring a store subscription we don't need yet.

  // ─── OPEN/CLOSE STATE ─────────────────────────────────────────────
  // Two independent mechanisms, matching the agreed UX:
  //
  //   openSections — MAJORS only (multi-select). Tracks whether each
  //     group's subtopic list is visible. Initial state: all CLOSED,
  //     so the page opens as a pure outline of group headers.
  //
  //   openLeafId   — the SINGLE open track panel (subtopic → tracks,
  //     singular selection). One nullable key across the whole tree:
  //     opening a subtopic under any major closes the previously open
  //     panel, exactly like TrackCardGroup's expandedFilename. Key
  //     format: `${group}-${displayName}`.
  let openSections = $state<Record<string, boolean>>({});
  let openLeafId: string | null = $state(null);

  function getKey(group: string, subtopic: string) {
    return `${group}-${subtopic}`;
  }

  function toggle(id: string) {
    openSections[id] = !openSections[id];
  }

  /**
   * Subtopic → tracks is SINGULAR selection: clicking a subtopic opens
   * its track list and closes whichever other one was open (even under
   * a different major); clicking it again closes it.
   */
  function toggleLeaf(key: string) {
    openLeafId = openLeafId === key ? null : key;
  }

  /**
   * EXPAND: outline depth — every major open, every subtopic visible,
  * every track panel closed (the panel level is left to the user's
  * own singular selection).
   */
  function expandAll() {
    if (!tree) return;
    const allOpen: Record<string, boolean> = {};
    for (const group of tree.groups) {
      allOpen[group.name] = true;
    }
    openSections = allOpen;
    openLeafId = null;
  }

  /**
   * COLLAPSE: back to the pure outline — majors closed, panels closed.
   */
  function collapseAll() {
    openSections = {};
    openLeafId = null;
  }

  // ─── STRUCTURE-CHANGE REBUILD ─────────────────────────────────────
  // When `tree` is REPLACED (e.g. a future remount after revalidation),
  // majors that still exist keep their open/closed state, new ones
  // default CLOSED (the outline default), removed ones drop. The
  // single open panel is cleared if its subtopic no longer exists.
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
    // Close the open panel if its leaf vanished from the new tree.
    const stillExists = untrack(() => {
      if (!openLeafId) return true;
      const [gName, ...rest] = openLeafId.split('-');
      const sName = rest.join('-');
      return t.groups.some(
        g => g.name === gName && g.subtopics.some(s => s.displayName === sName)
      );
    });
    if (!stillExists) openLeafId = null;
  });

  // ─── LOAD ─────────────────────────────────────────────────────────
  onMount(async () => {
    try {
      tree = await getGenreTree();
      // Initial state: pure outline — majors closed, no panel open.
      openSections = {};
      openLeafId = null;
    } catch (err) {
      console.error('[GenreTree] Failed to build genre tree:', err);
    } finally {
      isLoading = false;
    }
  });
</script>

<div class="max-w-2xl mx-auto py-8 px-4">
  {#if isLoading}
    <p class="text-gray-400 italic py-4">Loading genres…</p>
  {:else if !tree}
    <p class="text-gray-400 italic py-4">Genres could not be loaded.</p>
  {:else if tree.groups.length === 0}
    <p class="text-gray-400 italic py-4">No tracks found.</p>
  {:else}

    <!-- GLOBAL EXPAND/COLLAPSE CONTROLS (same chrome as TopicsTree) -->
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

        <!-- GROUP HEADER (collapsible, like TopicsTree majors) -->
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
            {#each group.subtopics as sub (`${group.name}|${sub.displayName}`)}
              {@const sectionId = getKey(group.name, sub.displayName)}
              <div class="mb-2">
                <!-- SUBTOPIC HEADER: singular selection — opens this
                     leaf's track list and closes any other open one. -->
                <button
                  class="flex items-center gap-2 py-1.5 font-semibold text-gray-800 hover:text-orange-600 transition-colors w-full text-left"
                  onclick={() => toggleLeaf(sectionId)}
                >
                  {#if openLeafId === sectionId}
                    <ChevronDown size={16} class="text-orange-600" />
                  {:else}
                    <ChevronRight size={16} />
                  {/if}
                  <span>{sub.displayName}</span>
                </button>

                {#if openLeafId === sectionId}
                  <div class="ml-6 border-l border-gray-200 pl-3 py-1">
                    <TrackList
                      apiBase={apiBase}
                      genres={sub.rawGenres}
                    />
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
