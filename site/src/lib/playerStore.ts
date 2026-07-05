import { writable } from 'svelte/store';
import type { Track } from './types.ts';

//export const isPlaylistOpen = writable(false);
export const trackList = writable<Track[]>([]);
export const currentTrackStore = writable<Track | null>(null);
export const statusStore = writable<'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'error'>('idle');
export const isAdminStore = writable(false);

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

