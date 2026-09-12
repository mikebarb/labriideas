<!-- src/components/TopicFeaturedCard.svelte -->
<!--
  NEW FILE: Compact featured-lecture card for the MegaMenu panel.

  Sibling of FeaturedCard (same useTrackActions wiring, same visual
  states) with three deliberate differences:
  1. COMPACT: row layout (title / speaker / action row) sized for the
     menu dropdown — no image background, no expansion.
  2. INERT BODY: only the explicit buttons act. In a hover-driven menu,
     click-to-play bodies cause misfires; buttons are predictable.
  3. HYDRATION CONTRACT from MegaMenu's derived overlay:
       - item.pendingHydration === true  → catalog not loaded yet:
         render title/speaker immediately, buttons disabled (Option A)
       - item.unavailable === true       → filename confirmed missing
         from catalog (file does not exist): "Lecture not available.",
         all buttons disabled
       - otherwise                       → fully functional
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

  // ─── Hydration contract (set by MegaMenu's derived overlay) ───
  const isUnavailable = $derived(item.unavailable === true);
  const isHydrating = $derived(item.pendingHydration === true);
  // All three action buttons are disabled until the catalog lookup
  // confirms the track — prevents queueing/downloading a file that
  // can't be resolved.
  const actionsDisabled = $derived(isUnavailable || isHydrating);

  // Visual state (native $store auto-subscription) — identical to FeaturedCard
  const isPlaying = $derived($currentTrackStore?.filename === item.filename && $statusStore === 'playing');
  const isCurrent = $derived($currentTrackStore?.filename === item.filename);
  const isPlayerLoading = $derived($statusStore === 'loading' || $statusStore === 'buffering');
  const queued = $derived(!!$trackList.find(t => t.filename === item.filename));

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

<div class="border-b border-gray-100 pb-3 last:border-0">
  <div class="flex items-center justify-between gap-3">
    <!-- Text block: INERT (no click handler — menu context, buttons only) -->
    <div class="min-w-0">
      <span class="block text-sm font-bold text-gray-900 truncate">{item.title}</span>
      <span class="block text-xs text-gray-500 mt-0.5 truncate">{item.speaker}</span>
    </div>

    <!-- Action row (compact, consistent with FeaturedCard states) -->
    <div class="flex items-center gap-1 shrink-0">
      <!-- PLAY: tri-state (loading spinner / pause / play) -->
      <button
        onclick={handlePlayLocal}
        disabled={actionsDisabled || isLoading}
        class="bg-green-600 hover:bg-green-700 text-white p-1.5 rounded-lg transition
               disabled:opacity-60 disabled:cursor-not-allowed"
        aria-label={isPlaying ? 'Pause track' : 'Play track'}
      >
        {#if isUnavailable}
          <Play size={14} />
        {:else if isLoading}
          <Loader2 size={14} class="animate-spin" />
        {:else if isPlaying}
          <Pause size={14} fill="currentColor" />
        {:else}
          <Play size={14} fill="currentColor" />
        {/if}
      </button>

      <!-- QUEUE: tick + disabled when already queued -->
      <button
        onclick={handleQueue}
        disabled={actionsDisabled || queued}
        class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-1.5 rounded-lg transition
               disabled:opacity-50 disabled:cursor-not-allowed"
        aria-label={queued ? 'Already in queue' : 'Add to queue'}
      >
        {#if queued}
          <span class="text-xs">✓</span>
        {:else}
          <Plus size={14} />
        {/if}
      </button>

      <!-- DOWNLOAD: admin only, spinner while downloading -->
      {#if $isAdminStore}
        <button
          onclick={handleDownload}
          disabled={actionsDisabled || isDownloading}
          class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-1.5 rounded-lg transition
                 disabled:opacity-50 disabled:cursor-wait"
          aria-label="Download track"
        >
          {#if isDownloading}
            <div class="w-3.5 h-3.5 border-2 border-gray-500 border-t-transparent rounded-full animate-spin"></div>
          {:else}
            <Download size={14} />
          {/if}
        </button>
      {/if}
    </div>
  </div>

  <!-- Confirmed catalog miss: the file does not exist. Fail visibly
       here rather than mysteriously at the player (presigned-URL 404). -->
  {#if isUnavailable}
    <p class="text-xs text-gray-400 italic mt-1">Lecture not available.</p>
  {/if}
</div>
