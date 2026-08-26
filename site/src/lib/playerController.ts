// src/lib/playerController.ts
import { get } from 'svelte/store';
import { trackList, currentTrackStore, statusStore, currentTimeStore } from './playerStore';
import { buildTrack } from './buildTrack';
import { downloadTrack } from './downloader';

/**
 * Centralized logic for Player interactions.
 * Decouples the UI (TrackCards, etc.) from the Player implementation.
 */

// ─── Bookmark Management ───
export function setBookmark(filename: string | null) {
  const current = get(trackList);
  const updated = current.map(t => ({
    ...t,
    isActive: t.filename === filename
  }));
  trackList.set(updated);
}

// ─── Play (Stream/Detour) ───
export function play(item: any, apiBase: string) {
  const track = buildTrack(item);
  // We do NOT call setBookmark here because streaming is a "Detour".
  // The 'isActive' bookmark remains on the track in the queue, if any.
  window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
}

// ─── Queue (Promotion) ───
export function queue(item: any) {
  const track = buildTrack(item);
  // Dispatch an event to add to queue (Player component listens for this)
  // We include a flag indicating it should be bookmarked as active
  window.dispatchEvent(new CustomEvent('add-to-queue', { detail: track }));
}

// ─── Download ───
export function download(item: any, apiBase: string, callbacks?: any) {
  const track = buildTrack(item);
  downloadTrack(track, apiBase, callbacks);
}

// ─── State Helpers ───
export function isQueued(item: any): boolean {
  return !!get(trackList).find(t => t.filename === item.filename);
}

/**
 * Snapshots the current audio time into the current track's state.
 * Call this before switching tracks or navigating away.
 */
export function snapshotCurrentPosition(audioElement: HTMLAudioElement | null) {
  const current = get(currentTrackStore);
  if (!current || !audioElement) return;

  const pos = audioElement.currentTime;
  
  // Update the store's track reference directly
  current.position = pos;
  
  // If the track is in the queue, sync the position into the queue list
  const list = get(trackList);
  const idx = list.findIndex(t => t.filename === current.filename);
  if (idx !== -1) {
    list[idx].position = pos;
    trackList.set([...list]); // Trigger Svelte store update
  }
}
