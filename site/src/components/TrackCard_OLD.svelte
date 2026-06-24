<!-- src/components/TrackCard.svelte -->
<script lang="ts">
  import { buildTrack } from '../lib/buildTrack.js';
  import { sanitizeKeywords } from '../lib/dataUtils.js';

  interface Props {
    item: any;
    expanded?: boolean;
    ontoggle?: (filename: string) => void;
    children?: import('svelte').Snippet<[any]>;
  }

  let { item, expanded = false, ontoggle, children }: Props = $props();

  const keywords = $derived(sanitizeKeywords(item.keywords ?? []));
  const categoryText = $derived(formatCategory(item.category));

  function handleHeaderClick() {
    ontoggle?.(item.filename);
  }

  function handlePlay(event: MouseEvent) {
    event.stopPropagation();
    const track = buildTrack(item);
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }

  function formatCategory(category: string | string[] | undefined): string {
    if (!category) return '';
    if (Array.isArray(category)) return category.join(', ');
    return category;
  }
</script>

<div class="border rounded bg-white transition-all hover:border-gray-300 {expanded ? 'bg-gray-50' : ''}">
  <button
    class="w-full text-left p-4 flex justify-between items-center min-h-11"
    onclick={handleHeaderClick}
    aria-expanded={expanded}
  >
    <div class="truncate flex-1">
      <p class="font-medium text-gray-900">{item.title ?? item.filename}</p>
      <p class="text-sm text-gray-500">{item.speaker}</p>
    </div>
    <span class="text-gray-400 ml-2">{expanded ? '▼' : '▶'}</span>
  </button>

  {#if expanded}
    <div class="px-4 pb-4 border-t border-gray-200 pt-4">
      <div class="flex items-center justify-between gap-4">
        <div class="flex-1 min-w-0">
          {#if categoryText}
            <span class="inline-block text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded mr-2">
              {categoryText}
            </span>
          {/if}
          {#if keywords.length > 0}
            <div class="mt-2 flex flex-wrap gap-1">
              {#each keywords as kw (kw)}
                <span class="text-xs bg-gray-100 text-gray-600 px-2 py-0.5 rounded border border-gray-200">
                  #{kw}
                </span>
              {/each}
            </div>
          {/if}
        </div>

        <div class="flex items-center gap-2 shrink-0">
          {#if children}
            {@render children(item)}
          {/if}
          <button
            onclick={handlePlay}
            class="bg-green-600 hover:bg-green-700 text-white px-6 py-2 rounded-lg font-bold transition"
          >
            ▶ Play Track
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
