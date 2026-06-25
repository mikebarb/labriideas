# Player Component — Architecture & Behavior

## Overview

The Player is a persistent audio playback component rendered at the bottom of every page. It is a single Svelte 5 component (`Player.svelte`) that manages all audio playback, queue management, and position tracking. It communicates with the rest of the application exclusively through CustomEvents dispatched on the `window` object.

## Route Map

The Player is not a page — it is a layout-level component rendered by `LabriLayout.astro` and appears on every page of the site.

| File | Purpose |
|---|---|
| `src/components/Player.svelte` | The entire player: UI, audio element, queue, position tracking, event handling |

## Architecture

### State (local to Player.svelte)

| Variable | Type | Purpose |
|---|---|---|
| `tracks` | `Track[]` | The playback queue (ordered list of tracks) |
| `currentTrack` | `Track \| null` | The currently active track |
| `status` | `'idle' \| 'loading' \| 'playing' \| 'paused' \| 'buffering' \| 'error'` | Current playback state |
| `currentTime` | `number` | Current playback position in seconds |
| `duration` | `number` | Total duration of the current track in seconds |
| `volume` | `number` | Volume level (0.0 – 1.0) |
| `isMuted` | `boolean` | Whether audio is muted |
| `trackPositions` | `Map<string, number>` | Saved playback positions keyed by filename |
| `isDragging` | `boolean` | Whether the user is currently dragging the seek bar |
| `dragProgress` | `number` | Seek bar position during drag (0–100) |
| `isMobileExpanded` | `boolean` | Whether the mobile player is expanded to full screen |

### Shared Stores (for external components)

| Store | Purpose |
|---|---|
| `trackList` | Exposes the current queue to `QueueDrawer.svelte` |
| `currentTrackStore` | Exposes the current track to other components |
| `statusStore` | Exposes playback status to other components |
| `isPlaylistOpen` | Controls the visibility of the queue drawer |
| `isAdminStore` | Exposes admin mode status |

### Audio Element

A single `<audio>` element is created at mount time and reused for all playback. It is never destroyed or recreated.

## Event Communication

The Player listens for three CustomEvents on `window`:

| Event | Detail shape | Source components | Behavior |
|---|---|---|---|
| `play-track` | `Track` object | `TrackCard`, `TrackList`, `SchaefferGrid`, Featured inline script | Plays the track (or toggles play/pause if it's the current track) |
| `remove-from-queue` | `{ filename: string }` | `QueueDrawer` | Removes the track from the queue |
| `download-track` | `Track` object | Admin UI | Opens the track's presigned URL in a new tab |

## User-Facing Behavior

### Playback

- **Clicking a track** dispatches `play-track`. The Player receives the event and calls `playTrack()`.
- **If the track is already the current track:** playback toggles between play and pause.
- **If the track is a different track:** the Player saves the current track's position, loads the new track, and starts playback.
- **If the track has been played before in the same session:** playback resumes from the saved position.
- **If the track has never been played:** playback starts from 0:00.

### Queue

- Tracks are automatically added to the queue when played.
- The queue is displayed in the `QueueDrawer` (toggled by the queue icon in the Player).
- Tracks can be removed from the queue via the `QueueDrawer`.
- Removing the currently playing track advances to the next track in the queue.
- Removing all tracks hides the Player entirely.

### Position Tracking

- Each track's playback position is saved in the `trackPositions` Map, keyed by filename.
- Positions are saved on every `timeupdate` event (approximately every 250ms).
- Positions persist for the lifetime of the page session. They are **not** persisted across page reloads.
- When switching tracks, the old track's position is saved before the new track loads.
- When returning to a previously played track, the saved position is restored after the audio metadata loads.

### Seek Bar

- The seek bar shows the current playback position as a percentage of total duration.
- The user can drag the seek bar to jump to any position in the track.
- During a drag, the position display updates in real-time but the audio does not seek until the drag ends.

### Playback Speed

- The user can change playback speed (0.75x, 1.0x, 1.25x, 1.5x, 2.0x).
- The speed setting is per-track and persists for the duration of the session.

### Volume

- Volume is controlled by a slider (0–100%).
- Mute toggles between muted and the previous volume level.

### Previous / Next

- **Previous:** Goes to the previous track in the queue. If already at the first track, resets the current track to 0:00.
- **Next:** Goes to the next track in the queue. If at the last track, does nothing.
- **Auto-advance:** When a track ends, the Player automatically advances to the next track.

### Skip Forward / Backward

- **Back 15s:** Jumps 15 seconds backward in the current track.
- **Forward 30s:** Jumps 30 seconds forward in the current track.

### Mobile Layout

- On mobile screens, the Player shows a compact bar at the bottom of the screen.
- Tapping the bar expands it to a full-screen player with all controls.
- The full-screen player can be minimized back to the compact bar.

### Error Handling

- If a track fails to load, the Player shows an error state.
- The user can retry by clicking the play/pause button again.
- If the browser blocks autoplay (`NotAllowedError`), the Player enters a paused state and waits for user interaction.

## Logical Flow: Playing a Track

User clicks a track card or play button
       ↓
Source component (TrackCard, TrackList, SchaefferGrid, etc.)
calls buildTrack(item) to construct a Track object
       ↓
Source component dispatches 'play-track' CustomEvent on window
with the Track object as event.detail
       ↓
Player's onMount event listener receives the event
       ↓
Player calls playTrack(event.detail)
       │
       ├── Is this the same track that's already loaded?
       │     YES → Toggle play/pause. Done.
       │
       └── NO → Different track
             │
             ├── Save current track's position to trackPositions
             │
             ├── Add track to queue (if not already present)
             │
             ├── Set status = 'loading', currentTime = 0, duration = 0
             │
             ├── Fetch presigned URL from API
             │
             ├── Pause current audio, set new src, load
             │
             ├── Check trackPositions for saved position
             │     │
             │     ├── Has saved position > 0?
             │     │     YES → Attach loadedmetadata listener to seek to that position
             │     │
             │     └── NO → No seek needed (starts from 0)
             │
             ├── Update currentTrack to the new track
             │
             ├── Call audioElement.play()
             │
             └── Set status = 'playing'


## Key Design Decisions

1. **Single audio element, reused** — Avoids creating/destroying audio elements on every track change. Simpler state management.

2. **Position tracking via Map** — In-memory only. No localStorage or IndexedDB. Positions are lost on page reload. This is intentional — the Player is designed for session-based listening, not long-term bookmarking.

3. **Race condition prevention** — `currentTrack` is only updated to the new track *after* the audio source is loaded and the seek callback is registered. This prevents the `timeupdate` listener from writing the old track's position to the new track's entry in `trackPositions`.

4. **Event-driven communication** — The Player does not import or reference any page-level components. It listens for events on `window`. This keeps the Player decoupled from the rest of the application.

5. **Per-track playback speed** — Speed is stored on the Track object, not globally. This allows different tracks to play at different speeds if needed.

6. **Queue is append-only from play actions** — Tracks are added to the queue when played. There is no "add to queue" button (yet). The queue is displayed and managed via the QueueDrawer.

## Maintenance Notes

- **To add a new event the Player should listen for:** Add a new `window.addEventListener` call in the `onMount` block.
- **To change position persistence behavior:** Modify the `trackPositions` Map usage (e.g., add localStorage read/write).
- **To change the seek bar behavior:** Modify `onSeekDown`, `onSeekMove`, `onSeekUp`.
- **To add new playback controls:** Add buttons in the template and corresponding functions in the script section.
- **To change the mobile layout:** Modify the `{#if !isMobileExpanded}` and `{:else}` blocks in the template.

## Inline Play Links: Playing Tracks from Static Content

### Overview

Any Astro page can include clickable text links that play a track in the audio player. The link only needs a `data-play-filename` attribute containing the track's filename. A shared Svelte component looks up the filename in the catalog and dispatches the play event to the Player.

### Architecture
```
User clicks "The Five Themes..." link in Astro page
       ↓
Astro's script catches the click (event delegation)
       ↓
It reads data-play-filename attribute from the link
       ↓
Dispatches 'play-track-by-filename' CustomEvent with the filename
       ↓
PlayTrackDispatcher.svelte receives the event
       ↓
Looks up the track in its loaded catalog (getCatalog)
       ↓
Calls buildTrack() to construct the Track object
       ↓
Dispatches 'play-track' CustomEvent with the Track
       ↓
Player.svelte receives the event and plays the track
```

### Files Involved

| File | Purpose |
|---|---|
| `src/lib/playTrack.ts` | Utility functions for dispatching play events |
| `src/components/PlayTrackDispatcher.svelte` | Invisible Svelte island that loads the catalog and resolves filenames |
| Any Astro page | Contains the clickable link and a small script block |

### The Library: `src/lib/playTrack.ts`

```ts
import { buildTrack } from './buildTrack.js';
import type { Track } from './types.js';

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

The Dispatcher Component: src/components/PlayTrackDispatcher.svelte
This component renders nothing. It loads the catalog once on mount and listens for play-track-by-filename events. When it receives one, it looks up the filename in the catalog, builds a Track object using buildTrack(), and dispatches the real play-track event.

Usage in an Astro Page
Step 1: Import the dispatcher and add it to the page.
Step 2: Create clickable links using data-play-filename

Adding the Feature to Any Page
Import PlayTrackDispatcher.svelte in the Astro frontmatter
Add <PlayTrackDispatcher client:load /> anywhere in the template (it renders nothing)
Add the <script> block that wires up data-play-filename clicks
Add data-play-filename="your-file.mp3" to any <a> tag

Requirements
The filename must match a track’s filename field in the catalog
The catalog is loaded by PlayTrackDispatcher on mount — the page must be navigated to (not server-rendered without hydration)
Only one PlayTrackDispatcher per page is needed, regardless of how many links exist