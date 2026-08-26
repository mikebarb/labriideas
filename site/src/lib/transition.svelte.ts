// src/lib/transition.svelte.ts
//
// NEW FILE: Shared track-transition flag ("the switch lock").
//
// Why this exists:
// When Play is clicked on a new track, the status store flips to
// 'loading' IMMEDIATELY, but currentTrackStore still holds the OLD
// track until loadTrack() completes. In that window, store-derived
// "isLoading" logic is ambiguous — the OLD card's check
// (isCurrent && isPlayerLoading) is true, so both the old and the
// new card can show a spinner.
//
// This module-level flag removes the ambiguity:
// - The card's click handler raises it (a transition is starting).
// - Player.svelte clears it when the transition actually finishes
//   (load complete, toggle handled, or load failed).
// - While it is raised, cards suppress store-based spinner logic and
//   rely solely on their local isPending state — so ONLY the clicked
//   card can display a spinner during the switch.
//
// NOTE: Must be a .svelte.ts file — $state runes are not available in
// plain .ts modules. Exposed via getter/setter because module-level
// $state cannot be reassigned from importing modules.

let isSwitching = $state(false);

/** True while a track transition is in flight (click → load complete). */
export function isTrackSwitching(): boolean {
  return isSwitching;
}

/** Raised by the UI on click; cleared by Player.svelte on completion. */
export function setTrackSwitching(val: boolean): void {
  isSwitching = val;
}
