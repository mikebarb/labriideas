<!-- src/components/AppShell.svelte -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { isPlaylistOpen, trackList } from '../lib/playerStore.js';
  import QueueDrawer from './QueueDrawer.svelte';

  interface Props {
    children: Snippet;
  }
  let { children }: Props = $props();
  
  // The player takes 96px (h-24) when showing, 0px when hidden
  let mainPaddingBottom = $derived($trackList.length > 0 ? 'pb-24' : 'pb-0');
</script>

<div class="flex h-full w-full overflow-hidden">
  
  <!-- Main Content: Reactive margin and padding -->
  <main 
    class="flex-1 overflow-auto transition-all duration-300 {mainPaddingBottom}"
  >
    {@render children()}
  </main>

  <!-- Queue Sidebar: Reactive visibility -->
  <div 
    class="transition-all duration-300 overflow-hidden hidden md:block"
    class:!w-80={$isPlaylistOpen}
    class:w-0={!$isPlaylistOpen}
  >
    <QueueDrawer />
  </div>
</div>
