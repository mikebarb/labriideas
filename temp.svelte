<!-- src/components/GenreTracksView.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';

  // Full-page track view for one genre leaf (Option A: query-param page).
  //
  // ROUTE RESOLUTION IS RUNTIME-ONLY — no getStaticPaths, no draft
  // preview. The genre tree is derived from the catalogue at runtime
  // (genreIndex), so any genre the catalogue produces resolves here
  // without a rebuild, and an unknown URL lands on 'not-found' rather
  // than a 404.
  //
  // URL: /genres/view/?group=<group-slug>&sub=<subtopic-slug>
  //
  // NOTE: the state variable is `viewState`, NOT `state` — a variable
  // named `state` is indistinguishable from the `$state` rune for the
  // TS tooling and produces spurious "used before declaration" /
  // "cannot use as a store" errors. Do not rename it back.

  import TrackList from './TrackList.svelte';
  import { getGenreTree, type GenreTree, type GenreLeaf } from '../lib/genreIndex';
  import { slugify } from '../lib/slugify.ts';

  interface Props {
    apiBase?: string;
  }
  let { apiBase = '' }: Props = $props();

  // ─── RESOLUTION STATE ─────────────────────────────────────────────
  type ViewState = 'loading' | 'found' | 'not-found';
  let viewState: ViewState = $state('loading');

  // DISPLAY names (not slugs) for the breadcrumb and heading.
  let groupName = $state('');
  let subName = $state('');
  let leaf: GenreLeaf | null = $state(null);

  // ─── URL PARAMS ───────────────────────────────────────────────────
  function readParams(): { group: string; sub: string } {
    const params = new URLSearchParams(window.location.search);
    return {
      group: params.get('group') ?? '',
      sub: params.get('sub') ?? '',
    };
  }

  // ─── RESOLUTION ───────────────────────────────────────────────────
  // Mirrors how the links were generated: slugify(displayName) on both
  // sides. Returns the DISPLAY names from the tree, so renamed casing in
  // the catalogue shows correctly rather than echoing the URL slug.
  function resolve(
    tree: GenreTree,
    groupSlug: string,
    subSlug: string
  ): { groupName: string; leaf: GenreLeaf } | null {
    if (!groupSlug || !subSlug) return null;
    for (const group of tree.groups) {
      if (slugify(group.name) !== groupSlug) continue;
      for (const sub of group.subtopics) {
        if (slugify(sub.displayName) === subSlug) {
          return { groupName: group.name, leaf: sub };
        }
      }
    }
    return null;
  }

  onMount(async () => {
    try {
      const { group, sub } = readParams();
      const tree = await getGenreTree();
      const found = resolve(tree, group, sub);

      if (found) {
        groupName = found.groupName;
        subName = found.leaf.displayName;
        leaf = found.leaf;
        viewState = 'found';
      } else {
        if (group || sub) {
          console.warn(`[GenreTracksView] No genre leaf for group="${group}" sub="${sub}"`);
        }
        viewState = 'not-found';
      }
    } catch (err) {
      console.error('[GenreTracksView] Failed to build genre tree:', err);
      viewState = 'not-found';
    }
  });
</script>

<div class="max-w-2xl mx-auto py-8 px-4">
  {#if viewState === 'loading'}
    <p class="text-gray-400 italic py-4">Loading tracks…</p>
  {:else if viewState === 'not-found'}
    <nav class="text-sm text-gray-500 mb-4">
      <a href="/genres/" class="hover:text-orange-600">Genres</a>
    </nav>
    <h1 class="text-3xl font-bold mb-2">Genre not found</h1>
    <p class="text-gray-500 mb-6">
      The requested genre could not be found in the lecture library. It may
      have been renamed in the catalogue — browse the genre list to find it
      under its new name.
    </p>
    <a
      href="/genres/"
      class="inline-block px-4 py-2 bg-orange-600 hover:bg-orange-700 text-white rounded transition-colors no-underline"
    >
      Browse all genres
    </a>
  {:else}

    <!-- Breadcrumb: Genres / Group / Subtopic -->
    <nav class="text-sm text-gray-500 mb-4">
      <a href="/genres/" class="hover:text-orange-600">Genres</a>
      <span> / </span>
      <span>{groupName}</span>
      <span> / </span>
      <span class="text-gray-300">{subName}</span>
    </nav>

    <h1 class="text-3xl font-bold mb-2">{subName}</h1>

    <!-- TrackList GENRE MODE: the leaf's raw genre strings — every
         collapsed spelling variant — matched against the catalogue's
         genre field (union semantics, case-insensitive). -->
    {#if leaf}
      <TrackList apiBase={apiBase} genres={leaf.rawGenres} />
    {/if}
  {/if}
</div>
