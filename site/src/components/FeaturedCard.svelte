<!-- src/components/FeaturedCard.svelte -->
<!--
  NEW FILE: Presentational component for a single featured lecture card.

  Unlike TrackCard (list row with expandable detail), this is an
  image-backed promotional card. It contains ZERO business logic:
  all action behavior and button state come from useTrackActions,
  the shared composable. A behavior fix there applies here automatically.

  The card body itself is a large play target (promotional UX); the
  explicit Play button in the action row behaves identically. When the
  card's track is currently playing, Player.svelte's playTrack CASE 1
  ("same track → toggle") turns any play click into a pause.
-->
<script lang="ts">
  import { Play, Pause, Loader2, Plus, Download } from 'lucide-svelte';
  import { useTrackActions } from '../lib/useTrackActions.svelte.js';
  import { isTrackSwitching } from '../lib/transition.svelte.js';
  import { currentTrackStore, statusStore, trackList } from '../lib/playerStore.js';
  import { isAdmin as isAdminStore } from '../lib/appStatusStore';

  interface Props {
    item: any;
    apiBase?: string;
  }

  let { item, apiBase = '' }: Props = $props();



  // Visual state (native $store auto-subscription)
  const isPlaying = $derived($currentTrackStore?.filename === item.filename && $statusStore === 'playing');
  const isCurrent = $derived($currentTrackStore?.filename === item.filename);
  const isPlayerLoading = $derived($statusStore === 'loading' || $statusStore === 'buffering');
  const queued = $derived(!!$trackList.find(t => t.filename === item.filename));
  // ─── Hydration contract (set by FeaturedGrid's derived overlay) ───
  // NEW: three-state support. Pre-hydration disables actions; a confirmed
  // catalog miss (file doesn't exist) shows "Lecture not available." and
  // disables everything. Fully-hydrated cards behave exactly as before.
  const isUnavailable = $derived(item.unavailable === true);
  const isHydrating = $derived(item.pendingHydration === true);
  const actionsDisabled = $derived(isUnavailable || isHydrating);

  // ─── Loading state: the bulletproof switch-lock logic ───
  let isPending = $state(false);
  const isLoading = $derived(
    isPending || (!isTrackSwitching() && isCurrent && isPlayerLoading)
  );
  $effect(() => {
    if (!isTrackSwitching() && isPending) isPending = false;
  });

  // ─── Actions + download spinner from the shared composable ───
  const { isDownloading, handlePlay, handleQueue, handleDownload } =
    useTrackActions(() => item, () => apiBase);

  function handlePlayLocal(event: MouseEvent) {
    isPending = true;
    handlePlay(event);
  }
</script>

<div 
  class="relative h-48 rounded-lg overflow-hidden bg-cover bg-center group cursor-pointer"
  style={`background-image: url(${item.image})`}
>
  <!-- Card body: clicking plays the track (promotional UX) -->
  <button 
    type="button"
    class="absolute inset-0 w-full h-full text-left border-0 p-0 bg-black/60 group-hover:bg-black/40 transition"
    onclick={handlePlayLocal}
    aria-label={`Play ${item.title ?? item.filename}`}
  >
    <div class="p-4 flex flex-col justify-end h-full pointer-events-none">
      <h3 class:list={[
        "text-gray-200 text-sm mt-1 bg-blue-500 w-fit px-1",
        item.titleBgColor
      ]}>{item.speaker}</h3>
      <h3 class:list={[
        "text-white text-xl font-bold bg-blue-500 w-fit px-1",
        item.titleBgColor
      ]}>{item.title}</h3>
      {#if isUnavailable}
        <p class="text-xs text-gray-300 italic mt-1">Lecture not available.</p>
      {/if}
    </div>
  </button>

  <!-- Action button row (bottom-right, consistent with TrackCard) -->
  <div class="absolute bottom-2 right-2 flex items-center gap-1 z-10">
    <!-- PLAY BUTTON: shows loading spinner while the track buffers,
         pause icon while this track is the active playing track -->
    <button
      onclick={handlePlayLocal}
      disabled={actionsDisabled || isLoading}
      class="bg-green-600 hover:bg-green-700 text-white p-2 rounded-lg transition
             disabled:opacity-60 disabled:cursor-not-allowed"
      aria-label={isPlaying ? 'Pause track' : 'Play track'}
    >
      {#if isLoading}
        <Loader2 size={16} class="animate-spin" />
      {:else if isPlaying}
        <Pause size={16} fill="currentColor" />
      {:else}
        <Play size={16} fill="currentColor" />
      {/if}
    </button>

    <!-- QUEUE BUTTON: Grayed out with a checkmark when the track is
         already queued. Adds to queue + background OPFS cache; does
         NOT auto-play. -->
    <button
      onclick={handleQueue}
      disabled={actionsDisabled || queued}
      class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-2 rounded-lg transition
             disabled:opacity-50 disabled:cursor-not-allowed"
      aria-label={queued ? 'Already in queue' : 'Add to queue'}
    >
      {#if queued}
        <span class="text-xs">✓</span>
      {:else}
        <Plus size={16} />
      {/if}
    </button>

    <!-- DOWNLOAD BUTTON: admin only. Spinner while downloading. -->
    {#if $isAdminStore}
      <button
        onclick={handleDownload}
        disabled={actionsDisabled || isDownloading}
        class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-2 rounded-lg transition
               disabled:opacity-50 disabled:cursor-wait"
        aria-label="Download track"
      >
        {#if isDownloading}
          <div class="w-4 h-4 border-2 border-gray-500 border-t-transparent rounded-full animate-spin"></div>
        {:else}
          <Download size={16} />
        {/if}
      </button>
    {/if}
  </div>
</div>
