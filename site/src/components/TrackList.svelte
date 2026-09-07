<!-- src/components/TrackList.svelte -->
<script lang="ts">
  import { getCachedCatalog } from '../lib/catalogStore.js'; 
  import { onMount } from 'svelte';
  import TrackCardGroup from './TrackCardGroup.svelte';

  interface Props {
    apiBase: string;
    tracks?: Array<{ displayTitle: string; filename: string }>;
    topic?: string;
    speaker?: string;
    categories?: string[];
  }

  let { apiBase, tracks: trackRefs = [], topic = '', speaker = '', categories = [] }: Props = $props();
  let playableTracks: any[] = $state([]);
  // NEW: playlist entries whose filename missed the catalog. We keep the
  // menu.json displayTitle so the page can show a disabled "Lecture not
  // available." row — visible feedback for admins curating menu.json,
  // who won't have the console open.
  let missingTracks: Array<{ displayTitle: string; filename: string }> = $state([]);
  let isLoading: boolean = $state(true);

  onMount(async () => {
    try {
      const { tracks: catalog } = await getCachedCatalog();

      if (trackRefs && trackRefs.length > 0) {
        // PLAYLIST MODE
        // CHANGED: catalog misses are now RETAINED and rendered as disabled
        // "Lecture not available." rows below the playable list (see
        // template) instead of being silently dropped. The page itself is
        // the admin's feedback channel — they see the missing lecture while
        // editing, not just the console. Still warns with the exact filename.
        playableTracks = [];
        missingTracks = [];
        for (const ref of trackRefs) {
            const full = catalog.find((t: any) => t.filename === ref.filename);
          if (full) {
            playableTracks.push({ ...full, displayTitle: ref.displayTitle });
          } else {
            console.warn(`[Playlist] Track filename not found in catalog: "${ref.filename}"`);
            missingTracks.push({ displayTitle: ref.displayTitle, filename: ref.filename });
          }
        }
      } else if (categories.length > 0) {
        // NEW: UNION-FILTER MODE (Major/Nested topic pages)
        // [...topic].astro aggregates every leaf category under a major
        // theme or sub-category and passes the raw strings here. A track
        // matches if ANY of its categories equals ANY entry in the list
        // (case-insensitive) — handles the catalog's category being a
        // plain string OR an array, same robustness as TOPICS MODE.
        const searchTags = categories.map(c => c.toLowerCase());
        playableTracks = catalog.filter((t: any) => {
          if (!t.category) return false;
          if (Array.isArray(t.category)) {
            return t.category.some((c: string) => searchTags.includes(c.toLowerCase()));
          }
          return searchTags.includes(t.category.toLowerCase());
        });
      } else if (topic) {
        // TOPICS MODE
        const searchTag = topic.toLowerCase();
        playableTracks = catalog.filter((t: any) => {
          if (!t.category) return false;
          if (Array.isArray(t.category)) {
            return t.category.map((c: string) => c.toLowerCase()).includes(searchTag);
          }
          return t.category.toLowerCase() === searchTag;
        });
      } else if (speaker) {
        // SPEAKER MODE
        const searchSpeaker = speaker.toLowerCase();
        playableTracks = catalog.filter((t: any) => {
          if (!t.speaker) return false;
          return t.speaker.toLowerCase() === searchSpeaker;
        });
      }
    } catch (err) {
      console.error("TrackList failed to load catalog:", err);
      playableTracks = [];
    } finally {
      isLoading = false;
    }
  });
</script>

{#if isLoading}
  <p class="text-gray-400 italic py-4">Loading tracks…</p>
{:else if playableTracks.length === 0 && missingTracks.length === 0}
  <p class="text-gray-400 italic py-4">No tracks found.</p>
{:else}
  {#if playableTracks.length > 0}
    <TrackCardGroup items={playableTracks } {apiBase}/>
  {/if}

  <!-- NEW: disabled rows for catalog misses (playlist mode). Styled to sit
       visually alongside TrackCards. Compound key guards against duplicate
       placeholder filenames, consistent with MegaMenu/FeaturedGrid. -->
  {#if missingTracks.length > 0}
    <div class="mt-2 space-y-2">
      {#each missingTracks as miss, i (`${miss.filename}-${i}`)}
        <div class="flex items-center gap-3 p-3 bg-gray-50 border border-gray-200 rounded-md opacity-70">
          <span class="text-xs font-bold text-red-500 uppercase tracking-wide shrink-0">Unavailable</span>
          <div class="min-w-0">
            <p class="text-sm font-medium text-gray-900 pb-0 wrap-break-word">{miss.displayTitle}</p>
            <p class="text-xs text-gray-400 italic pb-0">Lecture not available.</p>
          </div>
        </div>
      {/each}
    </div>
  {/if}
{/if}
