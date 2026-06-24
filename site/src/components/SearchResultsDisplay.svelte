<!-- src/components/SearchResultsDisplay.svelte -->
<script lang="ts">
  import type { Track } from '../lib/types.js';
  //import { sanitizeKeywords } from '../lib/dataUtils.js';
  import TrackCardGroup from './TrackCardGroup.svelte';

  interface Props {
    //tracks: Track[];
    tracks: any[];   // still accepts the same prop name and type as before
  }

  let { tracks }: Props = $props();
  //let expandedFilename: string | null = $state(null);

  //function toggle(filename: string): void {
  //  expandedFilename = expandedFilename === filename ? null : filename;
  //}

</script>

<div class="space-y-2">
  {#if tracks.length === 0}
    <div class="text-center py-8">
      <p class="text-gray-500 italic">No tracks match your filters.</p>
    </div>
  {:else}
    <p class="text-sm text-gray-500 mb-4">
      {tracks.length} {tracks.length === 1 ? 'track' : 'tracks'} foundxxx
    </p>
    <TrackCardGroup items={tracks} />
  {/if}
</div>
