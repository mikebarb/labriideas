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
  let isLoading: boolean = $state(true);

  onMount(async () => {
    try {
      const { tracks: catalog } = await getCachedCatalog();

      if (trackRefs && trackRefs.length > 0) {
        // PLAYLIST MODE
        playableTracks = trackRefs
          .map(ref => {
            const full = catalog.find((t: any) => t.filename === ref.filename);
            if (!full) return null;
            return { ...full, displayTitle: ref.displayTitle };
          })
          .filter(Boolean);
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
{:else if playableTracks.length === 0}
  <p class="text-gray-400 italic py-4">No tracks found.</p>
{:else}
  <TrackCardGroup items={playableTracks } {apiBase}/>
{/if}
