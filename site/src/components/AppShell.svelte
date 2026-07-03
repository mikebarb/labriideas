<!-- src/components/AppShell.svelte -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { mobileView, desktopQueueOpen, trackList } from '../lib/playerStore.js';
  import QueueDrawer from './QueueDrawer.svelte';

  interface Props {
    children: Snippet;
    apiBase: string;
  }
  let { children, apiBase }: Props = $props();
  
  // The player takes 96px (h-24) when showing, 0px when hidden
  let mainPaddingBottom = $derived($trackList.length > 0 ? 'pb-24' : 'pb-0');

  // CHANGED: Close drawer handler for mobile overlay
  function closeMobileDrawer() {
    mobileView.set('min');
  }

</script>

<div class="flex h-full w-full overflow-hidden">
  
  <!-- Main Content: Reactive margin and padding -->
  <main 
    class="flex-1 overflow-auto transition-all duration-300 {mainPaddingBottom}"
  >
    {@render children()}
  </main>

  <!-- Queue Sidebar: Reactive visibility -->
    <!--
    DESKTOP: Inline sidebar (>= md breakpoint)
    Driven by desktopQueueOpen. Slides in/out with width transition.
  -->
  <div 
    class="transition-all duration-300 overflow-hidden hidden md:block"
    class:!w-80={$desktopQueueOpen}
    class:w-0={!$desktopQueueOpen}
  >
    <QueueDrawer {apiBase} />
  </div>

  <!--
    MOBILE: Full-screen overlay (< md breakpoint)
    Only visible when mobileView === 'list'.
    The persistent player bar at the bottom (z-50) remains visible above
    the drawer (z-40) so users always have the toggle buttons.
  -->
  {#if $mobileView === 'list'}
    <div class="md:hidden fixed inset-0 z-40 bg-[#0e0e0e] flex flex-col">
      <div class="flex items-center justify-end p-2 border-b border-neutral-800">
        <button
          onclick={closeMobileDrawer}
          class="text-neutral-400 hover:text-white p-2 rounded-full hover:bg-white/10"
          aria-label="Close queue"
        >
          ✕
        </button>
      </div>
      <div class="flex-1 overflow-hidden">
        <QueueDrawer {apiBase} />
      </div>
    </div>
  {/if}
</div>
