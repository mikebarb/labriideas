//import { buildTrack } from './buildTrack.ts';
import type { Track } from './types.ts';

/**
 * Dispatch a play-track event with a fully built Track object.
 * Use this when you already have a Track ready to play.
 */
export function dispatchPlayTrack(track: Track): void {
  window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
}

/**
 * Dispatch a request to play a track by its filename.
 * A Svelte component with catalog access will handle the lookup.
 */
export function requestPlayByFilename(filename: string): void {
  window.dispatchEvent(new CustomEvent('play-track-by-filename', { detail: filename }));
}
