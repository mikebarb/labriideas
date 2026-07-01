<!-- src/components/QueueDrawer.svelte -->
<script lang="ts">
  import { Play, Pause, X, Download, Music, GripVertical } from 'lucide-svelte';
  import { 
    isPlaylistOpen, trackList, currentTrackStore, statusStore, isAdminStore,
    currentTimeStore, durationStore
  } from '../lib/playerStore.js';
  import type { Track } from '../lib/types.js';

  // CHANGED: Import the download function from the new library location.
  // Previously, downloads were triggered via a custom DOM event to Player.
  // Now, we call the library function directly. This is cleaner because:
  //   - QueueDrawer no longer needs to know about Player's internal API.
  //   - The same download logic can be used in the search results.
  import { downloadTrack } from '../lib/downloader';

  interface Props {
    apiBase: string;  // CHANGED: Added to enable downloads
  }
  let { apiBase }: Props = $props();

  // ─── Event dispatch (queue mutations go through Player) ───
  function playTrack(track: Track) {
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }

  function removeFromQueue(filename: string) {
    window.dispatchEvent(new CustomEvent('remove-from-queue', { detail: { filename } }));
  }

  // CHANGED: Replaced event-dispatch pattern with direct library call.
  // Previously, this dispatched a 'download-track' event to Player, which
  // would handle the actual download. Now we call the library function 
  // directly with a spinner state for user feedback.
  let downloadingFilename: string | null = $state(null);

  function handleDownload(track: Track) {
    // Prevent multiple simultaneous downloads of the same file
    if (downloadingFilename === track.filename) return;

    downloadingFilename = track.filename;

    downloadTrack(track, apiBase, {
      onComplete: () => { 
        downloadingFilename = null; 
      },
      onError: (err) => {
        console.error('Download failed:', err);
        downloadingFilename = null;
      },
    });
  }

  function closeDrawer() {
    isPlaylistOpen.set(false);
  }

  // ─── Progress visualization ───
  type ProgressKind = 'placeholder' | 'filled' | 'none';

  function getProgress(track: Track): { kind: ProgressKind; pct: number } {
    const isCurrent = $currentTrackStore?.filename === track.filename;
    if (isCurrent) return { kind: 'placeholder', pct: 0 };
    if (!track.duration || track.duration <= 0) return { kind: 'none', pct: 0 };
    return {
      kind: 'filled',
      pct: Math.min(100, ((track.position ?? 0) / track.duration) * 100),
    };
  }

  // ─── Drag and Drop State ───
  let draggedFilename: string | null = $state(null);
  let draggedIndex: number | null = $state(null);
  let dragOverIndex: number | null = $state(null);
  let isDragging = $state(false);

  // ─── Drag and Drop Handlers ───
  function startDrag(e: PointerEvent, track: Track, index: number) {
    e.preventDefault();
    draggedFilename = track.filename;
    draggedIndex = index;
    dragOverIndex = index;
    isDragging = true;
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging) return;

    const el = document.elementFromPoint(e.clientX, e.clientY);
    const row = el?.closest('[data-row-index]') as HTMLElement | null;

    if (row) {
      const idx = parseInt(row.getAttribute('data-row-index')!, 10);
      const rect = row.getBoundingClientRect();
      const isAboveHalf = e.clientY < rect.top + rect.height / 2;

      if (idx === draggedIndex) {
        dragOverIndex = idx;
        return;
      }

      dragOverIndex = isAboveHalf ? idx : idx + 1;
    }
  }

  function handlePointerUp() {
    if (!isDragging || !draggedFilename || dragOverIndex === null) {
      resetDragState();
      return;
    }

    window.dispatchEvent(new CustomEvent('reorder-queue', {
      detail: {
        filename: draggedFilename,
        newIndex: dragOverIndex
      }
    }));

    resetDragState();
  }

  function resetDragState() {
    isDragging = false;
    draggedFilename = null;
    draggedIndex = null;
    dragOverIndex = null;
  }

  // ─── Keyboard Reorder Handler ───
  function handleRowKeydown(e: KeyboardEvent, track: Track, index: number) {
    // Enter/Space plays the track (works on all devices)
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      playTrack(track);
      return;
    }

    // Alt+ArrowUp/Down = reorder (desktop only)
    const moveUp = e.altKey && e.key === 'ArrowUp';
    const moveDown = e.altKey && e.key === 'ArrowDown';
    if (!moveUp && !moveDown) return;

    // Skip on mobile — use drag-and-drop
    if (window.innerWidth < 768) return;

    e.preventDefault();
    e.stopPropagation();

    const target = e.currentTarget as HTMLElement;
    const currentIndex = parseInt(target.getAttribute('data-row-index') ?? '0', 10);

    const targetIndex = moveUp ? currentIndex - 1 : currentIndex + 2;
    if (targetIndex < 0 || targetIndex > $trackList.length) return;

    window.dispatchEvent(new CustomEvent('reorder-queue', {
      detail: {
        filename: track.filename,
        newIndex: targetIndex
      }
    }));

    // Refocus after reorder (works on desktop)
    setTimeout(() => {
      const row = document.querySelector<HTMLElement>(
        `[data-row-filename="${CSS.escape(track.filename)}"]`
      );
      if (row) {
        row.focus();
      }
    }, 0);
  }
</script>

<svelte:window 
  onpointermove={isDragging ? handlePointerMove : null} 
  onpointerup={isDragging ? handlePointerUp : null}
  onpointercancel={isDragging ? resetDragState : null}
/>

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
        {@const isBeingDragged = draggedFilename === track.filename}

        {#if isDragging && dragOverIndex === i}
          <div class="h-1 bg-[#1db954] rounded-full mx-2 mb-1 transition-all"></div>
        {/if}

        <div 
          data-row-index={i}
          data-row-filename={track.filename}
          role="button"
          tabindex="0"
          class="group flex items-center gap-3 p-2 rounded-md cursor-pointer transition hover:bg-white/5
                 focus:bg-white/10 focus:outline-none focus:ring-2 focus:ring-[#1db954]/30
                 {isCurrent ? 'bg-white/10' : ''} 
                 {isBeingDragged ? 'opacity-40 scale-[0.98]' : ''}"
          onclick={(e) => {
            (e.currentTarget as HTMLElement).focus();
            playTrack(track);
          }}
          onkeydown={(e) => handleRowKeydown(e, track, i)}
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
            
            {#if progress.kind === 'placeholder'}
              <div class="mt-1.5 flex items-center gap-0.75 h-2" aria-hidden="true">
                {#each Array(8) as _, barIdx}
                  <div
                    class="w-0.5 h-full rounded-sm bg-neutral-500"
                    style="opacity: {0.4 + (barIdx % 3) * 0.2}"
                  ></div>
                {/each}
              </div>
            {:else if progress.kind === 'filled'}
              <div class="mt-1.5 h-0.5 bg-neutral-800 rounded-full overflow-hidden">
                <div 
                  class="h-full bg-neutral-500 transition-all duration-300"
                  style="width: {progress.pct}%"
                ></div>
              </div>
            {/if}
          </div>

          <button
            class="text-neutral-400 hover:text-white p-1.5 rounded hover:bg-white/10 
                   opacity-100 md:opacity-0 md:group-hover:opacity-100 focus:opacity-100 
                   cursor-grab {isDragging ? 'cursor-grabbing' : ''}"
            style="touch-action: none;"
            aria-label="Drag to reorder or use Alt+Arrow keys"
            aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
            data-reorder-handle="true"
            data-filename={track.filename}
            onpointerdown={(e) => startDrag(e, track, i)}
            tabindex="0"
          >
            <GripVertical size={16} />
          </button>

          <button 
            class="text-neutral-400 hover:text-red-400 p-1.5 rounded hover:bg-white/10 
                   opacity-100 md:opacity-0 md:group-hover:opacity-100"
            onclick={(e) => { e.stopPropagation(); removeFromQueue(track.filename); }}
            aria-label="Remove from queue"
          >
            <X size={16} />
          </button>

            {#if $isAdminStore}
              <!--
                - onclick calls handleDownload(track)
                - button is disabled while downloading (prevents double-clicks).
                - icon is swapped for a spinner during the download.
                - cursor changes to 'wait' to indicate the button is busy.
              -->
              <button 
                class="text-neutral-400 hover:text-white p-1.5 rounded hover:bg-white/10 
                       opacity-100 md:opacity-0 md:group-hover:opacity-100
                       disabled:opacity-100 disabled:cursor-wait"
                onclick={(e) => { e.stopPropagation(); handleDownload(track); }}
                disabled={downloadingFilename === track.filename}
                aria-label="Download track"
              >
                {#if downloadingFilename === track.filename}
                  <div class="w-4 h-4 border-2 border-neutral-400 border-t-transparent rounded-full animate-spin"></div>
                {:else}
                  <Download size={16} />
                {/if}
              </button>
            {/if}
        </div>
      {/each}

      {#if isDragging && dragOverIndex === $trackList.length}
        <div class="h-1 bg-[#1db954] rounded-full mx-2 mt-1 transition-all"></div>
      {/if}
    {/if}
  </div>
</aside>
