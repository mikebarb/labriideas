<!-- src/components/QueueDrawer.svelte -->
<script lang="ts">
  import { Play, Pause, X, Download, Music, GripVertical } from 'lucide-svelte';
  import { 
    isPlaylistOpen, trackList, currentTrackStore, statusStore, isAdminStore,
    currentTimeStore, durationStore
  } from '../lib/playerStore.js';
  import type { Track } from '../lib/types.js';

  // ─── Track focus after reorder ───
  // When a reorder is dispatched, remember which track should retain
  // focus. After Svelte re-renders the list (triggered by $trackList
  // changing), if focus ended up on <body>, restore it to the row.
  let focusedFilename: string | null = $state(null);

  // ─── Event dispatch (queue mutations go through Player) ───
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

  // ─── Focus recovery effect ───
  // This runs AFTER Svelte updates the DOM in response to $trackList.
  // If the active element is <body> (focus was lost), restore it.
  $effect(() => {
    const _ = $trackList;

    if (!focusedFilename) return;

    requestAnimationFrame(() => {
      const activeEl = document.activeElement;
      
      // If focus was lost, restore it
      if (activeEl === document.body || activeEl === null) {
        // Narrow focusedFilename here — TypeScript needs to see this check
        // inside the same scope where it's used
        if (focusedFilename !== null) {
          const row = document.querySelector<HTMLElement>(
            `[data-row-filename="${CSS.escape(focusedFilename)}"]`
          );
          if (row) {
            row.focus();
          }
        }
      }
      // Reset after the attempt
      focusedFilename = null;
    });
  });

  // ─── Drag and Drop Handlers ───
  function startDrag(e: PointerEvent, track: Track, index: number) {
    // Prevent native text/image dragging glitches
    e.preventDefault();
    
    draggedFilename = track.filename;
    draggedIndex = index;
    dragOverIndex = index; // Initialize indicator at current position
    isDragging = true;
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging) return;

    // Find the row under the pointer
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const row = el?.closest('[data-row-index]') as HTMLElement | null;

    if (row) {
      const idx = parseInt(row.getAttribute('data-row-index')!, 10);
      const rect = row.getBoundingClientRect();
      const isAboveHalf = e.clientY < rect.top + rect.height / 2;

      // If hovering over the dragged item itself, keep indicator at its start
      if (idx === draggedIndex) {
        dragOverIndex = idx;
        return;
      }

      // Top half of row n = drop *before* row n (index n)
      // Bottom half of row n = drop *after* row n (index n+1)
      dragOverIndex = isAboveHalf ? idx : idx + 1;
    }
  }

  function handlePointerUp() {
    if (!isDragging || !draggedFilename || dragOverIndex === null) {
      resetDragState();
      return;
    }

    // Dispatch the reorder event. 
    // The raw visual index works perfectly with Array.splice 
    // because removing the item shifts indices automatically.
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

  // ─── Keyboard Reorder Handlers ───
  function handleKeyboardReorder(e: KeyboardEvent, track: Track, index: number) {
    const moveUp = e.altKey && e.key === 'ArrowUp';
    const moveDown = e.altKey && e.key === 'ArrowDown';

    if (!moveUp && !moveDown) return;

    e.preventDefault(); // Prevent page scroll

    const targetIndex = moveUp ? index - 1 : index + 1;
    
    // Bounds check
    if (targetIndex < 0 || targetIndex >= $trackList.length) return;

    window.dispatchEvent(new CustomEvent('reorder-queue', {
      detail: {
        filename: track.filename,
        newIndex: targetIndex
      }
    }));

    // Optional: refocus the handle of the moved item 
    // (Svelte 5 keyed each should keep the DOM node intact, 
    // so focus naturally stays on the moved handle)
  }

 // ─── Row-level keyboard reorder ───
  // The row itself is the focus target. Alt+ArrowUp/Down reorders.
function handleRowKeydown(e: KeyboardEvent, track: Track, index: number) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      playTrack(track);
      return;
    }

    const moveUp = e.altKey && e.key === 'ArrowUp';
    const moveDown = e.altKey && e.key === 'ArrowDown';
    if (!moveUp && !moveDown) return;

    e.preventDefault();
    e.stopPropagation();

    // Read current index from DOM attribute (fresh each time)
    const target = e.currentTarget as HTMLElement;
    const currentIndex = parseInt(target.getAttribute('data-current-index') ?? '0', 10);
    
    // Match the drag-and-drop semantics: `newIndex` is the position
    // in the array *before* the dragged item is spliced out. Moving
    // up = swap with previous (targetIndex = index - 1, no adjustment
    // applied because idx is never < newIndex). Moving down = swap with
    // next (targetIndex = index + 2, because splice will remove us
    // first, shifting indices down by 1; the adjustment then puts us
    // at index + 1 in the final array).
    const targetIndex = moveUp ? index - 1 : index + 2;
    if (targetIndex < 0 || targetIndex > $trackList.length) return;

    // Remember which track should stay focused after the reorder
    focusedFilename = track.filename;

    window.dispatchEvent(new CustomEvent('reorder-queue', {
      detail: {
        filename: track.filename,
        newIndex: targetIndex
      }
    }));

  }
</script>

<!-- Global listeners for dragging. Only active when isDragging is true -->
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

        <!-- Drop indicator (Above current row) -->
        {#if isDragging && dragOverIndex === i}
          <div class="h-1 bg-[#1db954] rounded-full mx-2 mb-1 transition-all"></div>
        {/if}

        <!-- Row wrapper for hit-testing -->
        <div data-row-index={i}>
          <div 
            class="group flex items-center gap-3 p-2 rounded-md cursor-pointer transition hover:bg-white/5
                   {isCurrent ? 'bg-white/10' : ''} 
                   {isBeingDragged ? 'opacity-40 scale-[0.98]' : ''}"
            role="button"
            tabindex="0"
            data-row-filename={track.filename}
            onclick={() => playTrack(track)}
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

            <!-- Drag Handle -->
            <button
              class="text-neutral-600 hover:text-neutral-300 p-1 opacity-0 group-hover:opacity-100 focus:opacity-100 cursor-grab {isDragging ? 'cursor-grabbing' : ''}"
              style="touch-action: none;"
              aria-label="Drag to reorder or use Alt+Arrow keys"
              aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
              data-reorder-handle="true"
              data-filename={track.filename}
              onpointerdown={(e) => startDrag(e, track, i)}
              tabindex="0"
            >
              <GripVertical size={14} />
            </button>

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
        </div>
      {/each}

      <!-- Drop indicator (At the very end of the list) -->
      {#if isDragging && dragOverIndex === $trackList.length}
        <div class="h-1 bg-[#1db954] rounded-full mx-2 mt-1 transition-all"></div>
      {/if}
    {/if}
  </div>
</aside>
