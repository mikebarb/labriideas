<!-- src/components/TrackCardGroup.svelte -->
<script lang="ts">
  import TrackCard from './TrackCard.svelte';

  interface Props {
    items: any[];                                     // array of catalog items
    children?: import('svelte').Snippet<[any]>;        // optional actions snippet
  }

  let { items, children }: Props = $props();

  // Single-expanded state: only one card can be open at a time
  let expandedFilename: string | null = $state(null);

  function handleToggle(filename: string) {
    expandedFilename = expandedFilename === filename ? null : filename;
  }
</script>

<div class="space-y-2">
  {#each items as item (item.filename)}
    <TrackCard
      {item}
      expanded={expandedFilename === item.filename}
      ontoggle={handleToggle}
    >
      {#if children}
        {@render children(item)}
      {/if}
    </TrackCard>
  {/each}
</div>
