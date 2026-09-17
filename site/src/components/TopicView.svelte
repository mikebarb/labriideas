<!-- src/components/TopicView.svelte -->
<script lang="ts">
  // Hybrid "Preview Island" for the topic detail routes (Option 2,
  // four-state contract, same as AlbumView / SchaefferPersonView).
  //
  // ROUTE EXISTENCE is a build-time decision (getStaticPaths in
  // [...topic].astro). This island re-resolves ONLY the current route
  // against the draft hierarchy at runtime, replicating the build's
  // slug-matching for one topic string:
  //   - major route (isMajor)        → draft root with matching slug
  //   - nested (two-segment) route   → draft root + category slugs
  //   - flat route                   → draft leaf category slug first,
  //                                     then structural category slug
  //     (same precedence as the build: leaves win over structural.)
  //
  // LIMITATION (inherent to static output): a draft's NEW categories/
  // leaves have no route until the next CI build — TopicsTree's links
  // to them 404. A draft that renames/removes the content behind this
  // route shows state 'removed' with the master for reference.

  import TrackList from './TrackList.svelte';
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';
  import { slugify } from '../lib/slugify.ts';

  interface Props {
    // Route param — the URL's source of truth (e.g. "theology-worldview"
    // or "theology-worldview/christian-ethics").
    topic: string;
    // Build-time props (SSR seed / permanent fallback).
    topicName: string;
    filterCategories: string[];
    isMajor: boolean;
    majorSlug: string | null;
    majorLabel: string | null;
    subCategories: Array<{ name: string; slug: string }>;
    apiBase?: string;
  }
  let {
    topic, topicName, filterCategories, isMajor, majorSlug, majorLabel,
    subCategories, apiBase = '',
  }: Props = $props();

  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's Topics hierarchy (undefined when the subMenu key itself
  // is missing/broken — distinct from this route's content being gone).
  const draftHierarchy = $derived(
    isPreviewing
      ? (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Topics')
          ?.hierarchy as Record<string, Record<string, any>> | undefined)
      : undefined
  );

  // ── Draft resolution helpers (mirror the build's node-shape logic) ──
  const leavesOf = (node: any): any[] =>
    Array.isArray(node) ? node : (node?.lectures ?? []);

  const collectLeafCategories = (node: any): string[] => {
    const out: string[] = [];
    for (const leaf of leavesOf(node)) {
      if (leaf?.category && !out.includes(leaf.category)) out.push(leaf.category);
    }
    return out;
  };

  // Re-resolve THIS route against the draft. Mirrors getStaticPaths'
  // matching rules for one topic string:
  //   returns matched props  → draft content
  //   returns null           → route's content removed/renamed in draft
  //   (draftHierarchy undefined is handled separately — section-missing.)
  function resolveDraft(): { topicName: string; filterCategories: string[]; subCategories: Array<{ name: string; slug: string }> } | null {
    if (!draftHierarchy) return null;

    // Major-theme route: match by the ROOT slug.
    if (isMajor) {
      for (const root of Object.keys(draftHierarchy)) {
        if (slugify(root) !== majorSlug) continue;
        const cats = Object.keys(draftHierarchy[root]).filter(k => k !== 'featured');
        const filter: string[] = [];
        for (const c of cats) {
          for (const x of collectLeafCategories(draftHierarchy[root][c])) {
            if (!filter.includes(x)) filter.push(x);
          }
        }
        return {
          topicName: root,
          filterCategories: filter,
          subCategories: cats.map(name => ({ name, slug: slugify(name) })),
        };
      }
      return null;
    }

    // Nested route (two segments): match root slug + category slug.
    if (topic.includes('/')) {
      const [rootSlug, catSlug] = topic.split('/');
      for (const root of Object.keys(draftHierarchy)) {
        if (slugify(root) !== rootSlug) continue;
        for (const cat of Object.keys(draftHierarchy[root])) {
          if (cat === 'featured' || slugify(cat) !== catSlug) continue;
          const leaves = leavesOf(draftHierarchy[root][cat]);
          return {
            topicName: (leaves[0] && leaves[0].subtopic) || cat,
            filterCategories: collectLeafCategories(draftHierarchy[root][cat]),
            subCategories: [],
          };
        }
      }
      return null;
    }

    // Flat route: leaf categories take precedence, then structural —
    // identical to the build's leafSlugs filter + final claim loop.
    for (const root of Object.keys(draftHierarchy)) {
      for (const cat of Object.keys(draftHierarchy[root])) {
        if (cat === 'featured') continue;
        for (const leaf of leavesOf(draftHierarchy[root][cat])) {
          if (leaf?.category && slugify(leaf.category) === topic) {
            return {
              topicName: leaf.subtopic || leaf.category,
              filterCategories: [leaf.category],
              subCategories: [],
            };
          }
        }
      }
    }
    for (const root of Object.keys(draftHierarchy)) {
      for (const cat of Object.keys(draftHierarchy[root])) {
        if (cat === 'featured' || slugify(cat) !== topic) continue;
        const leaves = leavesOf(draftHierarchy[root][cat]);
        return {
          topicName: (leaves[0] && leaves[0].subtopic) || cat,
          filterCategories: collectLeafCategories(draftHierarchy[root][cat]),
          subCategories: [],
        };
      }
    }
    return null;
  }

  // ── FOUR render states (banners render from the same values the
  //    content does — they can never contradict each other) ───
  const draftMatch = $derived(isPreviewing ? resolveDraft() : undefined);
  const draftState = $derived<'master' | 'draft' | 'removed' | 'section-missing'>(
    !isPreviewing
      ? 'master'
      : !draftHierarchy
        ? 'section-missing'
        : draftMatch
          ? 'draft'
          : 'removed'
  );

  const active = $derived(
    draftState === 'draft' ? draftMatch! : { topicName, filterCategories, subCategories }
  );

  // On a matched major route, the breadcrumb label follows the draft's
  // root name (a rename that keeps its slug is rare but honest to show).
  const activeMajorLabel = $derived(
    draftState === 'draft' && isMajor ? active.topicName : majorLabel
  );

  // Diagnostic mirror of the two failure banners.
  $effect(() => {
    if (draftState === 'section-missing') {
      console.warn(
        '[TopicView] Draft preview active but "Topics" subMenu not found in draft — rendering the deployed master. Check the subMenu key spelling in the editor.'
      );
    } else if (draftState === 'removed') {
      console.warn(
        `[TopicView] Draft preview active but route "${topic}" has no matching content in the draft (removed or renamed). Rendering the deployed master for reference.`
      );
    }
  });
</script>

{#if draftState === 'draft'}
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if draftState === 'removed'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ This topic's content was removed (or renamed) in your draft. Showing the deployed version for reference — this page may change or disappear after the next commit.
  </div>
{:else if draftState === 'section-missing'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "Topics" section was not found in it — this page is showing the deployed master. Check the subMenu key spelling in the editor.
  </div>
{/if}

<!-- Breadcrumb: Topics / [Major Theme /] Current -->
<nav class="text-sm text-gray-500 mb-4">
  <a href="/topics" class="hover:text-orange-600">Topics</a>
  {#if majorSlug}
    <span> / </span>
    <a href={`/topics/${majorSlug}`} class="hover:text-orange-600">{activeMajorLabel}</a>
  {/if}
  <span> / </span>
  <span class="text-gray-300">{active.topicName}</span>
</nav>

<h1 class="text-3xl font-bold mb-2">{active.topicName}</h1>

{#if isMajor && active.subCategories.length > 0}
  <div class="mb-8">
    <p class="text-gray-500 mb-4">Explore sub-categories below or browse all tracks.</p>
    <div class="flex flex-wrap gap-2">
      {#each active.subCategories as sub (sub.slug)}
        <a
          href={`/topics/${majorSlug}/${sub.slug}`}
          class="text-sm bg-gray-100 hover:bg-orange-100 text-gray-700 px-3 py-1 rounded-full border border-gray-200"
        >
          {sub.name}
        </a>
      {/each}
    </div>
  </div>
{/if}

<!-- TrackList filters the CATALOG by category at runtime — reads the
     ACTIVE (draft-aware) category list, so draft edits to leaf
     categories live-refilter the track list. -->
<TrackList categories={active.filterCategories} {apiBase} />
