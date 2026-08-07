// src/lib/playerStore.ts
import { writable } from 'svelte/store';
import type { Track } from './types';

// ... existing store definitions ...
export const mobileView = writable<'min' | 'max' | 'list'>('min');
export const desktopQueueOpen = writable(false);
export const trackList = writable<Track[]>([]);
export const currentTrackStore = writable<Track | null>(null);
export const statusStore = writable<'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'error'>('idle');
export const isAdminStore = writable(false);
export const currentTimeStore = writable(0);
export const durationStore = writable(0);

/**
 * REFACTOR: We are moving toward a state-aware model where the 
 * Track objects themselves hold their loading status.
 * 
 * WHY: This allows components (like TrackCard or QueueDrawer) to 
 * derive their UI state (spinner vs. play button) directly from 
 * the track's status, ensuring consistent feedback across the app.
 * 
 * NOTE: When you update the 'loading' property on a track inside 
 * the 'trackList' array, ensure you trigger a store update 
 * (e.g., trackList.set([...$trackList])) so Svelte observers react.
 */
