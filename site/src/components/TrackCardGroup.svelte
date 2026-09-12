<!-- src/components/TrackCardGroup.svelte -->
<script lang="ts">
  import TrackCard from './TrackCard.svelte';

  interface Props {
    items: any[];                                     // array of catalog items
    children?: import('svelte').Snippet<[any]>;        // optional actions snippet
    apiBase: string;
  }

  let { items, children, apiBase }: Props = $props();

  // Filter out null entries so we don't blow up on `item.filename`
  const safeItems = $derived(items.filter((item): item is any => item != null));

  // Single-expanded state: only one card can be open at a time
  let expandedFilename: string | null = $state(null);

  function handleToggle(filename: string) {
    expandedFilename = expandedFilename === filename ? null : filename;
  }
</script>

<div class="space-y-2">
  {#each safeItems as item (item.filename)}
    <TrackCard
      {item}
      expanded={expandedFilename === item.filename}
      ontoggle={handleToggle}
      {apiBase}
    >
      {#if children}
        {@render children(item)}
      {/if}
    </TrackCard>
  {/each}
</div>
