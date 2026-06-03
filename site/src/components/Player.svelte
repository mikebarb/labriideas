<script lang="ts">
  // ──────────────────────────────────────────────
  // Types
  // ──────────────────────────────────────────────
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';

  type TrackMeta = Record<string, any>;

  interface Track {
    id: string;
    filename: string;
    hash: string;
    metadata: TrackMeta;
    playbackRate?: number;
    title?: string;
    artist?: string;
  }

  type PlaybackStatus =
    | 'idle'
    | 'loading'
    | 'playing'
    | 'paused'
    | 'buffering'
    | 'error';

  interface PresignedTicket {
    url: string;
    expiresAt: number;
  }

  // ──────────────────────────────────────────────
  // Props
  // ──────────────────────────────────────────────
  interface Props {
    /** Base URL for the Go proxy (defaults to origin) */
    apiBase?: string;
    /** Presigned URL TTL in seconds (must match server-side config) */
    urlTtlSeconds?: number;
    /** Seconds before expiry to proactively refresh the URL */
    refreshBufferSeconds?: number;
    /** Admin mode flag — placeholder until JWT is wired */
    isAdmin?: boolean;
    /** Autoplay the first track on mount */
    autoplay?: boolean;
    /** Hide the built-in tracklist if CatalogViewer handles it */
    showTracklist?: boolean;
    /** children snippet for custom layout override */
    children?: Snippet;
  }

  let {
    apiBase = '',
    urlTtlSeconds = 3600,
    refreshBufferSeconds = 120,
    isAdmin = false,
    autoplay = false,
    showTracklist = true,
    children,
  }: Props = $props();

  // ──────────────────────────────────────────────
  // Reactive State
  // ──────────────────────────────────────────────
  // Tracks are now internal state, populated by the 'catalog-loaded' event
  let tracks: Track[] = $state([]); 
  
  let currentTrack: Track | null = $state(null);
  let status: PlaybackStatus = $state('idle');
  let currentTime: number = $state(0);
  let duration: number = $state(0);
  let buffered: number = $state(0);
  let volume: number = $state(0.8);
  let isMuted: boolean = $state(false);
  let errorMessage: string = $state('');
  let retryCount: number = $state(0);
  let playbackRate: number = $state(1.0);


  // Internal references
  let audioElement: HTMLAudioElement | null = null;
  let presignedTicket: PresignedTicket | null = null;
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;
  const MAX_RETRIES = 3;

  let seekBarElement: HTMLElement | null = $state(null);
  let isDragging: boolean = $state(false);
  let dragProgress: number = $state(0);

  // ──────────────────────────────────────────────
  // Derived
  // ──────────────────────────────────────────────
  //let progress: number = $derived(
  //  duration > 0 ? (currentTime / duration) * 100 : 0
  //);
  // If we are dragging, show the drag position. Otherwise, show actual time.
  let progress: number = $derived(
    isDragging ? dragProgress : (duration > 0 ? (currentTime / duration) * 100 : 0)
  );

  // If we are dragging, calculate the time based on drag position. 
  // Otherwise, use the actual audio current time.
  let displayTime: number = $derived(
    isDragging ? (dragProgress / 100) * duration : currentTime
  );

  let displayTitle: string = $derived(
    currentTrack?.title ?? 'No track selected'
  );

  let displayArtist: string = $derived(
    currentTrack?.artist ?? 'Unknown Artist'
  );

  let statusLabel: string = $derived(
    status === 'idle'      ? '' :
    status === 'loading'   ? 'Loading…' :
    status === 'playing'   ? 'Playing' :
    status === 'paused'    ? 'Paused' :
    status === 'buffering' ? 'Buffering…' :
    status === 'error'     ? `Error: ${errorMessage}` :
    ''
  );

  // ──────────────────────────────────────────────
  // Presigned URL Manager
  // ──────────────────────────────────────────────
  async function fetchPresignedUrl(filename: string): Promise<PresignedTicket> {
    const endpoint = `${apiBase}/api/download?file=${encodeURIComponent(filename)}`;
    const res = await fetch(endpoint);

    if (!res.ok) {
      const body = await res.text().catch(() => '');
      throw new Error(`Failed to get presigned URL (${res.status}): ${body}`);
    }

    const contentType = res.headers.get('content-type') ?? '';
    let url: string;

    if (contentType.includes('application/json')) {
      const json = await res.json();
      url = json.url ?? json;
    } else {
      url = await res.text();
    }

    return {
      url,
      expiresAt: Date.now() + urlTtlSeconds * 1000,
    };
  }

  function scheduleUrlRefresh() {
    clearRefreshTimer();
    if (!presignedTicket || !currentTrack) return;

    const msUntilExpiry = presignedTicket.expiresAt - Date.now();
    const bufferMs = refreshBufferSeconds * 1000;
    const refreshIn = Math.max(msUntilExpiry - bufferMs, 5000);

    refreshTimer = setTimeout(async () => {
      try {
        presignedTicket = await fetchPresignedUrl(currentTrack!.filename);
        if (audioElement && (status === 'playing' || status === 'buffering')) {
          const wasPlaying = !audioElement.paused;
          audioElement.src = presignedTicket.url;
          audioElement.currentTime = currentTime;
          if (wasPlaying) audioElement.play();
        }
        scheduleUrlRefresh();
      } catch (err) {
        console.error('Proactive URL refresh failed:', err);
        refreshTimer = setTimeout(() => scheduleUrlRefresh(), 30_000);
      }
    }, refreshIn);
  }

  function clearRefreshTimer() {
    if (refreshTimer !== null) {
      clearTimeout(refreshTimer);
      refreshTimer = null;
    }
  }

  // ──────────────────────────────────────────────
  // Audio Element Lifecycle (Svelte 5 Idiomatic)
  // ──────────────────────────────────────────────
  $effect(() => {
    const el = audioElement;
    if (!el) return; // Wait until the element actually exists in the DOM

    const handleCanPlay = () => {
      if (status === 'loading' || status === 'buffering') status = 'playing';
    };
    const handlePlay = () => { status = 'playing'; };
    const handlePause = () => {
      if (!el.ended && el.error === null && status !== 'loading') status = 'paused';
    };
    const handleTimeUpdate = () => { currentTime = el.currentTime; };
    const handleDurationChange = () => {
      if (el.duration && isFinite(el.duration)) duration = el.duration;
    };
    const handleProgress = () => {
      if (el.buffered.length > 0) buffered = el.buffered.end(el.buffered.length - 1);
    };
    const handleWaiting = () => {
      if (status === 'playing') status = 'buffering';
    };
    const handleEnded = () => { playNext(); };
    const handleError = () => { handleAudioError(el.error); };
    const handleStalled = () => {
      if (status === 'playing') status = 'buffering';
    };

    el.addEventListener('canplay', handleCanPlay);
    el.addEventListener('play', handlePlay);
    el.addEventListener('pause', handlePause);
    el.addEventListener('timeupdate', handleTimeUpdate);
    el.addEventListener('durationchange', handleDurationChange);
    el.addEventListener('progress', handleProgress);
    el.addEventListener('waiting', handleWaiting);
    el.addEventListener('ended', handleEnded);
    el.addEventListener('error', handleError);
    el.addEventListener('stalled', handleStalled);

    // Cleanup function: removes listeners if component is destroyed
    return () => {
      el.removeEventListener('canplay', handleCanPlay);
      el.removeEventListener('play', handlePlay);
      el.removeEventListener('pause', handlePause);
      el.removeEventListener('timeupdate', handleTimeUpdate);
      el.removeEventListener('durationchange', handleDurationChange);
      el.removeEventListener('progress', handleProgress);
      el.removeEventListener('waiting', handleWaiting);
      el.removeEventListener('ended', handleEnded);
      el.removeEventListener('error', handleError);
      el.removeEventListener('stalled', handleStalled);
    };
  });

  function handleAudioError(error: MediaError | null) {
    if (!error) {
      errorMessage = 'Unknown audio error';
      status = 'error';
      return;
    }

    switch (error.code) {
      case MediaError.MEDIA_ERR_ABORTED:
        errorMessage = 'Playback aborted';
        status = 'idle';
        return;

      case MediaError.MEDIA_ERR_NETWORK:
        errorMessage = 'Network error — URL may have expired';
        attemptRecovery();
        return;

      case MediaError.MEDIA_ERR_DECODE:
        errorMessage = 'Decode error — file may be corrupted';
        status = 'error';
        return;

      case MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED:
        errorMessage = 'Source not supported — attempting URL refresh';
        attemptRecovery();
        return;

      default:
        errorMessage = `Error code ${error.code}`;
        status = 'error';
    }
  }

  async function attemptRecovery() {
    if (retryCount >= MAX_RETRIES) {
      status = 'error';
      errorMessage = `Failed after ${MAX_RETRIES} retries: ${errorMessage}`;
      return;
    }

    retryCount++;
    status = 'buffering';
    console.warn(`Recovery attempt ${retryCount}/${MAX_RETRIES}`);

    try {
      presignedTicket = await fetchPresignedUrl(currentTrack!.filename);

      if (audioElement) {
        audioElement.src = presignedTicket.url;
        audioElement.currentTime = currentTime;
        await audioElement.play();
      }

      retryCount = 0;
      scheduleUrlRefresh();
    } catch (err: any) {
      errorMessage = err.message;
      retryTimer = setTimeout(
        () => attemptRecovery(),
        2000 * Math.pow(2, retryCount - 1)
      );
    }
  }

 // ──────────────────────────────────────────────
  // Seek Bar Drag Logic (Add these functions)
  // ──────────────────────────────────────────────
  function calculateProgress(e: PointerEvent): number {
    if (!seekBarElement) return 0;
    const rect = seekBarElement.getBoundingClientRect();
    const x = e.clientX - rect.left;
    return Math.max(0, Math.min(100, (x / rect.width) * 100));
  }

  function handleSeekPointerDown(e: PointerEvent) {
    if (!seekBarElement) return;
    isDragging = true;
    
    // Capture the pointer so we track mouse movement even outside the bar
    seekBarElement.setPointerCapture(e.pointerId);
    
    dragProgress = calculateProgress(e);
  }

  function handleSeekPointerMove(e: PointerEvent) {
    if (!isDragging) return;
    dragProgress = calculateProgress(e);
  }

  function handleSeekPointerUp(e: PointerEvent) {
    if (!isDragging) return;
    isDragging = false;
    
    // Apply the seek now that the user let go
    seekTo(dragProgress);
  }


  // ──────────────────────────────────────────────
  // Playback Controls
  // ──────────────────────────────────────────────
  async function playTrack(track: Track) {
    console.log('Attempting to play track:', track);
    clearRefreshTimer();
    retryCount = 0;
    currentTime = 0;
    duration = 0;
    buffered = 0;
    errorMessage = '';

    // Check if the track is already in our queue
    let trackToPlay = tracks.find((t) => t.filename === track.filename);

    if (!trackToPlay) {
      // --- METADATA EXTRACTION LOGIC ---
      console.log("playTrack - passed metadata: ", track.metadata);
      //const meta = track.metadata || {};
      console.log(`playTrack - Extracting metadata for ${track.filename}: `, track);
      // 1. Extract Title (fallback to filename if missing)
      const extractedTitle = 
        track.title || track.filename.replace(/\.[^/.]+$/, ""); // removes .mp3 extension if used as fallback
      // 2. Extract Artist/Author
      const extractedArtist = 
        track.artist || 'Unknown Artist';
      // If not in queue, create a new entry with a guaranteed unique ID (using filename)
      trackToPlay = {
        ...track,
        id: track.filename,
        playbackRate: 1.0,
        title: extractedTitle,
        artist: extractedArtist
      }; 
      // Add it to the end of the queue
      tracks = [...tracks, trackToPlay];
    }

    //currentTrack = track;
    currentTrack = trackToPlay;
    // 1. Force the status to 'loading' first to show the spinner
    status = 'loading';
    
    // Apply the track's specific playback rate immediately
    if (audioElement) {
      audioElement.playbackRate = trackToPlay.playbackRate || 1.0;
    }

    try {
      //presignedTicket = await fetchPresignedUrl(track.filename);
      presignedTicket = await fetchPresignedUrl(trackToPlay.filename);

      if (audioElement) {
        audioElement.src = presignedTicket.url;
        audioElement.load();
        //await audioElement.play();
      }

      // CRITICAL: Set the playback rate AFTER setting the src!
      // Browsers reset the rate to 1.0 when the source changes.
      audioElement.playbackRate = trackToPlay.playbackRate || 1.0;

      // 2. Wrap the play attempt
      const playPromise = audioElement.play();
      
      if (playPromise !== undefined) {
        await playPromise;
        status = 'playing';
      }

      scheduleUrlRefresh();

    } catch (err: any) {
       if (err.name === 'NotAllowedError' || err.name === 'AbortError') {
        // The browser blocked autoplay because the click came from another component.
        // We just pause it so the user can click the physical play button.
        // The browser blocked autoplay. Explicitly pause and show the play button.
        if (audioElement) {
          audioElement.pause();
          audioElement.currentTime = 0;
        }
        status = 'paused';
        errorMessage = ''; 
      } else {
        status = 'error';
        errorMessage = err.message;
      }
    }
  }

  const MIN_SPEED = 0.5;
  const MAX_SPEED = 3.0;

  function incrementSpeed() {
    if (!currentTrack) return;
    let currentRate = currentTrack.playbackRate || 1.0;
    // Add 0.1 and round to 1 decimal place to avoid floating point bugs
    let newRate = Math.round((currentRate + 0.1) * 10) / 10;
    if (newRate > MAX_SPEED) newRate = MAX_SPEED;
    setPlaybackRate(newRate);
  }

  function decrementSpeed() {
    if (!currentTrack) return;
    let currentRate = currentTrack.playbackRate || 1.0;
    let newRate = Math.round((currentRate - 0.1) * 10) / 10;
    if (newRate < MIN_SPEED) newRate = MIN_SPEED;
    setPlaybackRate(newRate);
  }

 function setPlaybackRate(rate: number) {
    //playbackRate = rate;
    if (currentTrack) {
      // Find the track in the array
      const idx = tracks.findIndex((t) => t.filename === currentTrack.filename);
      
      if (idx !== -1) {
        // Create a brand new object with the updated rate (Svelte 5 loves this)
        const updatedTrack = { ...tracks[idx], playbackRate: rate };
        
        // Replace the old track in the array
        tracks[idx] = updatedTrack;
        
        // Update the currentTrack reference to point to this new object
        currentTrack = updatedTrack;
      }
    }
    if (audioElement) {
      audioElement.playbackRate = rate;
    }
  }

  function removeFromQueue(filename: string) {
    const wasCurrentlyPlaying = currentTrack?.filename === filename;
    
    // Filter out the track
    tracks = tracks.filter((t) => t.filename !== filename);

    if (wasCurrentlyPlaying) {
      if (tracks.length === 0) {
        // Queue is empty, reset player
        currentTrack = null;
        status = 'idle';
        if (audioElement) {
          audioElement.pause();
          audioElement.src = '';
        }
      } else {
        // Play the next track in the queue, or stop if we deleted the last one
        playNext();
      }
    }
  }
  
  async function togglePlayPause() {
    if (!audioElement || !currentTrack) return;

    if (status === 'idle') {
      if (tracks.length > 0) {
        await playTrack(tracks[0]);
      }
      return;
    }

    if (status === 'error') {
      if (currentTrack) {
        await playTrack(currentTrack);
      }
      return;
    }

    if (audioElement.paused) {
      try {
        await audioElement.play();
      } catch (err: any) {
        if (err.name === 'NotAllowedError') {
          errorMessage = 'Autoplay blocked — click play to start';
          status = 'paused';
        }
      }
    } else {
      audioElement.pause();
    }
  }

  function seekTo(position: number) {
    if (!audioElement || !duration) return;
    audioElement.currentTime = (position / 100) * duration;
  }

  function setVolume(val: number) {
    volume = Math.max(0, Math.min(1, val));
    if (audioElement) {
      audioElement.volume = volume;
    }
  }

  function toggleMute() {
    isMuted = !isMuted;
    if (audioElement) {
      audioElement.muted = isMuted;
    }
  }

  function playNext() {
    if (!currentTrack || tracks.length === 0) return;
    const idx = tracks.findIndex((t) => t.filename === currentTrack.filename);
    
    // Only play next if we aren't at the end of the queue
    if (idx < tracks.length - 1) {
      const nextIdx = idx + 1;
      playTrack(tracks[nextIdx]);
    } else {
      // Optional: We've reached the end of the queue. 
      // You can either stop, or loop back to start. Let's loop back:
      playTrack(tracks[0]); 
    }
  }

  function playPrev() {
    if (!currentTrack || tracks.length === 0) return;
    const idx = tracks.findIndex((t) => t.filename === currentTrack.filename);
    
    // Only play prev if we aren't at the beginning
    if (idx > 0) {
      const prevIdx = idx - 1;
      playTrack(tracks[prevIdx]);
    } else {
      // At the beginning, just restart the current song or stay put
      if (audioElement) audioElement.currentTime = 0;
    }
  }

  async function downloadTrack(track: Track) {
    try {
      const ticket = await fetchPresignedUrl(track.filename);
      const a = document.createElement('a');
      a.href = ticket.url;
      a.download = track.metadata?.title ?? track.filename;
      a.target = '_blank';
      a.rel = 'noopener';
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
    } catch (err: any) {
      errorMessage = `Download failed: ${err.message}`;
    }
  }

  // ──────────────────────────────────────────────
  // Helpers
  // ──────────────────────────────────────────────
  function formatTime(seconds: number): string {
    if (!isFinite(seconds) || seconds < 0) return '0:00';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
  }

  function statusIcon(): string {
    switch (status) {
      case 'loading':
      case 'buffering':
        return '⏳';
      case 'playing':
        return '▶';
      case 'paused':
        return '⏸';
      case 'error':
        return '⚠';
      default:
        return '⏹';
    }
  }

  // ──────────────────────────────────────────────
  // Lifecycle & Event Listeners
  // ──────────────────────────────────────────────
  onMount(() => {

    // Listen for individual play requests from CatalogViewer island
    const handlePlay = (e: Event) => {
      console.log('Received play-track event with detail:', (e as CustomEvent).detail);
      playTrack((e as CustomEvent).detail);
    };

    window.addEventListener('play-track', handlePlay);

    return () => {
      window.removeEventListener('play-track', handlePlay);
      
      // Cleanup audio on component unmount
      clearRefreshTimer();
      if (retryTimer !== null) clearTimeout(retryTimer);
      if (audioElement) {
        audioElement.pause();
        audioElement.src = '';
      }
    };
  });

  $effect(() => {
    if (audioElement) {
      audioElement.volume = volume;
      audioElement.muted = isMuted;
      audioElement.playbackRate = playbackRate;
    }
  });

  $effect(() => {
    if (autoplay && tracks.length > 0 && status === 'idle' && !currentTrack) {
      playTrack(tracks[0]);
    }
  });
</script>

<!-- ────────────────────────────────────────────── -->
<!-- Template -->
<!-- ────────────────────────────────────────────── -->

{#if children}
  {@const controls = {
    playTrack,
    togglePlayPause,
    playNext,
    playPrev,
    seekTo,
    setVolume,
    toggleMute,
    downloadTrack,
  }}
  {@const state = {
    currentTrack,
    status,
    currentTime,
    duration,
    volume,
    isMuted,
    progress,
    buffered,
    errorMessage,
    statusLabel,
    displayTitle,
    displayArtist,
  }}
  {@const admin = isAdmin}
  {@const trackList = tracks}

  {@render children({ controls, state, admin, trackList })}
{:else}
  <!-- Default built-in UI — all Tailwind -->
  <div
    class="
      bg-[#0f0f0f] border border-[#222] rounded-xl p-5
      max-w-[480px] font-sans text-neutral-200
      {status === 'error' ? '!border-[#c0392b]' : ''}
    "
  >
    <!-- Hidden audio engine -->
    <audio
      bind:this={audioElement}
      crossorigin="anonymous"
      preload="auto"
    ></audio>

    <!-- ── Track Info ── -->
    <div class="flex items-center gap-3 mb-4">
      <span class="text-xl shrink-0">{statusIcon()}</span>
      <div class="flex flex-col min-w-0">
        <!-- NEW: Wrapper for Title + Speed Badge -->
        <div class="flex items-center gap-2">
          <span class="text-base font-semibold truncate">{displayTitle}</span>
          {#if currentTrack?.playbackRate && currentTrack.playbackRate !== 1.0}
            <span class="text-[9px] bg-[#1db954]/20 text-[#1db954] px-1.5 py-0.5 rounded flex-shrink-0">
              {currentTrack.playbackRate}x
            </span>
          {/if}
        </div>
        <!-- END NEW -->
        <span class="text-sm text-[#888] truncate">{displayArtist}</span>
      </div>
    </div>

    <!-- ── Progress / Seek Bar ── -->
    <div class="flex items-center gap-2 mb-4">
      <span class="text-xs text-[#888] min-w-[36px] text-center tabular-nums">
        {formatTime(displayTime)}
      </span>

      <!-- Seek rail -->
      <div
        bind:this={seekBarElement}
        class="seek-rail flex-1 h-1.5 bg-[#333] rounded-full relative cursor-pointer"
        role="slider"
        aria-label="Seek"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(progress)}
        tabindex="0"
        
        onpointerdown={handleSeekPointerDown}
        onpointermove={handleSeekPointerMove}
        onpointerup={handleSeekPointerUp}
        onpointercancel={handleSeekPointerUp}
        
        onkeydown={(e) => {
          if (e.key === 'ArrowRight') seekTo(Math.min(progress + 5, 100));
          if (e.key === 'ArrowLeft') seekTo(Math.max(progress - 5, 0));
        }}
      >
        <!-- Buffered -->
        <div
          class="absolute top-0 left-0 h-full bg-[#444] rounded-full transition-[width] duration-300"
          style="width: {duration > 0 ? (buffered / duration) * 100 : 0}%"
        ></div>
        <!-- Played -->
        <div
          class="absolute top-0 left-0 h-full bg-[#1db954] rounded-full transition-[width] {isDragging ? 'duration-0' : 'duration-100'}"
          style="width: {progress}%"
        ></div>
        <!-- Thumb (Grows slightly when dragging) -->
        <div
          class="
            absolute top-1/2 bg-white rounded-full -translate-x-1/2 -translate-y-1/2 shadow 
            transition-[left,width,height] {isDragging ? 'duration-0' : 'duration-100'}
            {isDragging ? 'w-4 h-4' : 'w-3 h-3'}
          "
          style="left: {progress}%"
        ></div>
      </div>

      <!-- Duration time on the right -->
      <span class="text-xs text-[#888] min-w-[36px] text-center tabular-nums">
        {formatTime(duration)}
      </span>
    </div>

    <!-- ── Main Controls ── -->
        <!-- ── Main Controls ── -->
    <div class="grid grid-cols-3 items-center mb-3">
      
      <!-- LEFT COLUMN: Speed Control Entity -->
      <div class="flex items-center justify-start">
        <div class="flex items-center gap-1 bg-[#1a1a1a] border border-[#333] rounded-lg px-1.5 py-1 select-none">
          <button 
            class="bg-transparent border-none text-white cursor-pointer text-sm w-6 h-6 flex items-center justify-center rounded hover:bg-white/10 transition disabled:opacity-30 disabled:cursor-default"
            onclick={decrementSpeed}
            disabled={(currentTrack?.playbackRate || 1.0) <= MIN_SPEED}
            aria-label="Decrease speed"
          >
            −
          </button>
          <div class="flex items-center gap-1 text-white px-1">
            <span class="text-sm">🐇</span>
            <span class="text-[11px] font-semibold min-w-[30px] text-center tabular-nums">
              {(currentTrack?.playbackRate || 1.0).toFixed(1)}x
            </span>
          </div>
          <button 
            class="bg-transparent border-none text-white cursor-pointer text-sm w-6 h-6 flex items-center justify-center rounded hover:bg-white/10 transition disabled:opacity-30 disabled:cursor-default"
            onclick={incrementSpeed}
            disabled={(currentTrack?.playbackRate || 1.0) >= MAX_SPEED}
            aria-label="Increase speed"
          >
            +
          </button>
        </div>
      </div>

      <!-- CENTER COLUMN: Playback Controls -->
      <div class="flex items-center justify-center gap-4">
        <!-- Previous -->
        <button
          class="
            bg-transparent border-none text-neutral-200 cursor-pointer
            text-xl p-1.5 rounded-full transition
            hover:bg-white/10 active:scale-[0.92]
            flex items-center justify-center w-10 h-10
            disabled:opacity-30 disabled:cursor-default
          "
          onclick={playPrev}
          aria-label="Previous track"
          disabled={tracks.length === 0}
        >
          ⏮
        </button>

        <!-- Play / Pause -->
        <button
          class="
            border-none cursor-pointer
            text-3xl p-2 rounded-full transition
            hover:active:scale-[0.92]
            flex items-center justify-center
            w-14 h-14
            bg-[#1db954] text-black
            hover:bg-[#1ed760]
            disabled:opacity-30 disabled:cursor-default
            shadow-lg shadow-[#1db954]/25
          "
          onclick={togglePlayPause}
          aria-label={status === 'playing' ? 'Pause' : 'Play'}
          disabled={tracks.length === 0 && status === 'idle'}
        >
          {#if status === 'loading' || status === 'buffering'}
            <span class="inline-block w-6 h-6 border-2 border-transparent border-t-current rounded-full animate-spin"></span>
          {:else if status === 'playing'}
            ⏸
          {:else}
            ▶
          {/if}
        </button>

        <!-- Next -->
        <button
          class="
            bg-transparent border-none text-neutral-200 cursor-pointer
            text-xl p-1.5 rounded-full transition
            hover:bg-white/10 active:scale-[0.92]
            flex items-center justify-center w-10 h-10
            disabled:opacity-30 disabled:cursor-default
          "
          onclick={playNext}
          aria-label="Next track"
          disabled={tracks.length === 0}
        >
          ⏭
        </button>

        <!-- Delete from Queue -->
        {#if status === 'playing' || status === 'paused'}
          <button
            class="bg-red-500/80 hover:bg-red-500 border-none text-white cursor-pointer text-xs p-1.5 rounded-full transition flex items-center justify-center w-8 h-8"
            onclick={(e) => { e.stopPropagation(); removeFromQueue(currentTrack.filename); }}
            aria-label="Remove from queue"
          >
            ✕
          </button>
        {/if}
      </div>

      <!-- RIGHT COLUMN: Volume -->
      <div class="flex items-center justify-end gap-1">
        <button
          class="
            bg-transparent border-none text-neutral-200 cursor-pointer
            text-xl p-1.5 rounded-full transition
            hover:bg-white/10
            flex items-center justify-center w-10 h-10
          "
          onclick={toggleMute}
          aria-label={isMuted ? 'Unmute' : 'Mute'}
        >
          {isMuted ? '🔇' : volume > 0.5 ? '🔊' : '🔉'}
        </button>
        <input
          type="range"
          min="0"
          max="1"
          step="0.01"
          bind:value={volume}
          oninput={(e) => setVolume(parseFloat(e.currentTarget.value))}
          aria-label="Volume"
          class="w-[70px] accent-[#1db954]"
        />
      </div>

    </div>

    <!-- ── Error Banner ── -->
    {#if status === 'error'}
      <div class="flex items-center justify-between bg-[#c0392b]/15 border border-[#c0392b] rounded-md px-3 py-2 text-sm mb-3">
        <span>{errorMessage}</span>
        <button
          class="bg-[#c0392b] border-none text-white px-2.5 py-1 rounded cursor-pointer text-xs hover:bg-[#e74c3c] transition"
          onclick={() => currentTrack && playTrack(currentTrack)}
        >
          Retry
        </button>
      </div>
    {:else if status === 'buffering'}
      <div class="bg-white/5 rounded px-3 py-1.5 text-sm text-[#888] text-center mb-3">
        Buffering…
      </div>
    {/if}

    <!-- ── Admin Section ── -->
    {#if isAdmin}
      <div class="flex items-center gap-2.5 border-t border-[#222] pt-3 mb-3">
        <span class="bg-[#f39c12] text-black text-[0.65rem] font-bold uppercase px-1.5 py-0.5 rounded tracking-wide">
          Admin
        </span>
        {#if currentTrack}
          <button
            class="bg-white/8 border border-[#333] text-[#ccc] px-2.5 py-1 rounded text-xs cursor-pointer hover:bg-white/15 transition"
            onclick={() => downloadTrack(currentTrack)}
          >
            ⬇ Download
          </button>
          <a
            class="bg-white/8 border border-[#333] text-[#ccc] px-2.5 py-1 rounded text-xs cursor-pointer hover:bg-white/15 transition no-underline"
            href="/admin/edit?id={currentTrack.id}"
            target="_blank"
            rel="noopener"
          >
            ✏ Edit Metadata
          </a>
        {/if}
      </div>
    {/if}

    <!-- ── Track List ── -->
    {#if showTracklist}
      {#if tracks.length > 0}
        <div class="tracklist max-h-60 overflow-y-auto border-t border-[#222] pt-2">
          {#each tracks as track, i (track.id)}
            <button
              class="
                flex items-center gap-2.5 w-full
                bg-transparent border-none
                px-1.5 py-2 rounded-md cursor-pointer
                text-sm text-left transition
                hover:bg-white/5
                {currentTrack?.id === track.id
                  ? 'text-[#1db954] bg-[#1db954]/8'
                  : 'text-[#bbb]'
                }
              "
              onclick={() => playTrack(track)}
            >
              <span
                class="
                  text-xs min-w-[20px] text-right
                  {currentTrack?.id === track.id ? 'text-[#1db954]' : 'text-[#666]'}
                "
              >
                {i + 1}
              </span>
               <span class="flex-1 truncate flex items-center gap-2">
                {track.metadata?.title ?? track.filename}
                 <!-- Show speed badge if not 1.0 (Use 'track' not 'currentTrack' here!) -->
                {#if track.playbackRate && track.playbackRate !== 1.0}
                   <span class="text-[9px] bg-[#1db954]/20 text-[#1db954] px-1.5 py-0.5 rounded flex-shrink-0">
                    {track.playbackRate}x
                  </span>
                {/if}
              </span>
              <span class="text-[#666] text-xs min-w-[60px] text-right">
                {track.metadata?.artist ?? ''}
              </span>
            </button>

            {#if isAdmin}
              <span class="flex gap-1">
                <button
                  class="bg-transparent border-none text-[#888] cursor-pointer text-sm p-0.5 rounded hover:text-white transition"
                  onclick={(e) => { e.stopPropagation(); downloadTrack(track); }}
                >
                  ⬇
                </button>
              </span>
            {/if}

          {/each}
        </div>
      {:else}
        <div class="text-center text-[#555] text-sm py-6">
          No tracks available
        </div>
      {/if}
    {/if}
  </div>
{/if}

<!-- ────────────────────────────────────────────── -->
<!-- Minimal styles — scrollbar only -->
<!-- ────────────────────────────────────────────── -->
<style>
  .tracklist::-webkit-scrollbar {
    width: 4px;
  }
  .tracklist::-webkit-scrollbar-track {
    background: transparent;
  }
  .tracklist::-webkit-scrollbar-thumb {
    background: #333;
    border-radius: 2px;
  }
</style>
