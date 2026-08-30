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
  import { fetchPresignedUrl } from '../lib/downloader.ts';
  import { removeTrackFromOpfs, getTrackBlob, saveTrackToOpfs } from '../lib/opfsStore.ts';
  import { isOnline } from '../lib/connectivityStore.ts';
  import { setTrackSwitching } from '../lib/transition.svelte.js';

  // ─── Controller Integration ───
  // Import the centralized business logic for queue bookmark management
  // and position snapshotting.
  import { setBookmark, snapshotCurrentPosition } from '../lib/playerController.js';

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

  // ─── Switch guard ───
  // Plain (non-reactive) flag: true while playTrack() is transitioning
  // between tracks. During the async loadTrack() window, the audio element
  // still holds the OUTGOING track's audio — but `currentTrack` may already
  // point at the INCOMING track (CASE 2/3 reassign it before the load).
  // Any timeupdate / pause / ended / durationchange event in that window
  // would attribute the old element's state to the WRONG track, corrupting
  // saved resume positions (e.g. the 'pause' fired by audioElement.load()
  // carries currentTime = 0, which would overwrite the incoming track's
  // saved position). All audio-element listeners check this flag.
  //
  // Deliberately NOT $state: nothing renders from this, and it flips too
  // fast to warrant effect scheduling.
  let isSwitching = false;

  /**
   * Failover guard: set true after a successful (or failed) OPFS
   * hot-swap attempt for the current audio source. Reset whenever a
   * new track is loaded. Prevents infinite swap loops if the OPFS
   * blob itself cannot be played.
   */
  let failoverAttempted = $state(false);

  /** Timer that delays the failover attempt while buffering,
   *  so a brief network hiccup doesn't trigger an unnecessary swap. */
  let bufferingTimer: ReturnType<typeof setTimeout> | null = null;

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
   */
  function commitQueue(): void {
    trackList.set(tracks);
    if (isQueueLoaded) {
      localStorage.setItem('labri_queue', JSON.stringify(tracks));
    }
  }

  // ─── Reactive Active Track Recovery ───
  // Restores the track marked `isActive: true` after a page reload
  // or persisted navigation.
  $effect(() => {
    if (!currentTrack && tracks.length > 0) {
      const activeTrack = $state.snapshot(tracks).find(t => t.isActive);
      if (activeTrack) {
        currentTrack = activeTrack;
        duration = activeTrack.duration ?? 0;
        currentTime = activeTrack.position ?? 0;
        
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

  /**
   * Derived flag: is the current track in the queue?
   * Used to determine navigation behavior:
   *   - In queue: SkipForward/SkipBack navigate the queue list
   *   - Not in queue (streaming/detour): SkipForward disabled,
   *     SkipBack = "Return to Bookmark"
   */
  let isCurrentTrackInQueue = $derived(
    currentTrack !== null && tracks.some(t => t.filename === currentTrack!.filename)
  );

  // ─── Mobile view toggles ───
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
   * `playTrack` can resume correctly on switch.
   */
  function updateCurrentTrackPosition(pos: number): void {
    if (!currentTrack) return;
    currentTrack.position = pos;
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
   * Load a track's audio source into the <audio> element.
   * 
   * PRIORITY ORDER (Local-First):
   *   1. OPFS (local disk cache) — instant, works offline
   *   2. In-memory valid presigned URL — avoids a network call
   *   3. Network — STREAM DIRECTLY from the presigned URL
   * 
   * FIXED (WebKitBlobResource error 3 on iPadOS): the network tier
   * previously downloaded the FULL audio file into one in-memory blob
   * and played it via a blob: URL ("single fetch" for playback +
   * caching). Safari/iPadOS — notably older devices under memory
   * pressure — fails to load large blob: URL media with
   * "WebKitBlobResource error 3", so playback silently died on the
   * affected iPad while working on Chrome/other devices.
   * 
   * The network tier now streams the presigned URL DIRECTLY into the
   * <audio> element. Safari/Chrome natively use HTTP Range requests:
   * playback starts as soon as the first chunk arrives, RAM usage is
   * near zero, and only the listened-to portion is downloaded.
   * The OPFS cache fetch runs as a SEPARATE background download so
   * offline-first behavior is fully preserved.
   */
  async function loadTrack(track: Track): Promise<void> {
    if (!audioElement) return;

    status = 'loading';
    failoverAttempted = false; // reset failover guard for the new source
    if (bufferingTimer) { 
      clearTimeout(bufferingTimer); 
      bufferingTimer = null; 
    }
    errorMessage = '';

    // ─── TIER 1: Try OPFS (Local Disk Cache) ───
    // Local blobs are small-risk (already on disk) and are how queued /
    // previously-cached tracks play offline. This tier is unchanged.
    if (track.hash) {
      const cachedBlob = await getTrackBlob(track.hash);
      if (cachedBlob) {
        // FIXED (race): waitForMetadata now attaches its 'loadedmetadata'
        // listener BEFORE we call load() below, so a fast local-blob
        // metadata parse can no longer slip past the listener.
        const metaReady = waitForMetadata(track);
        audioElement.src = URL.createObjectURL(cachedBlob);
        audioElement.load();
        audioElement.playbackRate = track.playbackRate ?? 1.0;
        await metaReady;
        return; // Successfully loaded from cache — done.
      }
    }

    // ─── TIER 2: Network — STREAM DIRECT ───
    // We don't pre-check $isOnline. The actual fetch IS the test.
    // If the user is offline OR the server is down, fetchPresignedUrl 
    // will throw. We catch it and inhibit all further progress.
    try {
      // Use existing presigned URL if still valid
      if (!track.url || !track.urlExpiresAt || track.urlExpiresAt < Date.now()) {
        const ticket = await fetchPresignedUrl(track.filename, apiBase);
        track.url = ticket.url;
        track.urlExpiresAt = ticket.expiresAt;
      }
      const finalUrl = track.url!;

      // FIXED (race): same listener-before-load ordering as the OPFS tier.
      const metaReady = waitForMetadata(track);
      // CHANGED (stream-direct): give the element the presigned URL
      // directly. The browser streams via HTTP Range requests —
      // near-zero RAM, instant start, only the listened-to portion
      // downloaded. No in-memory blob, no blob: URL.
      audioElement.src = finalUrl;
      audioElement.load();
      audioElement.playbackRate = track.playbackRate ?? 1.0;
      // ─── METADATA FIRST: Let the browser process the audio ───
      // The audio element needs the event loop free to parse the 
      // audio header and fire 'loadedmetadata'. We await that here;
      // with stream-direct this is fast (just the header bytes).
      await metaReady;

      // ─── BACKGROUND CACHE: Separate fetch, AFTER playback starts ───
      // CHANGED: the OPFS cache is now populated by its own background
      // download rather than sharing the playback fetch (there is no
      // playback fetch to share anymore — the browser streams internally).
      // Fire-and-forget: a cache failure can never affect playback.
      if (track.hash) {
        (async () => {
        try {
            const res = await fetch(finalUrl);
            if (!res.ok) return;
            const blob = await res.blob();
            await saveTrackToOpfs(track.hash!, blob);
            //console.log(`[Player] Background OPFS cache complete: ${track.filename}`);
        } catch (e) {
            // Cache failed, but playback still works this session.
          console.warn(`[Player] Background OPFS cache failed:`, e);
        }
        })();
      }

    } catch (err) {
      // ─── INHIBIT: Server unreachable ───
      // This catches BOTH:
      //   - "No internet" (TypeError: Failed to fetch)
      //   - "Server down" (5xx errors, DNS failures, CORS blocks)
      // The actual fetch attempt is the ground truth — more reliable 
      // than a cached $isOnline state.
      console.error('[Player] loadTrack FAILED with error:', err);
      console.error('[Player] Error type:', err instanceof Error ? err.constructor.name : typeof err);
      console.error('[Player] Error message:', err instanceof Error ? err.message : String(err));
      console.error('[Player] Error stack:', err instanceof Error ? err.stack : 'no stack');

      status = 'error';
      errorMessage = 'Library server unreachable. Please check your connection.';
      // Stop progress: no caching attempted. currentTrack REMAINS set —
      // the caller decides whether to revert the selection (CASE 2) or
      // show the error state in the bar (CASE 3).
      throw err;
    }
  }

  /**
   * Waits for the audio element to load the NEW track's metadata, then
   * restores the saved playback position.
   *
   * FIXED (resume position lost on track switch): the previous version
   * performed the restore seek inside 'loadedmetadata'. At HAVE_METADATA
   * the element's seekable ranges are not yet populated for blob URLs in
   * Chrome, so the seek silently snapped currentTime back to 0 — the UI
   * flashed the restored position (state variable) and then reverted when
   * the first real timeupdate reported the element's actual 0.00.
   *
   * The restore seek now happens on 'canplay' (readyState >= 3), when
   * seeking is guaranteed to work, and is verified one frame later with a
   * single retry if it still hasn't landed.
   *
   * ALSO FIXED: the readyState>=1 "fast path" has been REMOVED. Callers
   * invoke this BEFORE setting src (listener-first contract), so
   * readyState at call time reflects the PREVIOUS track — a fast-path
   * completion would read the old track's duration and seek the old
   * track's audio. We now ALWAYS wait for the next 'loadedmetadata',
   * which is guaranteed to fire because loadTrack() calls load()
   * immediately after invoking this helper.
   *
   * CONTRACT (unchanged): call BEFORE setting src / calling load(), then
   * await the returned promise after load() is invoked.
   */
  function waitForMetadata(track: Track): Promise<void> {
    return new Promise<void>((resolve) => {
      if (!audioElement) { resolve(); return; }
      const el = audioElement;

      // Capture the resume target up front, from the track's saved slot
      const resumeTarget =
        track.position && track.position > 0 && isFinite(track.position)
          ? track.position
          : null;

      const onMeta = () => {
        el.removeEventListener('loadedmetadata', onMeta);

        if (isFinite(el.duration) && el.duration > 0) {
          track.duration = el.duration;
          duration = el.duration;
        }

        // Nothing to restore — resolve immediately
        if (resumeTarget === null) { resolve(); return; }

        const doSeek = () => {
          const safe = Math.min(resumeTarget, el.duration - 0.5);
          el.currentTime = safe;
          currentTime = safe;
          // Verify the seek actually landed; retry once on the next frame
          // if the element discarded it.
          requestAnimationFrame(() => {
            if (Math.abs(el.currentTime - safe) > 1) {
              console.warn(`[Player] Seek did not stick (element at ${el.currentTime.toFixed(2)}), retrying`);
              el.currentTime = safe;
            }
            resolve();
          });
        };

        // Defer the seek until 'canplay': seeking is only reliable once
        // the browser has enough buffered data to populate seekable ranges.
        if (el.readyState >= 3) {
          doSeek();
        } else {
          const onCanPlay = () => {
            el.removeEventListener('canplay', onCanPlay);
            doSeek();
          };
          el.addEventListener('canplay', onCanPlay);
        }
      };

      el.addEventListener('loadedmetadata', onMeta);
    });
  }

  /**
   * Offline Hot-Swap: if the current track is streaming from the
   * network and a complete copy exists in OPFS, silently swap the
   * audio source to the local blob and resume at the stall position.
   *
   * Returns true if the swap succeeded. Called when the stream
   * stalls for a sustained period or a MEDIA_ERR_NETWORK occurs.
   *
   * Guardrails:
   *   - Only runs once per loaded source (failoverAttempted).
   *   - Skips if already playing from a blob: URL (already local).
   *   - Skips if the track has no hash or no OPFS copy exists.
   *
   * NOTE (stream-direct compatibility): a direct https:// src
   * correctly qualifies for failover — only blob: sources are skipped.
   */
  async function hotSwapToOpfs(): Promise<boolean> {
    if (!audioElement || !currentTrack || !currentTrack.hash) return false;
    if (audioElement.src.startsWith('blob:')) return false; // already local
    if (failoverAttempted) return false;
    failoverAttempted = true; // claim the attempt immediately (single attempt)

    const cachedBlob = await getTrackBlob(currentTrack.hash);
    if (!cachedBlob || !audioElement) return false;

    const track = currentTrack;
    const resumePos = audioElement.currentTime;
    const wasPlaying = status === 'playing' || status === 'buffering';

    isSwitching = true;
    try {
      audioElement.src = URL.createObjectURL(cachedBlob);
      audioElement.load();
      audioElement.playbackRate = track.playbackRate ?? 1.0;

      // Wait for seekable data, then restore the stall position.
      // Mirrors the resume pattern used in waitForMetadata().
      await new Promise<void>((resolve) => {
        const el = audioElement!;
        const doSeek = () => {
          if (isFinite(el.duration) && el.duration > 0) {
            el.currentTime = Math.min(resumePos, el.duration - 0.5);
          }
          resolve();
        };
        if (el.readyState >= 3) {
          doSeek();
        } else {
          const onCanPlay = () => {
            el.removeEventListener('canplay', onCanPlay);
            doSeek();
          };
          el.addEventListener('canplay', onCanPlay);
        }
      });

      currentTime = resumePos;
      status = 'buffering'; // cleared to 'playing'/'paused' by element events below

      if (wasPlaying) {
        await audioElement.play();
      }
      console.log(`[Player] Hot-swapped ${track.filename} to OPFS copy at ${resumePos.toFixed(1)}s`);
      return true;
    } catch (err) {
      console.warn('[Player] OPFS hot-swap failed:', err);
      return false;
    } finally {
      isSwitching = false;
    }
  }

  /**
   * Main entry point for playing a track.
   *
   * CASE 1: Same track → toggle play/pause
   * CASE 2: Track is in queue → load + play, update bookmark
   * CASE 3: Track not in queue → load + play (streaming detour)
   *
   * The bookmark (`isActive`) is only updated when playing FROM the queue
   * (CASE 2). Streaming (CASE 3) leaves the bookmark untouched so SkipBack
   * can return the user to their queued lecture.
   */
  async function playTrack(track: Track): Promise<void> {
    if (!audioElement) return;

    // CASE 1: Same track → toggle
    if (currentTrack?.filename === track.filename) {
      setTrackSwitching(false); // No transition occurs — release the switch lock
      await togglePlayPause();
      return;
    }

    // CASE 2: Existing track (exists in queue) → Load + Resume + Play
    //
    // FIXED: previously this branch switched `currentTrack` and called
    // performPlay() WITHOUT calling loadTrack(). The audio element kept
    // the PREVIOUS track's src, so clicking a queued track updated the UI
    // but kept playing (or re-seeking) the old audio — and after a page
    // reload, when the audio element had no src at all, it did nothing.
    //
    // The corrected flow mirrors CASE 3's discipline:
    //   freeze outgoing position → switch currentTrack → loadTrack()
    //   (which restores the saved position via waitForMetadata) → play,
    //   with error inhibition that reverts the selection if load fails.
    const existingIndex = tracks.findIndex(t => t.filename === track.filename);
    if (existingIndex !== -1) {
      // Freeze the outgoing track's position before switching
      if (currentTrack && currentTrack.filename !== track.filename) {
        const finalPos = audioElement.currentTime;
        updateCurrentTrackPosition(finalPos);
        commitQueue();
      }

      // Switch selection. Keep a reference to the previous track so we
      // can revert the UI if the load below fails.
      const previousTrack = currentTrack;
      const nextTrack = tracks[existingIndex];
      currentTrack = nextTrack;
      // ─── Update the Queue Bookmark ───
      // FIXED: setBookmark() only updates the STORE. commitQueue() below
      // overwrites the store with local `tracks`, so the bookmark change
      // must ALSO be mirrored into local state — otherwise the bookmark
      // silently reverts to the last handleAddToQueue assignment, and
      // detour-mode SkipBack returns to the wrong track.
      tracks = tracks.map(t => ({ ...t, isActive: t.filename === nextTrack.filename }));
      setBookmark(nextTrack.filename);
      commitQueue(); // ← structural: active-row indicator moves immediately

      isSwitching = true; // Suppress listener position-writes during load
      try {
        // Load the new track's audio into the element. waitForMetadata
        // (inside loadTrack) restores nextTrack.position safely, clamped
        // against the NEW track's real duration — so the old manual
        // `audioElement.currentTime = ...` seek has been REMOVED; it ran
        // before the new metadata was known and could clamp against the
        // wrong track's duration.
        await loadTrack(nextTrack);
      } catch (err: any) {
        // ─── INHIBIT: same discipline as CASE 3 ───
        // Revert the selection rather than leaving the UI showing a
        // track whose audio never loaded.
        console.error('[Player] Switch load failed, reverting selection:', err);
        currentTrack = previousTrack;
        commitQueue();
        return; // status/errorMessage already set inside loadTrack
      } finally {
        isSwitching = false; // Always release — success or failure
        setTrackSwitching(false); // Transition finished — release the switch lock
      }

      await performPlay();
      return;
    }

    // CASE 3: Track not in queue → Stream (Detour)
    // The bookmark remains on the last queued track, if any.

    // Freeze the OUTGOING track's position FIRST, while its audio is still
    // loaded in the element. (Previously this ran AFTER loadTrack(), when
    // audioElement.currentTime already belonged to the NEW track — which
    // wiped the outgoing track's resume position.)
    if (currentTrack) {
      // Only update position if the outgoing track is in the queue
      // (streaming tracks don't have a persistent position slot)
      if (tracks.some(t => t.filename === currentTrack!.filename)) {
      updateCurrentTrackPosition(audioElement.currentTime);
      }
    }

    // ─── SET STATE (EARLY) ───
    // FIXED: currentTrack was previously assigned only AFTER loadTrack()
    // completed, so the player bar did not appear until the entire load
    // finished — and if the load stalled or failed (notably Safari/iPadOS),
    // the bar never appeared at all. Setting state first shows the bar
    // (with the loading spinner) immediately. The isSwitching guard below
    // already protects listeners from misattributing the outgoing track.
    // On load failure, currentTrack stays set so the bar displays the
    // error state — the user gets visible feedback, not silence.
    currentTrack = track;
    currentTime = track.position ?? 0;
    duration = track.duration ?? 0;

    track.loading = true;          // Trigger spinner

    isSwitching = true;            // Suppress listener position-writes during load
    try {                          // Stream the track (may throw if offline or server unreachable)
      await loadTrack(track);
      } catch (err: any) {
      // ─── INHIBIT: The resolution failed. Abort playback. ───
        console.error('[Player] loadTrack failed, inhibiting track:', err);
        status = 'error';
        errorMessage = err.message || 'Track is not available.'; 
      track.loading = false;
        return; // ← The track is NOT added to the queue. No "ghost" track.
               // currentTrack stays set: the bar shows the error state.
    } finally {
      isSwitching = false;
      setTrackSwitching(false); // Transition finished — release the switch lock
    }

    // Track loaded successfully. Clear spinner.
    track.loading = false;
    
    await performPlay(); // Trigger play
  }

  // Refactored helper to maintain your clean separation
  async function performPlay() {
    try {
      await audioElement!.play();
      status = 'playing';
    } catch (err: any) {
      console.error('Play error:', err);
      status = err.name === 'NotAllowedError' ? 'paused' : 'error';
      if (status === 'error') errorMessage = err.message;
    }
  }

  /**
   * Skip to the next track.
   *
   * BEHAVIOR:
   *   - If the current track is in the queue and there is a next track,
   *     advance to it.
   *   - If the current track is NOT in the queue (streaming/detour),
   *     SkipForward is disabled (no-op). The user must return to the
   *     queue via SkipBack or select a new track.
   */
  function playNext() {
    if (!currentTrack) return;

    // ─── Detour Mode: SkipForward disabled ───
    if (!isCurrentTrackInQueue) {
      // No-op: forward navigation has no meaning when streaming.
      // The UI button should be disabled, but this guard prevents
      // programmatic calls from advancing unexpectedly.
      return;
    }

    // ─── Queue Mode: Advance to next track ───
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    if (idx !== -1 && idx < tracks.length - 1) {
      playTrack(tracks[idx + 1]);
  }
    // If we're at the end of the queue, do nothing (don't loop).
  }

  /**
   * Skip to the previous track.
   *
   * BEHAVIOR:
   *   - If the current track is in the queue: standard "Back" behavior.
   *     If more than 3 seconds in, restart the current track.
   *     If in the first 3 seconds, go to the previous queue item.
   *   - If the current track is NOT in the queue (streaming/detour):
   *     "Return to Bookmark" — find the track marked `isActive: true`
   *     in the queue and resume playing from its saved position.
   *     If the queue is empty, restart the current stream.
   */
  function playPrev() {
    if (!currentTrack || !audioElement) return;

    // ─── Detour Mode: Return to Bookmark ───
    if (!isCurrentTrackInQueue) {
      // Find the bookmarked track in the queue
      const bookmark = tracks.find(t => t.isActive);
      if (bookmark) {
        // Jump to the bookmarked track and resume from its saved position
        playTrack(bookmark);
        return;
      }
      
      // Queue is empty — restart the current stream
      audioElement.currentTime = 0;
      currentTime = 0;
      return;
    }

    // ─── Queue Mode: Standard "Back" behavior ───
    const idx = tracks.findIndex(t => t.filename === currentTrack!.filename);
    
    // If more than 3 seconds in, restart the current track
    if (audioElement.currentTime > 3) {
      audioElement.currentTime = 0;
      currentTime = 0;
      return;
    }
    
    // If in the first 3 seconds and there is a previous track, go to it
    if (idx > 0) {
      playTrack(tracks[idx - 1]);
    }
    // If at the first track, restart it
    else {
      audioElement.currentTime = 0;
      currentTime = 0;
    }
  }

  function jump(seconds: number) {
    if (audioElement) audioElement.currentTime = Math.max(0, audioElement.currentTime + seconds);
  }

  /**
   * Remove a track from the queue.
   *
   * If the removed track was the active bookmark, the bookmark is
   * reassigned to the next available track (or cleared if the queue
   * becomes empty).
   */
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
    const wasBookmarked = trackToRemove?.isActive;
    
    // Filter out the track
    tracks = tracks.filter(t => t.filename !== filename);
    commitQueue();
    
    // ─── Handle Bookmark Reassignment ───
    // If the removed track was the active bookmark, assign the bookmark
    // to the first track in the queue (if any remain).
    if (wasBookmarked && tracks.length > 0) {
      setBookmark(tracks[0].filename);
      // Update local tracks to reflect the store's bookmark change
      tracks = tracks.map((t, i) => ({
        ...t,
        isActive: i === 0
      }));
      commitQueue();
    } else if (wasBookmarked && tracks.length === 0) {
      // Queue is empty — clear the bookmark
      setBookmark(null);
    }
    
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
        // Play the first track in the queue after removal
        const nextTrack = tracks[0];
        playTrack(nextTrack);
      }
    }
  }

  /**
   * Reorder the queue. Triggered by QueueDrawer's drag-and-drop.
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
  // Three-button cluster: − decreases by 0.1, + increases by 0.1,
  // middle button shows current speed; long-press (~500ms) resets to 1.0x.
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
    // 2. Restore the Active Track (handled by the reactive $effect above)
    // 3. Bind Audio Element Listeners
    // 4. Bind External Event Listeners
    const savedQueue = localStorage.getItem('labri_queue');
    if (savedQueue) {
      try {
        const parsed = JSON.parse(savedQueue);
        if (Array.isArray(parsed)) {
          tracks = parsed;
        }
      } catch (e) {
        console.error("[Player] Failed to restore queue from localStorage", e);
      }
    }
    isQueueLoaded = true;
    commitQueue(); // Sync the store so QueueDrawer sees the restored list immediately
    
    if (audioElement) {
      audioElement.volume = volume;

      // Live playback tick — drives the player's own seek bar via the
      // currentTimeStore mirror. We also keep the in-memory resume
      // position fresh on the current track, but we deliberately do NOT
      // call commitQueue() here. The drawer renders frozen per-track
      // progress and a static placeholder for the current track; a
      // position update mid-playback is not a structural event.
      audioElement.addEventListener('timeupdate', () => {
        // isSwitching guard: mid-switch ticks belong to the OUTGOING
        // track's audio, but currentTrack may already be the INCOMING
        // track — an unguarded tick would write the old track's live
        // position into the new track's slot.
        if (audioElement && currentTrack && !isSwitching) {
          if (status === 'playing' || status === 'paused') {
            const t = audioElement.currentTime;
            currentTime = t;
            updateCurrentTrackPosition(t);
          }
        }
      });

      audioElement.addEventListener('durationchange', () => {
        // isSwitching guard: during a CASE 3 load, currentTrack is still
        // the outgoing track — don't stamp the NEW track's duration onto it.
        if (audioElement && currentTrack && !isSwitching && isFinite(audioElement.duration)) {
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
        // isSwitching guard: loadTrack() calls audioElement.load(), which
        // fires 'pause' on the outgoing audio with currentTime reset to 0.
        // Without this guard, that 0 would be written into the INCOMING
        // track's slot — the root cause of "track always reverts to 0".
        if (currentTrack && audioElement && !isSwitching) {
          updateCurrentTrackPosition(audioElement.currentTime);
          commitQueue(); // ← structural: position frozen at pause
        }
      });

      audioElement.addEventListener('ended', () => {
        // isSwitching guard: a stale 'ended' from the outgoing audio must
        // not zero the incoming track's position or double-advance the
        // queue — the in-flight playTrack switch owns what plays next.
        if (isSwitching) return;
        // Track ended — finalize position to 0 (Spotify-style: ended
        // tracks are "ready to replay from start", not 100% complete).
        if (currentTrack) {
          updateCurrentTrackPosition(0);
          commitQueue(); // ← structural: position finalized at end
        }
        playNext();
      });

      audioElement.addEventListener('error', () => {
        // MEDIA_ERR_NETWORK (code 2) = connection lost mid-stream.
        // Attempt OPFS failover before surfacing an error to the user.
        if (audioElement?.error?.code === MediaError.MEDIA_ERR_NETWORK && !failoverAttempted) {
          hotSwapToOpfs().then((ok) => {
            if (!ok) {
              status = 'error';
              errorMessage = 'Connection lost. Track is not queued for offline playback.';
            }
          });
          return;
        }
        status = 'error'; 
        errorMessage = 'Playback error';
      });

      // ─── Offline Hot-Swap Triggers ───

      // Stream stalled waiting for data. Show buffering state, then
      // give the network a grace period before failing over to OPFS.
      audioElement.addEventListener('waiting', () => {
        if (isSwitching) return;
        status = 'buffering';

        // Offline already → attempt failover quickly; online → grace period
        const graceMs = navigator.onLine ? 3000 : 500;
        if (bufferingTimer) clearTimeout(bufferingTimer);
        bufferingTimer = setTimeout(async () => {
          if (status !== 'buffering') return; // recovered in the meantime
          await hotSwapToOpfs();
          // If swap failed, stay in 'buffering' — playback resumes when
          // the network recovers and the browser refills the buffer.
        }, graceMs);
      });

      // Playback resumed — clear buffering state and cancel any pending failover.
      audioElement.addEventListener('playing', () => {
        if (bufferingTimer) { clearTimeout(bufferingTimer); bufferingTimer = null; }
        if (status === 'buffering') status = 'playing';
      });

    }

    // External event listeners for QueueDrawer and TrackList
    const handlePlay = (e: Event) => playTrack((e as CustomEvent).detail);
    const handleRemove = (e: Event) => removeFromQueue((e as CustomEvent).detail.filename);
    const handleReorder = (e: Event) => {
      const { filename, newIndex } = (e as CustomEvent).detail;
      reorderQueue(filename, newIndex);
    };

    /**
     * Handle "add-to-queue" event from the controller.
     *
     * Adds the track to the queue, sets the bookmark, and triggers
     * background OPFS caching. Does NOT auto-play — the user must
     * click Play to start streaming.
     *
     * This is the "Promotion" path: a track moves from "transient"
     * (streaming) to "persistent" (queued) without interrupting
     * the current audio.
     */
    const handleAddToQueue = (e: Event) => {
      const track = (e as CustomEvent).detail as Track;
      
      // Prevent duplicate queue entries
      if (tracks.some(t => t.filename === track.filename)) return;
      
      // Add to queue (appended at the end)
      tracks = [...tracks, track];
      
      // Update the bookmark: this track becomes the active one
      setBookmark(track.filename);
      
      // Mirror the bookmark change into local state
      tracks = tracks.map(t => ({
        ...t,
        isActive: t.filename === track.filename
      }));
      
      commitQueue();
      
      // ─── Background OPFS Caching ───
      // Fetch the track and save to OPFS so it's available offline
      // for future plays. This runs in the background — the user
      // can continue listening to whatever is currently playing.
      if (track.hash) {
        (async () => {
          // Mark the track as loading in the queue
          const loadingIdx = tracks.findIndex(t => t.filename === track.filename);
          if (loadingIdx !== -1) {
            tracks[loadingIdx] = { ...tracks[loadingIdx], loading: true };
            commitQueue();
          }
          
          try {
            const ticket = await fetchPresignedUrl(track.filename, apiBase);
            const response = await fetch(ticket.url);
            if (!response.ok) throw new Error(`Fetch failed: ${response.status}`);
            const blob = await response.blob();
            await saveTrackToOpfs(track.hash!, blob);
            
            // Mark as downloaded and clear loading
            if (loadingIdx !== -1) {
              tracks[loadingIdx] = { 
                ...tracks[loadingIdx], 
                loading: false, 
                isDownloaded: true 
              };
              commitQueue();
            }
          } catch (err) {
            console.warn(`[Player] Background cache failed for ${track.filename}:`, err);
            // Clear loading state even on failure
            if (loadingIdx !== -1) {
              tracks[loadingIdx] = { ...tracks[loadingIdx], loading: false };
              commitQueue();
            }
          }
        })();
      }
    };

    window.addEventListener('play-track', handlePlay);
    window.addEventListener('add-to-queue', handleAddToQueue);
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

    const handleTogglePlay = () => togglePlayPause();
    window.addEventListener('toggle-play', handleTogglePlay);

    return () => {
      window.removeEventListener('play-track', handlePlay);
      window.removeEventListener('add-to-queue', handleAddToQueue);
      window.removeEventListener('remove-from-queue', handleRemove);
      window.removeEventListener('reorder-queue', handleReorder);
      window.removeEventListener('keydown', handleGlobalReorder);
      window.removeEventListener('pagehide', flushPosition);
      window.removeEventListener('beforeunload', flushPosition);
      window.removeEventListener('toggle-play', handleTogglePlay);
    };
  });
</script>

<audio bind:this={audioElement} preload="auto"></audio>

<!-- DESKTOP LAYOUT -->
{#if tracks.length > 0 || currentTrack !== null}
  <!-- CHANGED: removed `fixed bottom-0 left-0 right-0 z-40` — the bar
       is now an in-flow flex child (LabriLayout body column). AppShell
       sizes itself to end at this bar's top edge. -->
  <div class="hidden md:flex h-24 bg-[#0e0e0e] border-t border-neutral-800 items-center px-6">
    
    <div class="flex-1 min-w-0 flex flex-col justify-center">
      {#if status === 'error' && errorMessage}
        <!-- NEW: visible error state — failures were previously silent -->
        <div class="text-sm font-semibold truncate text-red-400">{errorMessage}</div>
        <div class="text-xs text-neutral-500 truncate">Playback unavailable</div>
      {:else if currentTrack}
        <div class="text-sm font-semibold truncate text-white">{currentTrack.title ?? currentTrack.filename}</div>
        <div class="text-xs text-neutral-400 truncate">{currentTrack.speaker ?? 'Unknown Speaker'}</div>
      {:else}
        <div class="text-sm text-neutral-500">No track selected</div>
      {/if}
    </div>

    <div class="flex-1 max-w-2xl flex flex-col items-center gap-2">
      <div class="flex items-center gap-4">
        <!-- CHANGED: Previous now disables on the first queue track when
             ≤3s in (the only state where playPrev is a true no-op). -->
        <button onclick={playPrev} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Previous" disabled={!currentTrack || (isCurrentTrackInQueue && tracks.findIndex(t => t.filename === currentTrack?.filename) === 0 && currentTime <= 3)}>
          <SkipBack size={20} />
        </button>
        <button onclick={() => jump(-15)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Back 15s" disabled={!currentTrack}>
          <RotateCcw size={20} />
        </button>
        <button onclick={togglePlayPause} class="bg-white text-black rounded-full w-11 h-11 flex items-center justify-center hover:scale-105 transition disabled:opacity-30 shadow-lg" aria-label="Play/Pause" disabled={!currentTrack}>
          {#if status === 'playing'}<Pause size={22} fill="currentColor" />{:else if status === 'loading' || status === 'buffering'}<span class="loading-spinner text-black" style="width:22px;height:22px;">⏳</span>{:else}<Play size={22} fill="currentColor" class="ml-0.5" />{/if}
        </button>
        <button onclick={() => jump(30)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Forward 30s" disabled={!currentTrack}>
          <RotateCw size={20} />
        </button>
        <button onclick={playNext} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10 disabled:opacity-30" aria-label="Next" disabled={!currentTrack || !isCurrentTrackInQueue || (tracks.findIndex(t => t.filename === currentTrack?.filename) >= tracks.length - 1)}>
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
        Speed control: three-button cluster.
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

      <button onclick={() => desktopQueueOpen.update(v => !v)} class="text-neutral-300 hover:text-white p-1.5 rounded-full hover:bg-white/10" aria-label="Toggle queue">
        <ListMusic size={20} />
      </button>
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
    <!-- 1. Persistent bottom bar
         The pill keeps its floating visual (absolute, bottom-4, rounded,
         shadow) inside an in-flow `relative h-20` container so its space
         is RESERVED in the layout — page content scrolls to a stop above
         it. z-60 keeps the pill above the maxPlayer overlay (z-50). -->
    <div class="relative h-20">
      <div class="absolute bottom-4 left-1/2 -translate-x-1/2 w-[calc(100%-2rem)] max-w-md h-14 bg-[#181818] border border-neutral-800 rounded-full px-4 flex items-center gap-3 z-60 shadow-2xl">
        <button onclick={togglePlayPause} class="text-white p-1" aria-label="Play/Pause" disabled={!currentTrack}>
          {#if status === 'playing'}<Pause size={20} fill="currentColor" />{:else if status === 'loading' || status === 'buffering'}<span class="loading-spinner" style="width:20px;height:20px;">⏳</span>{:else}<Play size={20} fill="currentColor" class="ml-0.5" />{/if}
        </button>
        <div class="flex-1 min-w-0">
          {#if status === 'error' && errorMessage}
            <!-- NEW: visible error state — failures were previously silent -->
            <div class="text-sm font-medium truncate text-red-400">{errorMessage}</div>
            <div class="text-xs text-neutral-500 truncate">Playback unavailable</div>
          {:else}
            <div class="text-sm font-medium truncate text-white">{currentTrack?.title ?? 'No track'}</div>
            <div class="text-xs text-neutral-400 truncate">{currentTrack?.speaker ?? ''}</div>
          {/if}
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
          Expand toggle.
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
    </div>

    <!-- 2. maxPlayer view (only when active) -->
    {#if $mobileView === 'max'}
      <div class="fixed inset-0 bg-[#0e0e0e] z-50 flex flex-col p-6 pb-20">
        <div class="flex items-center justify-between mb-8">
          <h2 class="text-sm font-semibold uppercase tracking-wider text-neutral-400">Now Playing</h2>
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
          <!-- NEW: visible error state in the expanded player -->
          {#if status === 'error' && errorMessage}
            <p class="text-sm text-red-400 mt-2">{errorMessage}</p>
          {/if}
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
            {#if status === 'playing'}<Pause size={36} fill="currentColor" />{:else if status === 'loading' || status === 'buffering'}<span class="loading-spinner text-black" style="width:36px;height:36px;">⏳</span>{:else}<Play size={36} fill="currentColor" class="ml-1" />{/if}
          </button>
          <button onclick={playNext} class="text-white p-3" aria-label="Next" disabled={!currentTrack || !isCurrentTrackInQueue || (tracks.findIndex(t => t.filename === currentTrack?.filename) >= tracks.length - 1)}>
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
