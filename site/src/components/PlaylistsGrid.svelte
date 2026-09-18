<!--   src/components/PlaylistsGrid.svelte   -->
<script lang="ts">
  // Hybrid "Preview Island" (Option 2 pattern — same contract as
  // ContactSection.svelte / FeaturedGrid.svelte).
  //
  // Renders the playlist album grid from two sources:
  //   1. The STATIC fallback — albums prop, extracted from the
  //      build-time menu.json by the page's frontmatter and baked into
  //      the SSR HTML. What public visitors and search engines receive.
  //   2. The LIVE DRAFT — menuDataStore, ONLY when the visitor is an
  //      admin AND a draft preview is active. Covers album adds/renames,
  //      reordering, description edits, background images, and the
  //      suggest-a-playlist external link.
  //
  // Non-admin behavior: identical to the previous fully-static render.
  // After a Step C deploy, the committed draft becomes the new build
  // master and the preview condition stops firing for everyone.

  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  interface Album {
    id: string;
    title: string;
    titleBgColor?: string;
    description?: string;
    backgroundImage?: string;
    tracks?: unknown[];
    linkToForm?: string;
  }

  interface Props {
    // Build-time extraction from the page's frontmatter — the SSR seed
    // and the permanent fallback for non-admin visitors.
    albums: Album[];
  }
  let { albums }: Props = $props();

  // Draft preview is live ONLY for a logged-in admin with a draft applied.
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's copy of the Playlists section (subMenu key is
  // load-bearing — an exact-match locator in every consumer, including
  // this page's frontmatter and the [album] route). Resolved once,
  // shared by banner / diagnostic / selection.
  const draftSection = $derived(
    isPreviewing
      ? (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Playlists')) ?? undefined
      : undefined
  );

  // Diagnostic twin of the banner state — the two can never disagree.
  $effect(() => {
    if (isPreviewing && !draftSection) {
      console.warn(
        '[PlaylistsGrid] Draft preview active but "Playlists" subMenu not found in draft — rendering the deployed master. Check the subMenu key spelling in the editor.'
      );
    }
  });

  // Single source-selection point: draft albums when the draft section
  // resolved, else the static prop.
  const activeAlbums = $derived(
    isPreviewing && draftSection ? (draftSection.albums ?? []) : albums
  );

  // Build-time album ids, computed ONCE from the static prop. These are
  // the ids for which getStaticPaths() has generated real routes in the
  // current build. A draft album whose id is NOT in this set has no
  // page yet — clicking through would 404 (route existence is decided
  // at build time; see AlbumView.svelte's boundary notes).
  const buildTimeIds = $derived(new Set(albums.map(a => a.id)));

  // True when this album exists only in the draft — its grid card is
  // rendered as a NON-LINK (badge instead of navigation) until the next
  // commit + CI build creates its route.
  const isDraftOnly = (album: Album) =>
    isPreviewing && !buildTimeIds.has(album.id);

  // External-album classification (e.g. suggest-a-playlist → Typeform):
  // a linkToForm with no tracks renders as an external link card.
  const isExternal = (album: Album) =>
    !!(album.linkToForm && !album.tracks);
</script>

{#if isPreviewing && draftSection}
  <!-- Draft banner: the grid on screen IS the admin's uncommitted draft. -->
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if isPreviewing}
  <!-- Broken-preview banner: a draft IS active, but this section could
       not be found in it — the page is showing the deployed MASTER.
       Matches the console warn exactly. -->
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "Playlists" section was not found in it — this page is showing the deployed master. Check the subMenu key spelling in the editor.
  </div>
{/if}

<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
  <!-- Keyed by album.id — the route param, so keys stay unique across
       draft edits. (If a draft ever contains duplicate ids, Svelte will
       complain loudly rather than silently mis-render — acceptable for
       an admin preview surface.) -->
  {#each activeAlbums as album (album.id)}
    <div class="rounded-lg overflow-hidden bg-slate-800 shadow-lg">
      {#if isDraftOnly(album)}
        <!-- DRAFT-ONLY ALBUM: no route exists yet (build-time decision),
             so the card is rendered NON-CLICKABLE — the badge converts a
             confusing 404 into an informative state. After commit + CI
             build, the route exists and the card becomes a normal link. -->
        <div
          class="block relative h-48 bg-cover bg-center group cursor-not-allowed opacity-80"
          style={`background-image: url(${album.backgroundImage})`}
        >
          <div class="absolute inset-x-0 bottom-0 bg-linear-to-t from-black/80 to-transparent p-4">
            <h2 class:list={["text-white text-xl font-bold", album.titleBgColor]}>
              {album.title}
            </h2>
          </div>
          <!-- Subtle badge, top-right: readable against any album art. -->
          <span
            class="absolute top-2 right-2 bg-cyan-500/90 text-slate-900 text-xs font-bold px-2 py-1 rounded"
          >
            New — page appears after deploy
          </span>
        </div>
      {:else}
        <a
          href={isExternal(album) ? album.linkToForm : `/playlists/${album.id}/`}
          target={isExternal(album) ? '_blank' : undefined}
          rel={isExternal(album) ? 'noopener noreferrer' : undefined}
          class="block relative h-48 bg-cover bg-center group"
          style={`background-image: url(${album.backgroundImage})`}
        >
          <div class="absolute inset-x-0 bottom-0 bg-linear-to-t from-black/80 to-transparent p-4">
            <h2 class:list={["text-white text-xl font-bold", album.titleBgColor]}>
              {album.title}
            </h2>
          </div>
        </a>
      {/if}
      <div class="p-4">
        <p class="text-gray-300 text-sm">{album.description}</p>
      </div>
    </div>
  {/each}
</div>
