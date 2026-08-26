// src/lib/useTrackActions.svelte.ts
//
// Shared composable for the three canonical track ACTIONS and the
// download spinner lifecycle.
//
// DESIGN NOTE (Svelte 5): Store-derived VISUAL state (isPlaying,
// isLoading, queued) intentionally lives in each COMPONENT, not here.
// Legacy writable stores only auto-subscribe reactively inside .svelte
// component files via the `$store` syntax. Reading them inside a
// .svelte.ts composable is not reliably tracked by $derived. The
// shared, reusable part is the ACTION logic; per-component visual
// derivation is a few lines each and always correct.

import { play, queue, download } from './playerController.js';
import { setTrackSwitching } from './transition.svelte.js';

export function useTrackActions(getItem: () => any, getApiBase: () => string) {
  // ─── Download spinner state (owned here — no store dependency) ───
  let isDownloading = $state(false);

  // ─── Action handlers ───
  // Each stops propagation so card-level click handlers don't also fire.

  function handlePlay(event: MouseEvent) {
    event.stopPropagation();
    // Raise the global switch lock: Player.svelte clears it when the
    // transition finishes. Cards use this to suppress the ambiguous
    // store-based loading state during the switch window.
    setTrackSwitching(true);
    // Safety net: if the Player never answers (e.g. island not mounted),
    // release the lock after 10s so store-based spinners aren't frozen.
    setTimeout(() => setTrackSwitching(false), 10000);
    play(getItem(), getApiBase());
  }

  function handleQueue(event: MouseEvent) {
    event.stopPropagation();
    // Logic: Player controller promotes track to queue
    queue(getItem());
  }

  function handleDownload(event: MouseEvent) {
    event.stopPropagation();
    if (isDownloading) return; // double-click guard
    isDownloading = true;
    download(getItem(), getApiBase(), {
      onComplete: () => { isDownloading = false; },
      onError: () => { isDownloading = false; },
    });
  }

  return {
    get isDownloading() { return isDownloading; },
    // Actions
    handlePlay,
    handleQueue,
    handleDownload,
  };
}
