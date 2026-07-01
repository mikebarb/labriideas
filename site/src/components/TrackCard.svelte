<!-- src/components/TrackCard.svelte -->
<script lang="ts">
  //import type { Track } from '../lib/types.ts';
  import { buildTrack } from '../lib/buildTrack.ts';
  import { sanitizeKeywords } from '../lib/dataUtils.js';
  import { downloadTrack } from '../lib/downloader.js';
  import { Download } from 'lucide-svelte';

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

  // Download state — tracks which filename is currently downloading
  // so we can show a spinner on the right card.
  let downloadingFilename: string | null = $state(null);

  function handleHeaderClick() {
    ontoggle?.(item.filename);
  }

  function handlePlay(event: MouseEvent) {
    event.stopPropagation();
    const track = buildTrack(item);
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }

  // CHANGED: Download handler — calls the library function with
  // the track, apiBase, and lifecycle callbacks.
  function handleDownload(event: MouseEvent) {
    event.stopPropagation();
    
    // Prevent double-clicking the same file
    if (downloadingFilename === item.filename) return;

    const track = buildTrack(item);
    downloadingFilename = track.filename;

    downloadTrack(track, apiBase, {
      onComplete: () => { downloadingFilename = null; },
      onError: (err) => {
        console.error('Download failed:', err);
        downloadingFilename = null;
      },
    });
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
      onclick={handleHeaderClick}
      aria-expanded={expanded}
      aria-label={expanded ? 'Collapse details' : 'Expand details'}
    >
      {expanded ? '▼' : '▶'}
    </button>

    <!-- Title and speaker (clickable to expand) -->
    <button
      class="flex-1 min-w-0 text-left"
      onclick={handleHeaderClick}
      aria-expanded={expanded}
    >
      <p class="font-medium text-gray-900 wrap-break-words">{item.title ?? item.filename}</p>
      <p class="text-sm text-gray-500 wrap-break-words">{item.speaker}</p>
    </button>

    <!-- Play button (right, always visible) -->
    <button
      onclick={handlePlay}
      class="bg-green-600 hover:bg-green-700 text-white px-4 py-2 rounded-lg font-bold transition shrink-0"
      aria-label="Play track"
    >
      ▶ Play
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
          <!--
            CHANGED: Built-in download button for admin users.
            Renders a spinner while downloading, otherwise shows the
            Download icon. Disabled state prevents double-clicks.
          -->
          {#if isAdmin}
            <button
              onclick={handleDownload}
              disabled={downloadingFilename === item.filename}
              class="bg-gray-100 hover:bg-gray-200 text-gray-700 p-2 rounded-lg transition
                      disabled:opacity-50 disabled:cursor-wait"
              aria-label="Download track"
            >
              {#if downloadingFilename === item.filename}
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
