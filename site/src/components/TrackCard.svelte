<!-- src/components/TrackCard.svelte -->
<script lang="ts">
  import { sanitizeKeywords } from '../lib/dataUtils.js';
  import { Download, Pencil, Play, Pause, Loader2, Plus } from 'lucide-svelte';

  // Actions + download spinner come from the shared composable.
  import { useTrackActions } from '../lib/useTrackActions.svelte.js';
  // Global transition flag (the "switch lock")
  import { isTrackSwitching } from '../lib/transition.svelte.js';
  import { currentTrackStore, statusStore, trackList } from '../lib/playerStore.js';

  interface Props {
    item: any;
    expanded?: boolean;
    ontoggle?: (filename: string) => void;
    children?: import('svelte').Snippet<[any]>;
    apiBase?: string;
    isAdmin?: boolean;
  }

  let { item, expanded = false, ontoggle, children, apiBase = '', isAdmin = false }: Props = $props();

  const keywords = $derived(sanitizeKeywords(item.keywords ?? []));
  const categoryText = $derived(formatCategory(item.category));

  // Visual state (native $store auto-subscription — reliable here)
  const isPlaying = $derived($currentTrackStore?.filename === item.filename && $statusStore === 'playing');
  const isCurrent = $derived($currentTrackStore?.filename === item.filename);
  const isPlayerLoading = $derived($statusStore === 'loading' || $statusStore === 'buffering');
  const queued = $derived(!!$trackList.find(t => t.filename === item.filename));

  // ─── Loading state: the bulletproof switch-lock logic ───
  // Local pending: immediate spinner on THIS card only, set on click.
  let isPending = $state(false);

  // Store-based loading is only trusted when NO transition is in flight.
  // During a switch, currentTrackStore still holds the OLD track, so its
  // signal is ambiguous — only the clicked card (isPending) may spin.
  const isLoading = $derived(
    isPending || (!isTrackSwitching() && isCurrent && isPlayerLoading)
  );

  // When the transition completes, the store state is authoritative again.
  $effect(() => {
    if (!isTrackSwitching() && isPending) isPending = false;
  });

  // ─── Actions + download spinner from the shared composable ───
  const { isDownloading, handlePlay, handleQueue, handleDownload } =
    useTrackActions(() => item, () => apiBase);

  function handlePlayLocal(event: MouseEvent) {
    isPending = true; // Immediate feedback on THIS card
    handlePlay(event); // Composable raises the switch lock + dispatches play
  }

  // TrackCard stays decoupled from the editor; the parent decides
  // how the editor is mounted.
  function handleEdit(event: MouseEvent) {
    event.stopPropagation();
    window.dispatchEvent(new CustomEvent('edit-track', { detail: { track: item } }));
  }

  function formatCategory(category: string | string[] | undefined): string {
    if (!category) return '';
    if (Array.isArray(category)) return category.join(', ');
    return category;
  }
</script>

<div class="border rounded bg-white transition-all hover:border-gray-300 {expanded ? 'bg-gray-50' : ''}">
  <!-- Header: arrow on left, title/speaker in middle, play button on right -->
  <div class="w-full p-4 flex items-center gap-3 min-h-11">
    
    <!-- Expand/collapse arrow (left) -->
    <button
      class="text-gray-400 p-1 rounded hover:bg-gray-200 transition shrink-0"
    onclick={() => ontoggle?.(item.filename)}>
      {expanded ? '▼' : '▶'}
    </button>

    <button class="flex-1 min-w-0 text-left" onclick={() => ontoggle?.(item.filename)}>
      <p class="font-medium text-gray-900 truncate">{item.title ?? item.filename}</p>
      <p class="text-sm text-gray-500 truncate">{item.speaker}</p>
    </button>

    <!-- NEWPLAY BUTTON -->
    <button
      onclick={handlePlayLocal}
      disabled={isLoading}
      class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-lg font-bold transition shrink-0
             disabled:opacity-60 disabled:cursor-not-allowed flex items-center gap-2"
      aria-label={isPlaying ? 'Pause track' : 'Play track'}
    >
      {#if isLoading}
        <Loader2 size={18} class="animate-spin" /> Loading
      {:else if isPlaying}
        <Pause size={18} fill="currentColor" /> Pause
      {:else}
        <Play size={18} fill="currentColor" /> Play
      {/if}
    </button>
  </div>

  <!-- Expanded content: keywords/category on left, snippet buttons on right -->
  {#if expanded}
    <div class="px-4 pb-4 border-t border-gray-200 pt-4 ml-7">
      <div class="flex items-start justify-between gap-4">      
        <!-- Keywords and category (left) -->
        <div class="flex flex-col gap-2 flex-1 min-w-0">
          {#if categoryText}
            <span class="inline-block text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded">
              {categoryText}
            </span>
          {/if}
          {#if keywords.length > 0}
            <div class="flex flex-wrap gap-1">
              {#each keywords as kw (kw)}
                <span class="text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded border border-gray-200">
                  #{kw}
                </span>
              {/each}
            </div>
          {/if}
        </div>
      
        <!-- Action buttons (right-aligned) -->
        <div class="flex items-center gap-2 shrink-0">
          <!-- QUEUE BUTTON: Grayed out if already queued
            QUEUE BUTTON (NEW): Adds the track to the queue drawer and
            triggers a background download to OPFS. Does NOT auto-play.
            Grayed out with a checkmark when the track is already queued.
          -->
          <button
            onclick={handleQueue}
            disabled={queued}
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

          {#if isAdmin}
            <button
              onclick={handleEdit}
              class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-2 rounded-lg transition"
              aria-label="Edit track"
            >
              <Pencil size={16} />
            </button>
            
            <!--
            CHANGED: Built-in download button for admin users.
            Renders a spinner while downloading, otherwise shows the
            Download icon. Disabled state prevents double-clicks.
            -->
            <button
              onclick={handleDownload}
              disabled={isDownloading}
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
          <!-- Optional extra buttons (children snippet) -->
          {#if children}
            {@render children(item)}
          {/if}
        </div>
      </div>
    </div>     
  {/if}
</div>
