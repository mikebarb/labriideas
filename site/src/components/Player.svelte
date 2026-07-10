<!-- src/components/Player.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { 
    Play, Pause, SkipBack, SkipForward, RotateCcw, RotateCw, 
    Volume2, VolumeX, ListMusic, Minimize2, Maximize2
  } from 'lucide-svelte';
  import { 
    mobileView, desktopQueueOpen,
    trackList, currentTrackStore, statusStore, isAdminStore,
    currentTimeStore, durationStore
  } from '../lib/playerStore.js';
  import type { Track } from '../lib/types.ts';
  import { fetchPresignedUrl } from '../lib/downloader_OLD.ts';
  import { removeTrackFromOpfs, getTrackBlob } from '../lib/opfsStore';
    import type { unknown } from 'astro:schema';

  // ─── Props ───
  interface Props {
    apiBase?: string;
    showTracklist?: boolean;
    isAdmin?: boolean;
  }
  let { apiBase = '', showTracklist = true, isAdmin = false }: Props = $props();

  // ─── State ───
  // The tracks array is the source of truth. Each track holds its own
  // runtime state (position, duration, url, urlExpiresAt).
  let tracks: Track[] = $state([]);
  let isQueueLoaded = $state(false); // Guards against clearing localStorage on the first mount
  let currentTrack: Track | null = $state(null);
  let status = $state<'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'error'>('idle');
  let currentTime = $state(0);
  let duration = $state(0);
  let volume = $state(0.8);
  let isMuted = $state(false);
  let errorMessage = $state('');

  let audioElement: HTMLAudioElement | null = $state(null);
  let seekBarElement: HTMLElement | null = $state(null);
  let isDragging = $state(false);
  let dragProgress = $state(0);

   // ─── Speed Control ───
  let speedLongPressTimer: ReturnType<typeof setTimeout> | null = $state(null);
  let isSpeedLongPressing = $state(false);

  // ─── Store mirroring ───
  // IMPORTANT: We DO NOT use $effect to mirror tracks → trackList. That
  // would fire on every nested mutation (e.g. position updates during
  // playback, ~5x/sec) and force the QueueDrawer to re-render constantly.
  //
  // Instead, the `commitQueue()` helper is called EXPLICITLY at the
  // moments when the queue's *structure* or *frozen per-track progress*
  // changes:
  //   - Adding a track (playTrack on a new track)
  //   - Removing a track
  //   - Reordering (drag-and-drop, future)
  //   - Switching tracks (the outgoing track's position becomes frozen)
  //   - Pausing (current track's position becomes frozen)
  //   - Track ending (its position is finalized)
  //   - Playback rate change (label display)
  //   - pagehide / beforeunload (crash recovery — flush current position)
  //
  // Live playback state (currentTime) is mirrored to currentTimeStore /
  // statusStore / currentTrackStore via $effect — those are read by the
  // player's own seek bar and by QueueDrawer for the *current-track
  // indicator* (which row is active), not for any per-tick progress.
  $effect(() => { currentTrackStore.set(currentTrack); });
  $effect(() => { statusStore.set(status); });
  $effect(() => { isAdminStore.set(isAdmin); });
  $effect(() => { currentTimeStore.set(currentTime); });
  $effect(() => { durationStore.set(duration); });

  /**
   * Publish the current `tracks` snapshot to the shared store.
   * Call this only at structural/frozen events (see comment block above).
   */
  function commitQueue(): void {
    trackList.set(tracks);
    if (isQueueLoaded) {
      localStorage.setItem('labri_queue', JSON.stringify(tracks));
    }
  }

  // ─── Reactive Active Track Recovery ───
  // This runs whenever `tracks` changes. It handles BOTH cases:
  //   1. Fresh page load (onMount populates tracks from localStorage)
  //   2. Persisted page navigation (transition:persist keeps component alive,
  //      so onMount doesn't re-run, but the store still updates tracks)
  // If `currentTrack` is null but the queue has an `isActive` track,
  // we restore it. This makes the player "self-healing" across all 
  // navigation patterns.
  $effect(() => {
    if (!currentTrack && tracks.length > 0) {
      const activeTrack = $state.snapshot(tracks).find(t => t.isActive);
      if (activeTrack) {
        currentTrack = activeTrack;
        duration = activeTrack.duration ?? 0;
        currentTime = activeTrack.position ?? 0;
        console.log(`[Player] Effect restored active track: ${activeTrack.title ?? activeTrack.filename}`);
        
        if (audioElement) {
          loadTrack(activeTrack).catch(err => {
            console.warn(`[Player] Could not preload ${activeTrack.filename}:`, err);
          });
        }
      }
    }
  });


  // ─── Derived ───
  let progress = $derived(
    isDragging ? dragProgress : (duration > 0 ? (currentTime / duration) * 100 : 0)
  );
  let displayTime = $derived(
    isDragging ? (dragProgress / 100) * duration : currentTime
  );

   // ─── Mobile view toggles (NEW) ───
  // State machine: 'min' | 'max' | 'list'
  // Both buttons act as toggles, with the priority/selection logic
  // we agreed on. Each tap flips the relevant view on/off; if neither
  // is selected, the user returns to 'min'.
  function toggleMaxPlayer(): void {
    if ($mobileView === 'max') {
      mobileView.set('min');
    } else {
      mobileView.set('max');
    }
  }

  function togglePlaylist(): void {
    if ($mobileView === 'list') {
      mobileView.set('min');
    } else {
      mobileView.set('list');
    }
  }

  // ─── Helpers ───
  function formatTime(s: number): string {
    if (!isFinite(s) || s < 0) return '0:00';
    const m = Math.floor(s / 60);
    const sec = Math.floor(s % 60);
    return `${m}:${sec.toString().padStart(2, '0')}`;
  }

  function bindSeekBar(node: HTMLElement) {
    seekBarElement = node;
    return {
      destroy() {
        if (seekBarElement === node) seekBarElement = null;
      }
    };
  }

  /**
   * Update the position slot of the currently playing track in `tracks`,
   * without firing the store. The position is kept fresh in memory so
   * `playTrack` can resume correctly on switch. The store is only
   * notified when the position becomes "frozen" (pause / switch / end /
   * crash recovery) via commitQueue().
   */
  function updateCurrentTrackPosition(pos: number): void {
    if (!currentTrack) return;
    currentTrack.position = pos;
    // Mirror the new position onto the slot in `tracks` so it stays
    // consistent with currentTrack's reference. We use a spread to
    // create a new object — this keeps immutability explicit and avoids
    // any stale-reference surprises if something reads `tracks[idx]`
    // between this mutation and a subsequent commitQueue().
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    if (idx !== -1) {
      tracks[idx] = { ...tracks[idx], position: pos };
    }
  }

  // ─── Playback Functions ───
  
  /**
   * Toggle play/pause on the current track.
   */
  async function togglePlayPause(): Promise<void> {
    if (!audioElement) return;
    
    // If no current track, try to play the first one in the queue
    if (!currentTrack) {
      if (tracks.length > 0) {
        await playTrack(tracks[0]);
      }
      return;
    }
    
    // If there's an error, retry by reloading
    if (status === 'error') {
      await loadTrack(currentTrack);
      await audioElement.play();
      status = 'playing';
      return;
    }
    
    // Normal play/pause toggle
    if (audioElement.paused) {
      try {
        await audioElement.play();
      } catch (err) {
        console.error('Toggle play failed:', err);
      }
    } else {
      audioElement.pause();
    }
  }

  /**
   * Load a track's metadata (URL, duration) into its object.
   * Uses cached URL if still valid.
   */
  async function loadTrack(track: Track): Promise<void> {
    if (!audioElement) return;

    status = 'loading';
    errorMessage = '';

    // Use cached URL if valid, otherwise fetch new one
    if (!track.url || !track.urlExpiresAt || track.urlExpiresAt < Date.now()) {
      try {
        const ticket = await fetchPresignedUrl(track.filename, apiBase);
        track.url = ticket.url;
        track.urlExpiresAt = ticket.expiresAt;
      } catch (err: any) {
        console.error('Failed to fetch URL:', err);
        status = 'error';
        errorMessage = err.message;
        throw err;
      }
    }

    audioElement.src = track.url;
    audioElement.load();
    audioElement.playbackRate = track.playbackRate ?? 1.0;

    // Capture duration on metadata load
    await new Promise<void>((resolve) => {
      const onMeta = () => {
        if (audioElement && isFinite(audioElement.duration) && audioElement.duration > 0) {
          track.duration = audioElement.duration;
          duration = audioElement.duration;
        }
        audioElement?.removeEventListener('loadedmetadata', onMeta);
        resolve();
      };
      audioElement?.addEventListener('loadedmetadata', onMeta);
    });
  }

  /**
   * Main entry point. Handles:
   *   - Same track → toggle play/pause
   *   - New track → load + play
   *   - Switching tracks → save old position (frozen), load new, restore position
   */
  async function playTrack(track: Track): Promise<void> {
    if (!audioElement) return;

    // CASE 1: Same track → toggle
    if (currentTrack?.filename === track.filename) {
      await togglePlayPause();
      return;
    }

    // CASE 2: Different track
    // Finalize the outgoing track's progress — its position is now
    // "frozen" as progress made so far, so the drawer can show it.
    if (currentTrack && audioElement.src) {
      const finalPos = audioElement.currentTime;
      updateCurrentTrackPosition(finalPos);
      commitQueue(); // ← structural: current-track changed
    }

    // Add to queue if not already there
    if (!tracks.find(t => t.filename === track.filename)) {
      tracks = [...tracks, track];
      commitQueue(); // ← structural: queue grew
    }

    currentTime = 0;
    duration = 0;
    errorMessage = '';

    try {
      await loadTrack(track);
      
      // Restore position if any
      const savedPos = track.position ?? 0;
      if (savedPos > 0) {
        audioElement.currentTime = savedPos;
        currentTime = savedPos;
      }
      
         // Clear the flag on any previously active track
      if (currentTrack && currentTrack.filename !== track.filename) {
        const oldIdx = tracks.findIndex(t => t.filename === currentTrack!.filename);
        if (oldIdx !== -1) {
          tracks[oldIdx] = { ...tracks[oldIdx], isActive: false };
        }
      }

      // Set the flag on the newly active track
      const activeIdx = tracks.findIndex(t => t.filename === track.filename);
      if (activeIdx !== -1) {
        tracks[activeIdx] = { ...tracks[activeIdx], isActive: true };
      }

      currentTrack = track;

      await audioElement.play();
      status = 'playing';
    } catch (err: any) {
      console.error('Play error:', err);
      if (err.name === 'NotAllowedError') {
        status = 'paused';
      } else {
        status = 'error';
        errorMessage = err.message;
      }
    }
  }

  function playNext() {
    if (!currentTrack || tracks.length === 0) return;
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    if (idx < tracks.length - 1) playTrack(tracks[idx + 1]);
  }

  function playPrev() {
    if (!currentTrack || tracks.length === 0) return;
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    if (idx > 0) playTrack(tracks[idx - 1]);
    else if (audioElement) audioElement.currentTime = 0;
  }

  function jump(seconds: number) {
    if (audioElement) audioElement.currentTime = Math.max(0, audioElement.currentTime + seconds);
  }

  function removeFromQueue(filename: string) {
    // Capture the track before it's removed, so we can wipe OPFS ───
    const trackToRemove = tracks.find(t => t.filename === filename);
    
    // If it was the currently playing track and loaded from OPFS, free the memory ───
    if (currentTrack?.filename === filename && audioElement && audioElement.src.startsWith('blob:')) {
      URL.revokeObjectURL(audioElement.src);
    }
    
    // Purge the audio file from the OPFS hard drive ───
    if (trackToRemove?.hash) {
      removeTrackFromOpfs(trackToRemove.hash).catch((err: unknown) => {
        console.warn(`[OPFS] Failed to remove ${trackToRemove.hash}:`, err);
      });
    }

    const wasCurrent = currentTrack?.filename === filename;
    // Filter out the track — its position, duration, url all go with it
    tracks = tracks.filter(t => t.filename !== filename);
    commitQueue(); // ← structural: queue shrunk
    
    if (wasCurrent) {
      if (tracks.length === 0) {
        if (audioElement) {
          audioElement.pause();
          audioElement.currentTime = 0;
          audioElement.removeAttribute('src');
          audioElement.load();
        }
        currentTrack = null;
        status = 'idle';
        currentTime = 0;
        duration = 0;
        // Queue is now empty, so close both the mobile queue overlay
        // and the desktop sidebar.
        mobileView.set('min');
        desktopQueueOpen.set(false);
      } else {
        // Play next without restoring position (fresh start)
        const nextTrack = tracks[0];
        playTrack(nextTrack);
      }
    }
  }

  /**
   * Reorder the queue. Triggered by QueueDrawer's drag-and-drop.
   * No-op until DnD is wired up in a follow-up commit.
   */
  function reorderQueue(filename: string, newIndex: number): void {
    const idx = tracks.findIndex(t => t.filename === filename);
    if (idx === -1 || idx === newIndex) return;

    // Splice is in-place; reassign `tracks` so Svelte's local $state
    // reactivity fires for any Player-internal UI bound to it.
    const [moved] = tracks.splice(idx, 1);
    
    // BUGFIX: If we removed an item BEFORE the insertion index, 
    // the insertion index needs to shift down by 1 to account 
    // for the array shrinking.
    let adjustedNewIndex = newIndex;
    if (idx < newIndex) {
      adjustedNewIndex = newIndex - 1;
    }
    
    tracks.splice(adjustedNewIndex, 0, moved);
    tracks = [...tracks];
    commitQueue(); // ← structural: order changed
  }

  // ─── Speed Control ───
  // Changed from a toggle to +/- buttons with 0.1 increments.
  // Middle button shows current speed; long-press to reset to 1.0x.
  function setPlaybackRate(rate: number) {
    if (!currentTrack) return;
    // Clamp to reasonable bounds (0.25–3.0)
    rate = Math.max(0.25, Math.min(3.0, rate));
    // Round to nearest 0.1 to avoid floating point issues
    rate = Math.round(rate * 10) / 10;
    
    currentTrack.playbackRate = rate;
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    if (idx !== -1) {
      tracks[idx] = { ...tracks[idx], playbackRate: rate };
      commitQueue();
    }
    if (audioElement) audioElement.playbackRate = rate;
  }

  function decreaseSpeed() {
    if (!currentTrack) return;
    const current = currentTrack.playbackRate ?? 1.0;
    setPlaybackRate(current - 0.1);
  }

  function increaseSpeed() {
    if (!currentTrack) return;
    const current = currentTrack.playbackRate ?? 1.0;
    setPlaybackRate(current + 0.1);
  }

  function resetSpeed() {
    setPlaybackRate(1.0);
  }

  // ─── Long Press for Speed Reset ───
  // Prevents accidental resets — user must hold ~500ms to reset.
  function onSpeedPointerDown() {
    if (!currentTrack) return;
    isSpeedLongPressing = false;
    speedLongPressTimer = setTimeout(() => {
      isSpeedLongPressing = true;
      resetSpeed();
    }, 500);
  }

  function onSpeedPointerUp() {
    if (speedLongPressTimer) {
      clearTimeout(speedLongPressTimer);
      speedLongPressTimer = null;
    }
    // No action on short click — only long press triggers reset
  }

  function onSpeedPointerCancel() {
    if (speedLongPressTimer) {
      clearTimeout(speedLongPressTimer);
      speedLongPressTimer = null;
    }
  }

  function toggleMute() {
    isMuted = !isMuted;
    if (audioElement) audioElement.muted = isMuted;
  }

  // ─── Seek Bar ───
  function calcProgress(e: PointerEvent): number {
    if (!seekBarElement) return 0;
    const rect = seekBarElement.getBoundingClientRect();
    return Math.max(0, Math.min(100, ((e.clientX - rect.left) / rect.width) * 100));
  }

  function onSeekDown(e: PointerEvent) {
    if (!seekBarElement) return;
    isDragging = true;
    seekBarElement.setPointerCapture(e.pointerId);
    dragProgress = calcProgress(e);
  }

  function onSeekMove(e: PointerEvent) {
    if (isDragging) dragProgress = calcProgress(e);
  }

  function onSeekUp() {
    if (!isDragging || !audioElement || !duration) return;
    isDragging = false;
    audioElement.currentTime = (dragProgress / 100) * duration;
  }

  // ─── Global Keyboard Reorder Listener ───
  function handleGlobalReorder(e: KeyboardEvent) {
    // We only care about Alt + Arrow keys
    const moveUp = e.altKey && e.key === 'ArrowUp';
    const moveDown = e.altKey && e.key === 'ArrowDown';
    if (!moveUp && !moveDown) return;

    // Check if the currently focused element is a drag handle belonging to a track
    const target = e.target as HTMLElement;
    const handle = target.closest('[data-reorder-handle]') as HTMLButtonElement | null;

    // If the user isn't focused on a drag handle, do nothing
    if (!handle) return;

    e.preventDefault();

    const filename = handle.getAttribute('data-filename');
    const currentIndex = tracks.findIndex(t => t.filename === filename);

    // Safety check
    if (currentIndex === -1) return;

    const targetIndex = moveUp ? currentIndex - 1 : currentIndex + 1;
    
    // Bounds check
    if (targetIndex < 0 || targetIndex >= tracks.length) return;

    // Prevent moving a track to its own index
    if (targetIndex === currentIndex) return;

    // Dispatch the same reorder event
    window.dispatchEvent(new CustomEvent('reorder-queue', {
      detail: {
        filename: filename,
        newIndex: targetIndex
      }
    }));
  }

  // ─── Lifecycle ───
  onMount(() => {
    // The "Self-Healing" OnMount:
    // 1. Restore Queue from localStorage
    // 2. Restore the Active Track (NEW)
    // 3. Bind Audio Element Listeners
    // 4. Bind External Event Listeners
    const savedQueue = localStorage.getItem('labri_queue');
    if (savedQueue) {
      try {
        const parsed = JSON.parse(savedQueue);
        if (Array.isArray(parsed)) {
          tracks = parsed;
          const unwrappedForLog = $state.snapshot(tracks);
          console.log('[Player] Recovered from localStorage:', unwrappedForLog);
        }
      } catch (e) {
        console.error("[Player] Failed to restore queue from localStorage", e);
      }
    }
    isQueueLoaded = true;
    commitQueue(); // Sync the store so QueueDrawer sees the restored list immediately
    
    // ─── Restore the Active Track ───
    // We use $state.snapshot() to unwrap the reactive Proxy. 
    // The native HTML <audio> element and the OPFS API require 
    // plain, unwrapped JavaScript objects to work reliably.
    //const unwrappedTracks = $state.snapshot(tracks);
    //const activeTrack = unwrappedTracks.find(t => t.isActive);

    //console.log('[Player] Recovery process started. Active track found:', activeTrack);

    //if (activeTrack) {
      // Guard against undefined duration/position to prevent audio errors
    //  currentTrack = activeTrack;
    //  duration = activeTrack.duration ?? 0;
    // currentTime = activeTrack.position ?? 0;
      
    //  console.log(`[Player] Restored active track: ${activeTrack.title ?? activeTrack.filename}`);
      
      // Preload the audio (silently fail if offline/no cache)
    //  if (audioElement) {
    //    loadTrack(activeTrack).catch(err => {
    //      console.warn(`[Player] Could not preload ${activeTrack.filename}:`, err);
    //    });
    //  }
    //}

   
    if (audioElement) {
      audioElement.volume = volume;

      // Live playback tick — drives the player's own seek bar via the
      // currentTimeStore mirror. We also keep the in-memory resume
      // position fresh on the current track, but we deliberately do NOT
      // call commitQueue() here. The drawer renders frozen per-track
      // progress and a static placeholder for the current track; a
      // position update mid-playback is not a structural event.
      audioElement.addEventListener('timeupdate', () => {
        if (audioElement && currentTrack) {
          if (status === 'playing' || status === 'paused') {
            const t = audioElement.currentTime;
            currentTime = t;
            updateCurrentTrackPosition(t);
          }
        }
      });

      audioElement.addEventListener('durationchange', () => {
        if (audioElement && currentTrack && isFinite(audioElement.duration)) {
          duration = audioElement.duration;
          currentTrack.duration = audioElement.duration;
          // No commitQueue — duration isn't surfaced in the drawer.
        }
      });

      audioElement.addEventListener('play', () => status = 'playing');

      // On pause, the current track's progress becomes "frozen" — this
      // IS a structural event for the drawer's purposes, so commit.
      audioElement.addEventListener('pause', () => {
        if (status !== 'loading') status = 'paused';
        if (currentTrack && audioElement) {
          updateCurrentTrackPosition(audioElement.currentTime);
          commitQueue(); // ← structural: position frozen at pause
        }
      });

      audioElement.addEventListener('ended', () => {
        // Track ended — finalize position to 0 (Spotify-style: ended
        // tracks are "ready to replay from start", not 100% complete).
        if (currentTrack) {
          updateCurrentTrackPosition(0);
          commitQueue(); // ← structural: position finalized at end
        }
        playNext();
      });

      audioElement.addEventListener('error', () => { status = 'error'; errorMessage = 'Playback error'; });
    }

    // External event listeners for QueueDrawer and TrackList
    const handlePlay = (e: Event) => playTrack((e as CustomEvent).detail);
    const handleRemove = (e: Event) => removeFromQueue((e as CustomEvent).detail.filename);
    const handleReorder = (e: Event) => {
      const { filename, newIndex } = (e as CustomEvent).detail;
      reorderQueue(filename, newIndex);
    };

    window.addEventListener('play-track', handlePlay);
    window.addEventListener('remove-from-queue', handleRemove);
    window.addEventListener('reorder-queue', handleReorder);

     // Catch Alt + Arrow keys globally when a drag handle is focused
    window.addEventListener('keydown', handleGlobalReorder);

    // Crash recovery — flush current position into the store on tab
    // close / navigation so a reload can resume correctly. pagehide is
    // more reliable than beforeunload on mobile.
    const flushPosition = () => {
      if (!currentTrack || !audioElement) return;
      updateCurrentTrackPosition(audioElement.currentTime);
      commitQueue(); // ← crash-recovery flush
    };
    window.addEventListener('pagehide', flushPosition);
    window.addEventListener('beforeunload', flushPosition);

    return () => {
      window.removeEventListener('play-track', handlePlay);
      window.removeEventListener('remove-from-queue', handleRemove);
      window.removeEventListener('reorder-queue', handleReorder);
      window.removeEventListener('keydown', handleGlobalReorder);
      window.removeEventListener('pagehide', flushPosition);
      window.removeEventListener('beforeunload', flushPosition);
    };
  });
</script>

<audio bind:this={audioElement} preload="auto"></audio>

<!-- DESKTOP LAYOUT -->
{#if tracks.length > 0}
  <div class="hidden md:flex fixed bottom-0 left-0 right-0 h-24 bg-[#0e0e0e] border-t border-neutral-800 items-center px-6 z-40">
    
    <div class="flex-1 min-w-0 flex flex-col justify-center">
      {#if currentTrack}
        <div class="text-sm font-semibold truncate text-white">{currentTrack.title ?? currentTrack.filename}</div>
        <div class="text-xs text-neutral-400 truncate">{currentTrack.speaker ?? 'Unknown Speaker'}</div>
      {:else}
        <div class="text-sm text-neutral-500">No track selected</div>
      {/if}
    </div>

    <div class="flex-1 max-w-2xl flex flex-col items-center gap-2">
      <div class="flex items-center gap-4">
        <button onclick={playPrev} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Previous" disabled={!currentTrack}>
          <SkipBack size={20} />
        </button>
        <button onclick={() => jump(-15)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Back 15s" disabled={!currentTrack}>
          <RotateCcw size={20} />
        </button>
        <button onclick={togglePlayPause} class="bg-white text-black rounded-full w-11 h-11 flex items-center justify-center hover:scale-105 transition disabled:opacity-30 shadow-lg" aria-label="Play/Pause" disabled={!currentTrack}>
          {#if status === 'playing'}<Pause size={22} fill="currentColor" />{:else}<Play size={22} fill="currentColor" class="ml-0.5" />{/if}
        </button>
        <button onclick={() => jump(30)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Forward 30s" disabled={!currentTrack}>
          <RotateCw size={20} />
        </button>
        <button onclick={playNext} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Next" disabled={!currentTrack}>
          <SkipForward size={20} />
        </button>
      </div>
      <div class="w-full flex items-center gap-3">
        <span class="text-[11px] text-neutral-400 tabular-nums w-10 text-right">{formatTime(displayTime)}</span>
        <div 
          use:bindSeekBar
          class="flex-1 h-1 bg-neutral-700 rounded-full relative cursor-pointer group"
          role="slider" 
          tabindex="0"
          aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress)}
          onpointerdown={onSeekDown} onpointermove={onSeekMove} onpointerup={onSeekUp} onpointercancel={onSeekUp}
        >
          <div class="absolute top-0 left-0 h-full bg-white rounded-full" style="width: {progress}%"></div>
          <div class="absolute top-1/2 w-3 h-3 bg-white rounded-full -translate-y-1/2 -translate-x-1/2 opacity-0 group-hover:opacity-100" style="left: {progress}%"></div>
        </div>
        <span class="text-[11px] text-neutral-400 tabular-nums w-10">{formatTime(duration)}</span>
      </div>
    </div>

    <div class="flex-1 flex items-center justify-end gap-3">
      <!--
        CHANGED: Speed control redesigned from a single toggle button
        to a three-button cluster:
          - = decreases speed by 0.1
          [1.0x] = shows current speed; LONG-PRESS to reset to 1.0x
          + = increases speed by 0.1
        The long-press prevents accidental resets.
      -->
      <div class="flex items-center gap-1">
        <button 
          onclick={decreaseSpeed}
          class="text-neutral-300 hover:text-white w-6 h-6 rounded hover:bg-white/10 disabled:opacity-30 flex items-center justify-center text-base leading-none"
          aria-label="Decrease speed"
          disabled={!currentTrack}
        >
          −
        </button>
        <button
          onpointerdown={onSpeedPointerDown}
          onpointerup={onSpeedPointerUp}
          onpointercancel={onSpeedPointerCancel}
          ondragstart={(e) => e.preventDefault()}
          class="text-xs text-neutral-300 hover:text-white min-w-12 px-2 py-1 rounded hover:bg-white/10 disabled:opacity-30 select-none"
          aria-label="Current speed. Long press to reset to 1.0x"
          disabled={!currentTrack}
        >
          {(currentTrack?.playbackRate ?? 1.0).toFixed(2)}x
        </button>
        <button
          onclick={increaseSpeed}
          class="text-neutral-300 hover:text-white w-6 h-6 rounded hover:bg-white/10 disabled:opacity-30 flex items-center justify-center text-base leading-none"
          aria-label="Increase speed"
          disabled={!currentTrack}
        >
          +
        </button>
      </div>

      <button onclick={() => desktopQueueOpen.update(v => !v)}  class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10" aria-label="Toggle queue">
        <ListMusic size={20} />
      </button>

      <!-- {#if isAdmin && currentTrack}
        <button onclick={() => downloadTrack(currentTrack!)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10" aria-label="Download">
          <Download size={18} />
        </button>
      {/if} -->
    </div>
  </div>

  <!-- MOBILE LAYOUT
    Three views now share a single root with two pieces:
      1. A persistent bottom bar (always rendered when tracks exist)
      2. The view-specific content above the bar (only when maxPlayer is shown)

    The QueueDrawer is rendered separately by AppShell.svelte when
    $mobileView === 'list', so it sits above the bar and is dismissable
    via the bar's Playlist toggle or its own X button.
  -->
  <div class="md:hidden">
    <!-- 1. Persistent bottom bar -->
    <div class="fixed bottom-4 left-1/2 -translate-x-1/2 w-[calc(100%-2rem)] max-w-md h-14 bg-[#181818] border border-neutral-800 rounded-full px-4 flex items-center gap-3 z-60 shadow-2xl">
      <button onclick={togglePlayPause} class="text-white p-1" aria-label="Play/Pause" disabled={!currentTrack}>
        {#if status === 'playing'}<Pause size={20} fill="currentColor" />{:else}<Play size={20} fill="currentColor" class="ml-0.5" />{/if}
      </button>
      <div class="flex-1 min-w-0">
        <div class="text-sm font-medium truncate text-white">{currentTrack?.title ?? 'No track'}</div>
        <div class="text-xs text-neutral-400 truncate">{currentTrack?.speaker ?? ''}</div>
      </div>

      <!--
        Playlist toggle (CHANGED).
        - On the bar: shows track list (replaces maxPlayer if shown)
        - In trackList: returns to min
        - Visual state indicates which view is active
      -->
      <button 
        onclick={togglePlaylist} 
        class="text-neutral-300 hover:text-white p-2 {$mobileView === 'list' ? 'text-white bg-white/10' : ''} rounded-full" 
        aria-label="Toggle queue"
        aria-pressed={$mobileView === 'list'}
      >
        <ListMusic size={18} />
      </button>

      <!--
        Expand toggle (NEW on mobile bar).
        - On the bar: shows maxPlayer (replaces trackList if shown)
        - In maxPlayer: returns to min
      -->
      <button 
        onclick={toggleMaxPlayer} 
        class="text-neutral-300 hover:text-white p-2 {$mobileView === 'max' ? 'text-white bg-white/10' : ''} rounded-full" 
        aria-label="Toggle expanded player"
        aria-pressed={$mobileView === 'max'}
      >
        {#if $mobileView === 'max'}
          <Minimize2 size={18} />
        {:else}
          <Maximize2 size={18} />
        {/if}
      </button>
    </div>

    <!-- 2. maxPlayer view (only when active) -->
    {#if $mobileView === 'max'}
      <div class="fixed inset-0 bg-[#0e0e0e] z-50 flex flex-col p-6 pb-20">
        <div class="flex items-center justify-between mb-8">
          <h2 class="text-sm font-semibold uppercase tracking-wider text-neutral-400">Now Playing</h2>
          <!-- CHANGED: X close button on header. Same effect as the bar's Expand toggle. -->
          <button 
            onclick={toggleMaxPlayer} 
            class="text-neutral-300 hover:text-white p-2 rounded-full hover:bg-white/10" 
            aria-label="Minimize"
          >
            <Minimize2 size={24} />
          </button>
        </div>

        <div class="text-center mb-12 px-4">
          <h1 class="text-2xl font-bold text-white leading-tight mb-2">
            {currentTrack?.title ?? 'No track selected'}
          </h1>
          <p class="text-base text-neutral-400">{currentTrack?.speaker ?? ''}</p>
        </div>

        <div class="mb-8 px-2">
          <div 
            use:bindSeekBar
            class="h-2 bg-neutral-700 rounded-full relative cursor-pointer"
            role="slider"
            aria-label="Seek"
            aria-valuemin="0"
            aria-valuemax={duration || 0}
            aria-valuenow={currentTime}
            tabindex="0"
            onpointerdown={onSeekDown} 
            onpointermove={onSeekMove} 
            onpointerup={onSeekUp} 
            onpointercancel={onSeekUp}
          >
            <div class="absolute top-0 left-0 h-full bg-white rounded-full" style="width: {progress}%"></div>
          </div>
          <div class="flex justify-between text-xs text-neutral-400 mt-2 tabular-nums">
            <span>{formatTime(displayTime)}</span>
            <span>{formatTime(duration)}</span>
          </div>
        </div>

        <div class="flex items-center justify-center gap-6 mb-8">
          <button onclick={() => jump(-15)} class="text-white p-3" aria-label="Back 15s" disabled={!currentTrack}>
            <div class="flex flex-col items-center">
              <RotateCcw size={28} />
              <span class="text-[10px] mt-0.5">15</span>
            </div>
          </button>
          <button onclick={playPrev} class="text-white p-3" aria-label="Previous" disabled={!currentTrack}>
            <SkipBack size={32} />
          </button>
          <button onclick={togglePlayPause} class="bg-white text-black rounded-full w-20 h-20 flex items-center justify-center shadow-xl" aria-label="Play/Pause" disabled={!currentTrack}>
            {#if status === 'playing'}<Pause size={36} fill="currentColor" />{:else}<Play size={36} fill="currentColor" class="ml-1" />{/if}
          </button>
          <button onclick={playNext} class="text-white p-3" aria-label="Next" disabled={!currentTrack}>
            <SkipForward size={32} />
          </button>
          <button onclick={() => jump(30)} class="text-white p-3" aria-label="Forward 30s" disabled={!currentTrack}>
            <div class="flex flex-col items-center">
              <RotateCw size={28} />
              <span class="text-[10px] mt-0.5">30</span>
            </div>
          </button>
        </div>

        <div class="mt-auto space-y-4 pb-4">
          <div class="flex items-center justify-center gap-2">
            <button onclick={decreaseSpeed} class="text-neutral-300 hover:text-white w-10 h-10 rounded-full hover:bg-white/10 disabled:opacity-30 flex items-center justify-center text-lg leading-none" aria-label="Decrease speed" disabled={!currentTrack}>−</button>
            <button
              onpointerdown={onSpeedPointerDown}
              onpointerup={onSpeedPointerUp}
              onpointercancel={onSpeedPointerCancel}
              ondragstart={(e) => e.preventDefault()}
              class="text-sm text-neutral-300 hover:text-white min-w-16 px-4 py-2 rounded-full hover:bg-white/10 disabled:opacity-30 select-none"
              aria-label="Current speed. Long press to reset to 1.0x"
              disabled={!currentTrack}
            >
              {(currentTrack?.playbackRate ?? 1.0).toFixed(2)}x
            </button>
            <button onclick={increaseSpeed} class="text-neutral-300 hover:text-white w-10 h-10 rounded-full hover:bg-white/10 disabled:opacity-30 flex items-center justify-center text-lg leading-none" aria-label="Increase speed" disabled={!currentTrack}>+</button>
          </div>

          <div class="flex items-center justify-center gap-3">
            <button onclick={toggleMute} class="text-white p-2" aria-label="Mute">
              {#if isMuted || volume === 0}<VolumeX size={20} />{:else}<Volume2 size={20} />{/if}
            </button>
            <input type="range" min="0" max="1" step="0.01" value={volume} oninput={(e) => { volume = parseFloat(e.currentTarget.value); if (audioElement) audioElement.volume = volume; }} class="flex-1 accent-white" />
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}
