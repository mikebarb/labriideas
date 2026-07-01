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
| `src/lib/buildTrack.ts` | Builds a Track object from a catalog item |
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

buildTrack.ts
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


# Player & Queue System — Architecture & Implementation

## Overview

The L'Abri Ideas Library audio player features a full-featured queue management system with drag-and-drop and keyboard reordering, persistent playback across page navigations, and per-track progress tracking. Three Svelte components work together to provide this functionality:

| Component | File | Responsibility |
|-----------|------|----------------|
| **Player** | `src/components/Player.svelte` | Audio playback, mobile fullscreen UI, queue event handling, queue persistence |
| **QueueDrawer** | `src/components/QueueDrawer.svelte` | Visual queue list, drag-and-drop reorder, keyboard reorder, progress display |
| **AppShell** | `src/components/AppShell.svelte` | Layout container; desktop sidebar or mobile fullscreen overlay for the queue |

---

## Data Flow

```
User Action → QueueDrawer → Custom Event → Player → Store (`trackList`) → QueueDrawer (reactive)
                                         → IndexedDB (via store)
                                         → URL hash (persist position)
```

All queue mutations (add, remove, reorder) are dispatched as custom DOM events from `QueueDrawer` to `Player`, which is the authoritative owner of queue state. `Player` mutates its local `tracks` array, writes to the `trackList` Svelte store, and commits to IndexedDB via `commitQueue()`.

### Event Bus (Custom DOM Events)

| Event | Origin | Detail | Handler in Player |
|-------|--------|--------|-------------------|
| `play-track` | QueueDrawer row click | `Track` | `handlePlayTrackExternal` |
| `remove-from-queue` | QueueDrawer remove button | `{ filename }` | `removeFromQueue` |
| `reorder-queue` | QueueDrawer drag/keyboard | `{ filename, newIndex }` | `reorderQueue` |
| `download-track` | QueueDrawer download button | `Track` | `downloadTrack` |

---

## Component Details

### QueueDrawer (`src/components/QueueDrawer.svelte`)

#### Rendering Location

Rendered by `AppShell` in two layouts:

- **Desktop**: Inline sidebar (`hidden md:block`) that slides in/out via width transition
- **Mobile**: Fullscreen overlay (`md:hidden fixed inset-0`) with close button

#### Progress Visualization

Per-track progress is computed by `getProgress(track)`:

```typescript
type ProgressKind = 'placeholder' | 'filled' | 'none';

function getProgress(track: Track): { kind: ProgressKind; pct: number }
```

- **Currently playing track**: A dotted placeholder bar (8 vertical segments with varying opacity) — live reading of `$currentTimeStore`/`$durationStore` was removed to avoid excessive reactivity.
- **Previously played tracks**: A filled progress bar based on `track.position / track.duration`, frozen at the last reported position.
- **Unplayed tracks**: No progress bar displayed.

#### Reordering — Drag and Drop

Hand-rolled pointer-based drag-and-drop (no external library):

1. User presses the drag handle (`onpointerdown`) → `startDrag()` sets `draggedFilename`, `draggedIndex`, `isDragging`.
2. As the pointer moves (`handlePointerMove`):
   - `document.elementFromPoint()` finds the row under the cursor.
   - `closest('[data-row-index]')` walks up to the row wrapper.
   - `getBoundingClientRect()` determines if the cursor is in the top or bottom half.
   - Sets `dragOverIndex` to the insertion point.
3. Green indicator lines render at `dragOverIndex` positions.
4. On release (`handlePointerUp`): dispatches `reorder-queue` event with `{ filename, newIndex: dragOverIndex }`.

#### Reordering — Keyboard (Desktop Only)

`Alt+ArrowUp` / `Alt+ArrowDown` on a focused row:

1. `handleRowKeydown` checks `e.altKey && (e.key === 'ArrowUp' | 'ArrowDown')`.
2. On mobile (`window.innerWidth < 768`), keyboard reorder is silently skipped — users should use drag-and-drop.
3. Reads the row's current index from `data-row-index` attribute (live from DOM, avoids stale closure).
4. Computes target index using `index + 2` for down (compensates for splice-off-by-one in `Player`).
5. Dispatches `reorder-queue` with `{ filename, newIndex }`.
6. After dispatch, uses `setTimeout(0, ...)` → `document.querySelector('[data-row-filename="..."]').focus()` to retain focus on the moved row for repeated navigation.

#### Action Buttons

Each row shows three action buttons (plus a fourth for admin):

| Button | Icon | Behavior | Class pattern |
|--------|------|----------|---------------|
| Drag handle | `GripVertical` | `onpointerdown` → `startDrag` | `opacity-100 md:opacity-0 md:group-hover:opacity-100` |
| Remove | `X` | `onclick` → `removeFromQueue` | Same |
| Download (admin) | `Download` | `onclick` → `downloadTrack` | Same |

The `opacity-100 md:opacity-0 md:group-hover:opacity-100` pattern makes buttons **always visible on mobile** (no hover state) and **hover-to-reveal on desktop**.

### Player (`src/components/Player.svelte`)

#### Queue State Ownership

`Player` manages queue state in a local `tracks: Track[]` array. All mutations go through this array:

1. User action → event → handler → `tracks` mutated
2. Handler calls `trackList.set(tracks)` to update the store
3. Handler calls `commitQueue()` to persist to IndexedDB

#### `commitQueue()`

```typescript
function commitQueue(): void {
  const payload = tracks.map(t => ({
    filename: t.filename,
    duration: t.duration,
    position: t.position
  }));
  localStorage.setItem('queue', JSON.stringify(payload));
}
```

Saves only `filename`, `duration`, and `position` — enough for persistence without serializing the full track object.

#### Initialization (`$effect` on mount)

```typescript
$effect(() => {
  const saved = localStorage.getItem('queue');
  if (saved) {
    const restored = JSON.parse(saved).map((item: any) => ({
      ...item,
      title: item.filename,
      speaker: ''
    }));
    tracks = restored;
    trackList.set(tracks);
    currentPosition = 0;
  }
});
```

In a real app, this would also hydrate `title` and `speaker` by fetching track metadata.

#### Reordering (`reorderQueue`)

```typescript
function reorderQueue(filename: string, newIndex: number): void {
  const idx = tracks.findIndex(t => t.filename === filename);
  if (idx === -1 || idx === newIndex) return;

  const [moved] = tracks.splice(idx, 1);

  let adjustedNewIndex = newIndex;
  if (idx < newIndex) {
    adjustedNewIndex = newIndex - 1;  // Array is now one shorter
  }

  tracks.splice(adjustedNewIndex, 0, moved);
  tracks = [...tracks];
  trackList.set(tracks);
  commitQueue();
}
```

`newIndex` is the **pre-splice** position (the index to insert at before the item is removed). The adjustment `newIndex - 1` accounts for the shift when removing an item earlier in the array.

#### Network State Handling

Track list is downloaded during app init. `Player` listens for `update-track-list` events and replaces `tracks`:

```typescript
window.addEventListener('update-track-list', ((e: CustomEvent) => {
  tracks = e.detail;
  trackList.set(tracks);
}) as EventListener);
```

#### Playback State Persistence on Navigation

```typescript
window.addEventListener('pagehide', () => updateCurrentTrackPosition());
window.addEventListener('beforeunload', () => updateCurrentTrackPosition());
```

Saves the current track position to IndexedDB via `updateCurrentTrackPosition()` before the page unloads.

### AppShell (`src/components/AppShell.svelte`)

#### Layout

```svelte
<div class="flex h-full w-full overflow-hidden">
  <main class="flex-1 overflow-auto pb-24">
    {@render children()}
  </main>

  <!-- Desktop sidebar -->
  <div class="hidden md:block transition-all duration-300 overflow-hidden"
       class:!w-80={$isPlaylistOpen}
       class:w-0={!$isPlaylistOpen}>
    <QueueDrawer />
  </div>

  <!-- Mobile overlay -->
  {#if $isPlaylistOpen}
    <div class="md:hidden fixed inset-0 z-40 bg-[#0e0e0e] flex flex-col">
      <div class="flex items-center justify-end p-2 border-b border-neutral-800">
        <button onclick={() => isPlaylistOpen.set(false)}>✕</button>
      </div>
      <div class="flex-1 overflow-hidden">
        <QueueDrawer />
      </div>
    </div>
  {/if}
</div>
```

- **Desktop**: push-layout sidebar, slides in/out with width transition
- **Mobile**: fixed overlay, rendered below the player (z-40 vs player's z-50)
- The `pb-24` padding on `<main>` accounts for the fixed player bar at the bottom

#### Player in Layout (`LabriLayout.astro`)

```astro
<div transition:persist="audio-player" class="fixed bottom-0 left-0 right-0 z-50 ...">
  <Player client:load ... />
</div>
```

`transition:persist` preserves audio playback across Astro view transitions.

---

## Logical Flow Diagrams

### Queue Addition → Playback

```
Search page → user clicks track
  ↓
Add to queue API (not shown here, external) → updates trackList store
  ↓
Player initializes: restores queue from localStorage
  ↓
User taps mini-player
  ↓
Player expands to full screen (mobile) / shows now-playing (desktop)
  ↓
User taps Pause/Play
  ↓
statusStore updates → play/pause icon reflects state
```

### Queue Reordering (Drag and Drop)

```
User presses drag handle on row B (index 1)
  ↓
startDrag(): sets draggedFilename = B.filename, draggedIndex = 1, isDragging = true
  ↓
User drags B down past row C (index 2)
  ↓
handlePointerMove():
  - elementFromPoint() finds row C
  - closest('[data-row-index]') identifies row index 2
  - getBoundingClientRect(): pointer in bottom half → dragOverIndex = 3
  ↓
Green indicator renders between C and D
  ↓
User releases pointer
  ↓
handlePointerUp():
  - Dispatches reorder-queue { filename: B, newIndex: 3 }
  ↓
Player.reorderQueue():
  - idx = 1, newIndex = 3
  - splice(1, 1) → [A, C, D]
  - idx < newIndex → adjustedNewIndex = 2
  - splice(2, 0, B) → [A, C, B, D]
  ↓
tracks = [...tracks], trackList.set(), commitQueue()
  ↓
QueueDrawer re-renders with new order
```

### Queue Reordering (Keyboard — Desktop)

```
User clicks row C (index 2) → focus on row div with data-row-index="2"
  ↓
User presses Alt+ArrowDown
  ↓
handleRowKeydown():
  - moveDown = true, window
