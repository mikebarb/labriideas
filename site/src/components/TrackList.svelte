<!-- src/components/TrackList.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { onMount } from 'svelte';
  import TrackCardGroup from './TrackCardGroup.svelte';

  interface Props {
    tracks?: Array<{ displayTitle: string; filename: string }>;
    topic?: string;
    speaker?: string;
  }

  //let { tracks: trackRefs = [], topic = '' }: Props = $props();
  let { tracks: trackRefs = [], topic = '', speaker = '' }: Props = $props();
  let playableTracks: any[] = $state([]);
  let isLoading: boolean = $state(true);

  onMount(async () => {
    try {
      const catalog = await getCatalog();

      if (trackRefs && trackRefs.length > 0) {
        // PLAYLIST MODE
        playableTracks = trackRefs
          .map(ref => {
            const full = catalog.find((t: any) => t.filename === ref.filename);
            if (!full) return null;
            return { ...full, displayTitle: ref.displayTitle };
          })
          .filter(Boolean);
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
  <TrackCardGroup items={playableTracks} />
{/if}
