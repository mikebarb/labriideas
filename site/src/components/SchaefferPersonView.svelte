<!-- src/components/SchaefferPersonView.svelte -->
<script lang="ts">
  // Hybrid "Preview Island" for the Schaeffer person route (Option 2,
  // same four-state contract as AlbumView.svelte).
  //
  // ROUTE EXISTENCE is a build-time decision (getStaticPaths in
  // [slug].astro): a person added in the draft has no page until the
  // next CI build; a person removed from the draft leaves a deployed
  // route that must be handled explicitly (state D), not silently.
  //
  // The preview has real teeth here: TrackList filters the CATALOG by
  // speakerName at runtime, so a draft edit to a speaker's name
  // live-refilters this page's track list, not just the heading.
  // (Corollary: renaming a speaker in the draft will empty the track
  // list until the catalog catches up — an honest preview of what the
  // deployed page would do, since the catalog keys on the real name.)

  import TrackList from './TrackList.svelte';
  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  interface Props {
    // Build-time person object (SSR seed / permanent fallback).
    person: any;
    // Route param — the slug the URL identifies.
    slug: string;
    apiBase?: string;
  }
  let { person, slug, apiBase = '' }: Props = $props();

  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's copy of the whole collection (undefined when the
  // top-level key itself is broken — distinct from a missing person).
  const draftCollection = $derived(
    isPreviewing ? ($menuData as any).schaefferCollection : undefined
  );

  // The draft's version of THIS person (undefined when not previewing,
  // when the key is broken, or when the person was removed/renamed).
  const draftPerson = $derived(
    draftCollection?.people?.[slug]
  );

  // FOUR render states — banners render from the same derived value
  // the content does, so they can never contradict each other.
  const draftState = $derived<'master' | 'draft' | 'person-removed' | 'section-missing'>(
    !isPreviewing
      ? 'master'
      : draftPerson
        ? 'draft'
        : draftCollection
          ? 'person-removed'
          : 'section-missing'
  );

  const activePerson = $derived(draftState === 'draft' ? draftPerson : person);

  // Diagnostic mirror of the two failure banners.
  $effect(() => {
    if (draftState === 'section-missing') {
      console.warn(
        '[SchaefferPersonView] Draft preview active but "schaefferCollection" key not found in draft — rendering the deployed master. Check the key spelling in the editor.'
      );
    } else if (draftState === 'person-removed') {
      console.warn(
        `[SchaefferPersonView] Draft preview active but person "${slug}" is not in the draft (removed or renamed). Rendering the deployed master for reference.`
      );
    }
  });
</script>

{#if draftState === 'draft'}
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if draftState === 'person-removed'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ This person was removed (or renamed) in your draft. Showing the deployed version for reference — this page will disappear after the next commit.
  </div>
{:else if draftState === 'section-missing'}
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "schaefferCollection" section was not found in it — this page is showing the deployed master. Check the key spelling in the editor.
  </div>
{/if}

<nav class="text-sm text-gray-500 mb-4">
  <a href="/schaeffer" class="hover:text-orange-600">Schaeffer Collection</a>
  <span> / </span>
  <span class="text-gray-300">{activePerson.speakerName}</span>
</nav>

<h1 class="text-3xl font-bold mb-6 text-gray-900">{activePerson.speakerName}</h1>
<p class="text-sm text-gray-500 mb-6">
  All lectures and sermons by {activePerson.speakerName}
</p>

<!-- TrackList filters the CATALOG by speaker at runtime — reads the
     ACTIVE (draft-aware) speakerName, so a draft rename live-refilters
     the list. Nested inside this island: no client:* directive needed. -->
<TrackList speaker={activePerson.speakerName} {apiBase} />
