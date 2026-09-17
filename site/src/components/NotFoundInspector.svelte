<!-- src/components/NotFoundInspector.svelte -->
<script lang="ts">
  // Smart 404: explains WHY the page is missing, in draft-aware terms.
  //
  // A static build serves the same 404.html for every unmatched path, so
  // the failed URL is read at runtime (window.location) and matched
  // against BOTH the live draft and the deployed master:
  //   1. Draft hit  → the route exists only in the admin's uncommitted
  //      draft (a new/renamed category, album, or person). Its page
  //      appears after commit + CI build — the documented build-time
  //      route limitation, stated plainly instead of as a dead error.
  //   2. Miss everywhere → possible genuine category mismatch in
  //      menu.json (a deployed link pointing at a slug nothing owns).
  //   3. Not previewing → generic not-found with navigation.

  import { onMount } from 'svelte';
  import { menuData, menuPreviewSource } from '../lib/menuDataStore.ts';
  import { isAdmin } from '../lib/appStatusStore.ts';
  import { slugify } from '../lib/slugify.ts';
  import masterMenu from '../data/menu.json';

  // The failed path, normalized: no leading/trailing slashes, segments
  // as an array (["topics", "xintroduction-to-l-abri"] etc.).
  let segments = $state<string[]>([]);
  onMount(() => {
    segments = window.location.pathname
      .split('/')
      .filter(s => s !== '');
  });

  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // Node-shape helpers (mirror the build's acceptance of both shapes).
  const leavesOf = (node: any): any[] =>
    Array.isArray(node) ? node : (node?.lectures ?? []);

  // Walks a menu object's hierarchy/sections looking for anything that
  // owns the requested slug. Returns a human-readable hit or null.
  function findHit(menu: any): { kind: string; name: string } | null {
    if (!menu) return null;
    const topicSlug = segments.slice(1).join('/');

    // ── /topics/... ──
    if (segments[0] === 'topics' && segments.length >= 2) {
      const hierarchy = menu.subMenus?.find((s: any) => s.subMenu === 'Topics')?.hierarchy;
      if (hierarchy) {
        for (const root of Object.keys(hierarchy)) {
          if (slugify(root) === segments[1]) {
            return { kind: 'topic (major theme)', name: root };
          }
          for (const cat of Object.keys(hierarchy[root])) {
            if (cat === 'featured') continue;
            if (slugify(cat) === segments[1]) {
              return { kind: 'topic (category)', name: cat };
            }
            if (segments.length === 3 && slugify(cat) === segments[2]) {
              return { kind: 'topic (sub-category)', name: cat };
            }
            for (const leaf of leavesOf(hierarchy[root][cat])) {
              if (leaf?.category && slugify(leaf.category) === segments[1]) {
                return { kind: 'topic (lecture category)', name: leaf.category };
              }
            }
          }
        }
      }
    }

    // ── /playlists/<id>/ ──
    if (segments[0] === 'playlists' && segments.length >= 2) {
      const albums = menu.subMenus?.find((s: any) => s.subMenu === 'Playlists')?.albums ?? [];
      const album = albums.find((a: any) => a.id === segments[1]);
      if (album) return { kind: 'playlist album', name: album.title ?? album.id };
    }

    // ── /schaeffer/<slug>/ ──
    if (segments[0] === 'schaeffer' && segments.length >= 2) {
      const people = menu.schaefferCollection?.people ?? {};
      const person = people[segments[1]];
      if (person) return { kind: 'Schaeffer collection person', name: person.speakerName ?? segments[1] };
    }

    return null;
  }

  // Resolution against draft and master. Only meaningful for the menu's
  // route namespaces; anything else stays a plain 404.
  const draftHit = $derived(
    isPreviewing && ['topics', 'playlists', 'schaeffer'].includes(segments[0])
      ? findHit($menuData)
      : null
  );
  const masterHit = $derived(
    ['topics', 'playlists', 'schaeffer'].includes(segments[0])
      ? findHit(masterMenu)
      : null
  );

  // verdict drives the template — banner and text can't disagree.
  const verdict = $derived<'draft-only' | 'mismatch' | 'generic'>(
    draftHit ? 'draft-only' : masterHit ? 'generic' : (segments.length ? 'mismatch' : 'generic')
  );
</script>

{#if verdict === 'draft-only' && draftHit}
  <!-- The failed route exists in the admin's uncommitted draft. Cyan =
       draft item with no route yet (same signal language as the
       "New — page appears after deploy" album badge). -->
  <div class="mb-6 px-3 py-3 bg-cyan-100 border border-cyan-400 text-cyan-900 text-sm rounded">
    <p class="font-bold mb-1">This page exists in your uncommitted draft.</p>
    <p>
      "{draftHit.name}" ({draftHit.kind}) was added or renamed in your draft, but its route is
      created at build time. <strong>Commit the draft and let the CI build finish</strong> —
      this URL will work afterwards.
    </p>
  </div>
{:else if verdict === 'mismatch'}
  <!-- Matches nothing, anywhere — a genuinely broken link. -->
  <div class="mb-6 px-3 py-3 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    <p class="font-bold mb-1">No matching category was found.</p>
    <p>
      The URL <code class="bg-red-50 px-1 rounded">/{segments.join('/')}</code> doesn't match any
      topic, playlist, or Schaeffer entry — <strong>possible category mismatch in menu.json</strong>
      (a link whose slug no longer corresponds to any category name).
    </p>
  </div>
{:else}
  <h1 class="text-3xl font-bold text-gray-900 mb-3">Page not found</h1>
  <p class="text-gray-500 mb-6">
    The page you're looking for doesn't exist or may have moved.
  </p>
{/if}

<nav class="flex flex-wrap gap-3 text-sm">
  <a href="/" class="text-orange-600 hover:text-orange-700 underline">Home</a>
  <a href="/topics" class="text-orange-600 hover:text-orange-700 underline">Topics</a>
  <a href="/playlists" class="text-orange-600 hover:text-orange-700 underline">Playlists</a>
</nav>
