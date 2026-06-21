<!-- src/components/SearchResultsDisplay.svelte -->
<script lang="ts">
  import type { Track } from '../lib/types';
  import { sanitizeKeywords } from '../lib/dataUtils.js';

  interface Props {
    tracks: Track[];
  }

  let { tracks }: Props = $props();
  let expandedFilename: string | null = $state(null);

  function toggle(filename: string): void {
    expandedFilename = expandedFilename === filename ? null : filename;
  }

  function play(track: Track): void {
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }

  function formatCategory(category: string | string[] | undefined): string {
    if (!category) return '';
    if (Array.isArray(category)) return category.join(', ');
    return category;
  }

  // Re-introduced: The Translation Layer
  function getKeywords(track: Track): string[] {
    return sanitizeKeywords(track.keywords);
  }

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
    
    {#each tracks as track (track.filename)}
      <div class="border rounded transition-all {expandedFilename === track.filename ? 'bg-gray-50' : 'bg-white'}">
        <button 
          class="w-full text-left p-4 flex justify-between items-center min-h-11"
          onclick={() => toggle(track.filename)}
        >
          <div class="truncate flex-1">
            <p class="font-medium text-gray-900">{track.title || track.filename}</p>
            <p class="text-sm text-gray-500">{track.speaker}</p>
          </div>
          <span class="text-gray-400 ml-2">{expandedFilename === track.filename ? '▼' : '▶'}</span>
        </button>

        {#if expandedFilename === track.filename}
          <div class="px-4 pb-4 border-t border-gray-200 pt-4">
            <div class="flex items-center justify-between">
              <div class="flex-1">
                {#if track.category}
                  <span class="inline-block text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded mr-2">
                    {formatCategory(track.category)}
                  </span>
                {/if}
                {#if getKeywords(track).length > 0}
                  <div class="mt-2 flex flex-wrap gap-1">
                    {#each getKeywords(track) as kw (kw)}
                      <span class="text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded border border-gray-200">
                        #{kw}
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
              <button 
                onclick={() => play(track)}
                class="bg-green-600 hover:bg-green-700 text-white px-6 py-2 rounded-lg font-bold transition"
              >
                ▶ Play Track
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
