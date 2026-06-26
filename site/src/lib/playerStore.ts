import { writable } from 'svelte/store';
import type { Track } from './types.js';

export const isPlaylistOpen = writable(false);
export const trackList = writable<Track[]>([]);
export const currentTrackStore = writable<Track | null>(null);
export const statusStore = writable<'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'error'>('idle');
export const isAdminStore = writable(false);

// Live playback time and duration (for the seek bar in the Player UI)
export const currentTimeStore = writable(0);
export const durationStore = writable(0);
