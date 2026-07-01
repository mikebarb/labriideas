<!-- src/components/SearchResultsDisplay.svelte -->
<script lang="ts">
  import type { Track } from '../lib/types.ts';
  //import { sanitizeKeywords } from '../lib/dataUtils.ts';
  import TrackCardGroup from './TrackCardGroup.svelte';

  interface Props {
    //tracks: Track[];
    tracks: any[];   // still accepts the same prop name and type as before
    apiBase: string;
    isAdmin?: boolean;
  }

   let { tracks, apiBase, isAdmin = false }: Props = $props();

</script>

<div class="space-y-2">
  {#if tracks.length === 0}
    <div class="text-center py-8">
      <p class="text-gray-500 italic">No tracks match your filters.</p>
    </div>
  {:else}
    <p class="text-sm text-gray-500 mb-4">
      {tracks.length} {tracks.length === 1 ? 'track' : 'tracks'} found
    </p>
   <TrackCardGroup items={tracks} {apiBase} {isAdmin} />
  {/if}
</div>
