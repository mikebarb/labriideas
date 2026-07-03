<!-- src/components/AppShell.svelte -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { mobileView, desktopQueueOpen, trackList } from '../lib/playerStore.js';
  import { onMount } from 'svelte';
  import QueueDrawer from './QueueDrawer.svelte';
  import MetadataEditor from './MetadataEditor.svelte';

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
  // Global state for the metadata editor.
  // When set, MetadataEditor mounts and shows the editor for that track.
  let editingTrack = $state(null);

  function closeEditor() {
    editingTrack = null;
  }

  // Listen for 'edit-track' events from anywhere in the app.
  // CatalogViewer, TrackCard, etc. can all dispatch this and the
  // editor opens here.
  onMount(() => {
    const handleEditTrack = (event: Event) => {
      const customEvent = event as CustomEvent<{ track: any }>;
      editingTrack = customEvent.detail.track;
    };
    window.addEventListener('edit-track', handleEditTrack);
    return () => {
      window.removeEventListener('edit-track', handleEditTrack);
    };
  });
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

<!--
  CHANGED: Global MetadataEditor mount.
  Renders as a modal/overlay when any component dispatches 'edit-track'.
  The editor itself handles its own styling (slate-800 panel).
-->
{#if editingTrack}
   <!-- 
    CHANGED: Fixed-position overlay so the editor floats above
    all other content (player, queue, page content).
  -->
  <div class="fixed inset-0 z-100 flex items-center justify-center p-4 bg-black/60">
    <div class="max-w-2xl w-full max-h-[90vh] overflow-y-auto">
      <div class="max-w-2xl w-full max-h-[80vh] flex flex-col">
        <MetadataEditor 
          track={editingTrack} 
          onClose={closeEditor} 
          apiBase={apiBase}
        />
        </div>
    </div>
  </div>
{/if}
