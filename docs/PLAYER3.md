# Player Architecture Documentation

## Overview

The Player is a Svelte 5 component that handles audio playback for the Ideas Library. It receives track play requests via a `play-track` CustomEvent dispatched on `window`, manages a queue of tracks, and provides a full-featured playback UI (play/pause, seek, volume, playback speed, mobile/desktop layouts, and a queue drawer).

## Architecture Philosophy

After several iterations, we converged on a clean architecture where **the Track object is the single source of truth**. All runtime state (playback position, duration, presigned URL) is stored directly on the Track object, not in separate Maps or stores.

### Before (old approach — buggy)

Three separate Maps that had to be manually synced with the tracks array:

- `trackPositions` Map — filename to seconds
- `trackDurations` Map — filename to seconds
- `trackUrls` Map — filename to URL object

Stale data persisted when tracks were removed and re-added. Reactivity was fragile.

### After (current approach — clean)

The `tracks: Track[]` array is the single source of truth. Each Track object carries its own runtime state:

| Field | Type | Description |
|---|---|---|
| `filename` | `string` | Unique absolute file reference |
| `title` | `string?` | Display title |
| `speaker` | `string?` | Speaker name |
| `position` | `number?` | Last known playback position (seconds) |
| `duration` | `number?` | Track duration (seconds, 0 until loaded) |
| `url` | `string?` | Presigned S3 URL |
| `urlExpiresAt` | `number?` | Timestamp when URL expires |
| `playbackRate` | `number` | Current playback speed |
| `id`, `hash`, `metadata` | various | Catalog metadata |

When a track is removed from the queue, its object (with all runtime data) is simply filtered out. No stale Map entries to clean up. Reactivity works naturally with Svelte 5 `$state`.

## Key Concepts

There are three distinct playback operations:

| Concept | When | Responsibility |
|---|---|---|
| **Load** | First time a track is added | Fetch presigned URL, capture duration |
| **Play** | User clicks already-playing track | Toggle play/pause |
| **Resume** | User switches to a different track | Save old position, load new, restore position if available |

## Files

| File | Purpose |
|---|---|
| `src/components/Player.svelte` | Main player component (UI + audio logic) |
| `src/components/QueueDrawer.svelte` | Queue sidebar (shows progress bars) |
| `src/lib/playerStore.ts` | Shared Svelte stores (track list, current track, status, time) |
| `src/lib/buildTrack.js` | Builds a Track object from a catalog item |
| `src/lib/types.ts` | TypeScript interface for Track |

## Data Flow

### Play a track (from anywhere)

User clicks track in TrackCard / QueueDrawer / inline link

→ `window.dispatchEvent(new CustomEvent('play-track', { detail: track }))`

→ `Player.svelte` listens in `onMount`

→ `Player.playTrack(track)`:

1. If same track as current: toggle play/pause
2. If different track:
   - Save old track position: `currentTrack.position = audioElement.currentTime`
   - Add to `tracks` array if new
   - `loadTrack(track)`:
     - If URL missing or expired, fetch a new presigned URL
     - Set `audioElement.src = url` and load
     - On `loadedmetadata`, capture `track.duration`
   - Restore `track.position` if > 0
   - Set `currentTrack = track`
   - `audioElement.play()`

### Position tracking

Each `timeupdate` event (approx 250ms):

```js
currentTime = audioElement.currentTime;
$currentTrack.position = audioElement.currentTime;

Both the local currentTime variable (for seek bar) and the track’s own position field are updated together.
Remove track from queue
User clicks X in QueueDrawer → dispatches remove-from-queue with { filename } → Player.removeFromQueue(filename):

If it was the current track:
Queue empty? Stop playback.
Queue not empty? Play next track (fresh, no position restore).

Progress Bar (QueueDrawer)
Each track in the queue shows a progress bar. The logic reads from the track object:

buildTrack.js
Creates a Track object from a catalog item. Only populates immutable catalog fields; runtime fields are omitted (left undefined).

Adding Tracks to the Queue
Any component can add a track by dispatching a play-track event:
const track = buildTrack(catalogItem);
window.dispatchEvent(new CustomEvent('play-track', { detail: track }));

The Player will:
Add the track to the queue (if not already present)
Load its metadata (URL, duration)
Restore saved position (if any)
Start playback
Removing Tracks from the Queue
Dispatch a remove-from-queue event:

The Player will:
Remove the track from the queue (its position, duration, and URL are all discarded)
If it was the current track, either play the next one or stop
Downloading Tracks
Dispatch a download-track event (admin only):



