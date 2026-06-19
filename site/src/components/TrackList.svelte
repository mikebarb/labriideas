<!-- src/components/TrackList.svelte -->
<script lang="ts">
  import { getCatalog } from '../lib/catalogStore.js';
  import { onMount } from 'svelte';

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

  function playThisTrack(track: any) {
    const cleanTrack = $state.snapshot(track);
    window.dispatchEvent(new CustomEvent('play-track', { detail: cleanTrack }));
  }
</script>

{#if isLoading}
  <p class="text-gray-400 italic py-4">Loading tracks…</p>
{:else if playableTracks.length === 0}
  <p class="text-gray-400 italic py-4">No tracks found.</p>
{:else}
  <ul class="space-y-2">
    {#each playableTracks as track (track.filename)}
      <li class="flex items-center justify-between p-3 bg-white/5 rounded hover:bg-white/10 transition">
        <span class="truncate text-sm text-gray-200">
          {track.displayTitle ?? track.title ?? track.filename}
          {#if track.speaker}
            <span class="text-gray-500 ml-2">— {track.speaker}</span>
          {/if}
        </span>
        <button
          type="button"
          onclick={() => playThisTrack(track)}
          class="bg-green-600 hover:bg-green-500 text-black px-3 py-1 rounded text-sm font-semibold transition flex-shrink-0 ml-3"
        >
          Play
        </button>
      </li>
    {/each}
  </ul>
{/if}
