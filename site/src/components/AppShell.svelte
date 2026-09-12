<!-- src/components/AppShell.svelte -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { mobileView, desktopQueueOpen } from '../lib/playerStore.js';
  import { onMount } from 'svelte';
  import { refreshAuth } from '../lib/appStatusStore'; 
  import { authClient } from '../lib/authClient';
  import QueueDrawer from './QueueDrawer.svelte';
  import MetadataEditor from './MetadataEditor.svelte';
  import { getCachedCatalog } from '../lib/catalogStore.js';

  interface Props {
    children: Snippet;
    apiBase: string;
  }
  let { children, apiBase }: Props = $props();
  
  // CHANGED: Removed mainPaddingBottom (pb-24) workaround.
  // The player is now an in-flow flex sibling (LabriLayout), so AppShell's
  // available height already ends at the player's top edge — padding the
  // scrollable content would create a phantom 96px of blank space instead
  // of clearing a fixed overlay.

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

  // Called to wake up the sleeping server and preload the catalog
  let isSyncing = $state(false);
  async function performInitialHandshake(): Promise<void> {
    isSyncing = true; // Show a subtle "Syncing..." icon
    try {
      await getCachedCatalog();
    } finally {
      isSyncing = false; // Hide it
    }
  }

  // Listen for 'edit-track' events from anywhere in the app.
  // CatalogViewer, TrackCard, etc. can all dispatch this and the
  // editor opens here.
  onMount(() => {
    // wake up the go server and preload catalog
    performInitialHandshake();

    // edit-track handling.
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

<!--
  CHANGED: root container is now `relative` so the mobile queue overlay
  can be positioned `absolute inset-0` — covering the app area only,
  ending exactly at the in-flow player's top edge.
-->
<div class="relative flex h-full w-full overflow-hidden">
  
  <!-- Main Content -->
  <main 
    class="flex-1 overflow-auto transition-all duration-300"
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
    MOBILE: Overlay (< md breakpoint)
    Only visible when mobileView === 'list'.
    CHANGED: `absolute inset-0` (was `fixed inset-0 z-40`) — the overlay
    now fills AppShell's area only and ends at the player's top edge.
    The in-flow player bar below remains visible by geometry, no longer
    relying on z-index stacking (player z-50 > drawer z-40) which broke
    when the player moved into the document flow.
  -->
  {#if $mobileView === 'list'}
    <div class="md:hidden absolute inset-0 z-40 bg-[#0e0e0e] flex flex-col">
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
