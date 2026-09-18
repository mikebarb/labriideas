<!-- src/components/AlbumView.svelte -->
<script lang="ts">
  // Hybrid "Preview Island" for the album detail route (Option 2 pattern).
  //
  // UNIQUE AMONG PAGES: this route's EXISTENCE is decided at build time by
  // getStaticPaths() in [album].astro. Two boundaries follow, neither
  // fixable in the browser:
  //   1. Adding an album in the draft → no /playlists/<new-id>/ route
  //      exists in the build output; the index grid previews the new card,
  //      but its detail page 404s until the next CI build.
  //   2. Removing/renaming an album in the draft → the deployed route
  //      still exists. Handled explicitly below (state D) rather than
  //      silently showing the master as if nothing changed.
  //
  // Within an EXISTING route (same id), content previews fully:
  //   1. STATIC fallback: album prop, extracted at build time from
  //      menu.json — what public visitors and search engines receive.
  //   2. LIVE DRAFT: menuDataStore, only when admin + draft is active.

  import TrackList from './TrackList.svelte';
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  interface Props {
    // Build-time album object (SSR seed / permanent fallback).
    album: any;
    // Route param — the id the URL identifies. (Astro.params.album; equals
    // album.id for the deployed build. A draft that renames the id
    // intentionally misses here — its page would live at a different URL
    // that doesn't exist until commit.)
    albumId: string;
    apiBase?: string;
  }
  let { album, albumId, apiBase = '' }: Props = $props();

  // Draft preview is live ONLY for a logged-in admin with a draft applied.
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's Playlists section (undefined if the subMenu key itself is
  // missing/broken — a different failure mode from a missing album).
  const draftPlaylists = $derived(
    isPreviewing
      ? (($menuData as any).subMenus.find((s: any) => s.subMenu === 'Playlists') as any)
      : undefined
  );

  // The draft's version of THIS album (undefined when not previewing,
  // when the section key is broken, or when the album was removed/renamed).
  const draftAlbum = $derived(
    draftPlaylists?.albums?.find((a: any) => a.id === albumId)
  );

  // ─── FOUR render states ───
  //   A. not previewing                    → master, no banner
  //   B. previewing + draft album found    → DRAFT content, amber banner
  //   C. previewing + Playlists key broken → master, red "section missing"
  //   D. previewing + key OK, album gone   → master, red "removed in draft"
  //
  // Banners render from the SAME derived value the content does, so
  // banner and screen can never contradict each other.
  const draftState = $derived<'master' | 'draft' | 'album-removed' | 'section-missing'>(
    !isPreviewing
      ? 'master'
      : draftAlbum
        ? 'draft'
        : draftPlaylists
          ? 'album-removed'
          : 'section-missing'
  );

  const activeAlbum = $derived(draftState === 'draft' ? draftAlbum : album);

  // Diagnostic mirror of the two failure banners.
  $effect(() => {
    if (draftState === 'section-missing') {
      console.warn(
        '[AlbumView] Draft preview active but "Playlists" subMenu not found in draft — rendering the deployed master. Check the subMenu key spelling in the editor.'
      );
    } else if (draftState === 'album-removed') {
      console.warn(
        `[AlbumView] Draft preview active but album "${albumId}" is not in the draft (removed or renamed). Rendering the deployed master for reference.`
      );
    }
  });
</script>

{#if draftState === 'draft'}
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if draftState === 'album-removed'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ This album was removed (or renamed) in your draft. Showing the deployed version for reference — this page will disappear after the next commit.
  </div>
{:else if draftState === 'section-missing'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "Playlists" section was not found in it — this page is showing the deployed master. Check the subMenu key spelling in the editor.
  </div>
{/if}

<h1 class="text-3xl font-bold mb-4 text-gray-900">{activeAlbum.title}</h1>
<p class="text-gray-400 mb-8">{activeAlbum.description}</p>

<!-- tracks now flow from the ACTIVE album (draft when previewing, else
     master). TrackList renders as a nested component inside this island
     — no client:* directive (that's Astro-only, and the parent island is
     already hydrated). -->
<TrackList tracks={activeAlbum.tracks} {apiBase} />
