import { writable } from 'svelte/store';
import type { Track } from './types.ts';
// Removed: import type { DownloadedTrack } from './opfsStore.ts'; (Was causing type leaks/errors)
// Removed: import { isOnline as isOnlineStore, verifyConnectivity } from './connectivityStore.ts'; (Dead import)

export const trackList = writable<Track[]>([]);
export const currentTrackStore = writable<Track | null>(null);
export const statusStore = writable<'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'error'>('idle');

// Live playback time and duration (for the seek bar in the Player UI)
export const currentTimeStore = writable(0);
export const durationStore = writable(0);

// Three views on mobile:
//   'min'  → minimised player bar (default)
//   'max'  → expanded player detail
//   'list' → track list / queue (handled by AppShell's overlay)
//
// Buttons in the persistent bottom bar toggle between these.
export const mobileView = writable<'min' | 'max' | 'list'>('min');

// On desktop, the queue drawer is a sidebar that can be opened/closed
// independently of the player. On mobile, the queue is one of the three
// views above (mobileView === 'list'), so this doesn't apply.
export const desktopQueueOpen = writable(false);

// ─── Connectivity State ───
// Re-exported from the dedicated connectivity module so components
// can import everything player-related from one place.
export { isOnline, verifyConnectivity } from './connectivityStore.ts';

// ─── Download State (for progress UI) ───
export type DownloadStatus = 'idle' | 'downloading' | 'error';
export const downloadStatusStore = writable<{
  status: DownloadStatus;
  filename: string | null;
  progress: number;  // 0-100
  error: string | null;
}>({
  status: 'idle',
  filename: null,
  progress: 0,
  error: null
});
