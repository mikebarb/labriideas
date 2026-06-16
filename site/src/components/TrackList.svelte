<script>
  import { getCatalog } from '../lib/catalogStore.js';
  import { onMount } from 'svelte';

  // 'tracks' comes from menu.json via the [album].astro page
  let { tracks: trackRefs = [] } = $props();

  let playableTracks = $state([]);

  onMount(async () => {
    const catalog = await getCatalog();
    
    // For each track reference in menu.json, find the full object in the catalog
    playableTracks = trackRefs
      .map(ref => catalog.find(t => t.filename === ref.filename))
      .filter(Boolean); // Remove any that weren't found
    });

  function playThisTrack(track) {
    // Dispatch the event your global Player is already listening for
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }
</script>

{#if playableTracks.length === 0}
  <p class="text-gray-400 italic">Loading tracks...</p>
{:else}
  <ul class="space-y-2">
    {#each playableTracks as track}
      <li class="flex items-center justify-between p-3 bg-white/5 rounded hover:bg-white/10">
        <span class="truncate">{track.title || track.filename}</span>
        <button 
          onclick={() => playThisTrack(track)}
          class="bg-green-600 hover:bg-green-500 text-black px-3 py-1 rounded text-sm font-semibold"
        >
          Play
        </button>
      </li>
    {/each}
  </ul>
{/if}
