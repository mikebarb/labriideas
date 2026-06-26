<!-- src/components/QueueDrawer.svelte -->
<script lang="ts">
  import { Play, Pause, X, Download, Music } from 'lucide-svelte';
  import { 
    isPlaylistOpen, trackList, currentTrackStore, statusStore, isAdminStore,
    currentTimeStore, durationStore
  } from '../lib/playerStore.js';
  import type { Track } from '../lib/types.js';

  function playTrack(track: Track) {
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }

  function removeFromQueue(filename: string) {
    window.dispatchEvent(new CustomEvent('remove-from-queue', { detail: { filename } }));
  }

  function downloadTrack(track: Track) {
    window.dispatchEvent(new CustomEvent('download-track', { detail: track }));
  }

  function closeDrawer() {
    isPlaylistOpen.set(false);
  }

  /**
   * Get the progress (0-100) for a given track.
   * - Currently playing track: live currentTime/duration
   * - Other tracks with saved position: track.position / track.duration
   * - Tracks never played or duration unknown: 0
   */
  function getProgress(track: Track): number {
    const isCurrent = $currentTrackStore?.filename === track.filename;
    
    if (isCurrent && $durationStore > 0) {
      return ($currentTimeStore / $durationStore) * 100;
    }
    
    if (!track.duration || track.duration <= 0) return 0;
    
    return Math.min(100, ((track.position ?? 0) / (track.duration ?? 1)) * 100);  
  }
</script>

<aside class="w-80 flex flex-col h-full bg-[#0e0e0e] text-white border-l border-neutral-800">
  <div class="flex items-center justify-between p-4 border-b border-neutral-800">
    <h2 class="text-base font-semibold flex items-center gap-2">
      <Music size={18} /> Up Next
    </h2>
    <button 
      onclick={closeDrawer}
      class="text-neutral-400 hover:text-white p-1 rounded-full hover:bg-white/10"
      aria-label="Close queue"
    >
      <X size={20} />
    </button>
  </div>

  <div class="flex-1 overflow-y-auto p-2">
    {#if $trackList.length === 0}
      <div class="text-center text-neutral-500 text-sm py-10 px-4">
        Your queue is empty. Add tracks to start listening.
      </div>
    {:else}
      {#each $trackList as track, i (track.filename)}
        {@const isCurrent = $currentTrackStore?.filename === track.filename}
        {@const progress = getProgress(track)}
        <div 
          class="group flex items-center gap-3 p-2 rounded-md cursor-pointer transition hover:bg-white/5
                 {isCurrent ? 'bg-white/10' : ''}"
          role="button"
          tabindex="0"
          onclick={() => playTrack(track)}
          onkeydown={(e) => { if (e.key === 'Enter') playTrack(track); }}
        >
          <span class="text-xs text-neutral-500 w-5 text-right">
            {#if isCurrent && $statusStore === 'playing'}
              <Pause size={12} class="text-[#1db954]" />
            {:else}
              {i + 1}
            {/if}
          </span>
          
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium wrap-break-words">
              {track.title ?? track.filename}
            </div>
            <div class="text-xs text-neutral-400 wrap-break-words">
              {track.speaker ?? 'Unknown Speaker'}
            </div>
            
            <!-- Progress bar: shown if track has progress -->
            {#if progress > 0}
              <div class="mt-1.5 h-0.5 bg-neutral-800 rounded-full overflow-hidden">
                <div 
                  class="h-full {isCurrent ? 'bg-[#1db954]' : 'bg-neutral-500'} transition-all duration-300"
                  style="width: {progress}%"
                ></div>
              </div>
            {/if}
          </div>

          <button 
            class="text-neutral-500 hover:text-red-400 p-1 opacity-0 group-hover:opacity-100"
            onclick={(e) => { e.stopPropagation(); removeFromQueue(track.filename); }}
            aria-label="Remove from queue"
          >
            <X size={14} />
          </button>

          {#if $isAdminStore}
            <button 
              class="text-neutral-500 hover:text-white p-1 opacity-0 group-hover:opacity-100"
              onclick={(e) => { e.stopPropagation(); downloadTrack(track); }}
              aria-label="Download track"
            >
              <Download size={14} />
            </button>
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</aside>
