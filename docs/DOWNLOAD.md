# Player & Queue System — Architecture & Implementation

## Overview

The L'Abri Ideas Library audio player features a full-featured queue management system with drag-and-drop and keyboard reordering, persistent playback across page navigations, per-track progress tracking, and admin download capability. Three Svelte components work together to provide this functionality:

| Component | File | Responsibility |
|-----------|------|----------------|
| **Player** | `src/components/Player.svelte` | Audio playback, mobile fullscreen UI, queue event handling, queue persistence |
| **QueueDrawer** | `src/components/QueueDrawer.svelte` | Visual queue list, drag-and-drop reorder, keyboard reorder, progress display, download button |
| **AppShell** | `src/components/AppShell.svelte` | Layout container; desktop sidebar or mobile fullscreen overlay for the queue |

Additionally, a shared library handles all download functionality:

| Library | File | Responsibility |
|---------|------|----------------|
| **Downloader** | `src/lib/downloader.ts` | `fetchPresignedUrl` and `downloadTrack` — used by both Player (for playback URLs) and UI components (for downloads) |

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
| `play-track` | QueueDrawer row click, TrackCard play button | `Track` | `handlePlayTrackExternal` |
| `remove-from-queue` | QueueDrawer remove button | `{ filename }` | `removeFromQueue` |
| `reorder-queue` | QueueDrawer drag/keyboard | `{ filename, newIndex }` | `reorderQueue` |

Downloads are **not** event-based. They call the shared library directly.

---

## Download Architecture

### Why a shared library

Downloads are available from two places:
1. **QueueDrawer** — download button on each queue row (admin users only)
2. **TrackCard** (search results) — download button on expanded search result cards (admin users only)

Both need the same logic: get a presigned URL, fetch the full file as a blob, trigger a browser download, and show a spinner while in progress. This logic is extracted into `src/lib/downloader.ts` so it's not duplicated.

### `src/lib/downloader.ts`

```typescript
export interface DownloadCallbacks {
  onStart?: () => void;
  onComplete?: () => void;
  onError?: (err: Error) => void;
}

export async function fetchPresignedUrl(
  filename: string,
  apiBase: string
): Promise<PresignedUrlResult>;

export function downloadTrack(
  track: Track,
  apiBase: string,
  callbacks: DownloadCallbacks
): void;
```

`fetchPresignedUrl` is also used by `Player.svelte` for loading playback audio URLs. It's not download-specific — it's a general presigned URL utility — but it lives in the downloader library because that's the primary consumer.

### `downloadTrack` flow

1. Component calls `downloadTrack(track, apiBase, callbacks)`
2. `onStart()` callback fires — component shows a spinner
3. `fetchPresignedUrl` gets the download URL from the API
4. `XMLHttpRequest` downloads the full file as a blob
5. A temporary `<a>` element is created with `download={track.filename}`
6. The browser's download manager saves the file to the user's Downloads folder
7. The blob URL is revoked (memory cleanup)
8. `onComplete()` callback fires — component hides the spinner
9. On error, `onError()` fires — component logs the error and hides the spinner

### Memory management

- The blob URL is revoked immediately after the download is triggered
- The browser can garbage-collect the blob once the download completes
- No memory leaks for typical lecture audio files (50–150 MB)

### Spinner UI

Both QueueDrawer and TrackCard show a small CSS spinner (16×16px) inside the download button while the download is in progress. The button is disabled during the download to prevent double-clicks.

---

## Component Details

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

`newIndex` is the **pre-splice** position. The adjustment `newIndex - 1` accounts for the shift when removing an item earlier in the array.

#### Download library integration

`Player` imports `fetchPresignedUrl` from `src/lib/downloader.ts` for playback URL loading. The `downloadTrack` function was removed from Player entirely — it's called directly by QueueDrawer and TrackCard.

#### Store mirroring

```typescript
$effect(() => { currentTrackStore.set(currentTrack); });
$effect(() => { statusStore.set(status); });
$effect(() => { isAdminStore.set(isAdmin); });
$effect(() => { currentTimeStore.set(currentTime); });
$effect(() => { durationStore.set(duration); });
```

`isAdminStore` is written by Player and read by `TrackCard.svelte` (via `SearchResults2.svelte`) to control visibility of the download button.

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

- **Currently playing track**: A dotted placeholder bar (8 vertical segments with varying opacity)
- **Previously played tracks**: A filled progress bar based on `track.position / track.duration`, frozen at the last reported position
- **Unplayed tracks**: No progress bar displayed

#### Reordering — Drag and Drop

Hand-rolled pointer-based drag-and-drop (no external library):

1. User presses the drag handle (`onpointerdown`) → `startDrag()`
2. `handlePointerMove` uses `document.elementFromPoint()` + `closest('[data-row-index]')` + `getBoundingClientRect()` to determine insertion point
3. Green indicator lines render at `dragOverIndex` positions
4. On release (`handlePointerUp`): dispatches `reorder-queue` event

#### Reordering — Keyboard (Desktop Only)

`Alt+ArrowUp` / `Alt+ArrowDown` on a focused row. Disabled on mobile (`window.innerWidth < 768`). Uses `setTimeout(0)` refocus to retain focus on the moved row.

#### Download Button

```typescript
import { downloadTrack } from '../lib/downloader';

let downloadingFilename: string | null = $state(null);

function handleDownload(track: Track) {
  if (downloadingFilename === track.filename) return;
  downloadingFilename = track.filename;

  downloadTrack(track, apiBase, {
    onComplete: () => { downloadingFilename = null; },
    onError: (err) => { console.error('Download failed:', err); downloadingFilename = null; },
  });
}
```

Button UI:
- Shows spinner during download
- Disabled state prevents double-clicks
- Always visible on mobile (`opacity-100 md:opacity-0 md:group-hover:opacity-100`)

### TrackCard (`src/components/TrackCard.svelte`)

Search result card with built-in download button (admin users only).

#### Props

```typescript
interface Props {
  item: any;
  expanded?: boolean;
  ontoggle?: (filename: string) => void;
  children?: Snippet<[any]>;
  apiBase?: string;    // For download API calls
  isAdmin?: boolean;   // Controls download button visibility
}
```

#### Download Button

The download button is baked into TrackCard (Option A design decision) rather than using the children snippet pattern. This avoids threading `apiBase` through the snippet chain.

The button appears inside the expanded card section, alongside the keywords/category, on the right side.

#### `isAdmin` Flow

```
LabriLayout.astro passes isAdmin to Player.svelte
  → Player writes isAdminStore.set(isAdmin)
    → SearchResults2.svelte reads $isAdminStore
      → Passes to SearchResultsDisplay → TrackCardGroup → TrackCard
```

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
    <QueueDrawer {apiBase} />
  </div>

  <!-- Mobile overlay -->
  {#if $isPlaylistOpen}
    <div class="md:hidden fixed inset-0 z-40 bg-[#0e0e0e] flex flex-col">
      ...
      <QueueDrawer {apiBase} />
    </div>
  {/if}
</div>
```

`apiBase` is passed from `LabriLayout.astro` to `AppShell`, then to `QueueDrawer`.

---

## Logical Flow Diagrams

### Queue Addition → Playback

```
Search page → user clicks track
  ↓
Add to queue API (external) → updates trackList store
  ↓
Player initializes: restores queue from localStorage
  ↓
User taps mini-player → Player expands
  ↓
User taps Pause/Play → statusStore updates
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
User releases pointer → handlePointerUp()
  ↓
dispatches reorder-queue { filename: B, newIndex: 3 }
  ↓
Player.reorderQueue(): splice logic → [A, C, B, D]
  ↓
tracks = [...tracks], trackList.set(), commitQueue()
  ↓
QueueDrawer re-renders with new order
```

### Queue Reordering (Keyboard — Desktop)

```
User clicks row C (index 2) → focus on row div
  ↓
User presses Alt+ArrowDown
  ↓
handleRowKeydown():
  - moveDown = true, window.innerWidth >= 768
  - Reads data-row-index = 2 from e.currentTarget
  - targetIndex = 2 + 2 = 4
  - Dispatches reorder-queue { filename: C, newIndex: 4 }
  ↓
Player.reorderQueue(): [A, B, D, C, E]
  ↓
setTimeout(0) refocus: finds row with data-row-filename="C", calls .focus()
  ↓
User presses Alt+ArrowDown again
  - data-row-index now = 3 (C moved to index 3)
  - targetIndex = 3 + 2 = 5
  - Dispatches → moves C past E → [A, B, D, E, C]
```

### Download (Queue or Search)

```
User clicks download button (admin only)
  ↓
handleDownload(track):
  - Sets downloadingFilename = track.filename (shows spinner)
  - Calls downloadTrack(track, apiBase, callbacks)
  ↓
downloadTrack():
  - Calls fetchPresignedUrl(filename, apiBase) → gets presigned URL
  - XMLHttpRequest GETs the audio file as a blob
  - Creates <a download={filename}> with blob URL
  - Triggers click → browser download manager saves file
  - Revokes blob URL (memory cleanup)
  ↓
onComplete() fires → downloadingFilename = null (hides spinner)
```

---

## Key Design Decisions

1. **Events over direct calls**: QueueDrawer dispatches DOM events instead of importing Player functions. This keeps components decoupled and avoids circular dependencies.

2. **`commitQueue()` explicit calls**: No `$effect` watcher on `trackList` — mutations explicitly call `commitQueue()` after each change. Prevents redundant writes (especially during live playback position updates).

3. **Per-track position frozen**: No live time tracking. `getProgress` reads from the frozen `track.position` field, not from the live `$currentTimeStore`. Prevents unnecessary re-renders.

4. **Keyboard reorder on desktop only**: `Alt+ArrowUp/Down` is disabled on mobile (`window.innerWidth < 768`). Drag-and-drop is the expected mobile interaction.

5. **Drag handle always visible on mobile**: `opacity-100 md:opacity-0` pattern ensures the drag handle is always visible on touch devices.

6. **No `$effect` for focus recovery**: `setTimeout(0)` + `document.querySelector().focus()` after reorder is simpler and more predictable than the `$effect` + `queueMicrotask` approach.

7. **Download button baked into TrackCard (Option A)**: Rather than threading `apiBase` through a children snippet, the download button is built directly into TrackCard. This is simpler and avoids prop drilling through the component chain.

8. **Shared download library**: `src/lib/downloader.ts` houses both `fetchPresignedUrl` (used by Player for playback URLs) and `downloadTrack` (used by QueueDrawer and TrackCard). This avoids duplication and keeps the download logic centralized.

9. **`isAdmin` via store**: `Player.svelte` writes `isAdminStore.set(isAdmin)`, which is read by `SearchResults2.svelte` to control download button visibility. This avoids threading `isAdmin` through the layout → search → display → card chain.

10. **Download uses blob, not streaming**: The full file is fetched as a blob before triggering the browser download. This is intentional — the user wants the entire file, and lecture audio files (50-150 MB) are small enough that this is memory-safe. The blob URL is revoked immediately after the download starts.

---

## Version History

| Date | Change |
|------|--------|
| 2026-07-01 | Initial queue + player architecture |
| 2026-07-01 | Removed `$effect` mirror in Player, added explicit `commitQueue()` |
| 2026-07-01 | Added drag-and-drop reorder with green indicator |
| 2026-07-01 | Added keyboard reorder (Alt+Arrow) with focus recovery |
| 2026-07-01 | Mobile: fullscreen overlay for queue, buttons always visible |
| 2026-07-01 | Frozen per-track progress (dotted placeholder for current track) |
| 2026-07-01 | Created `src/lib/downloader.ts` — shared download library with `fetchPresignedUrl` and `downloadTrack` |
| 2026-07-01 | Moved `fetchPresignedUrl` from Player to downloader library (both Player and download use it) |
| 2026-07-01 | Added download button to QueueDrawer with spinner |
| 2026-07-01 | Added download button to TrackCard (search results) with spinner |
| 2026-07-01 | Removed `download-track` custom event — downloads now call library directly |

---

## Future Considerations

- **Accessibility**: Add `aria-activedescendant` / `role="listbox"` for screen reader support.
- **Touch drag**: Ensure `touch-action: none` is respected on all mobile browsers; test with external keyboard on iPad.
- **Download feedback**: Current implementation has a spinner but no percentage progress. For very large files, consider adding a progress bar during the `fetch()` phase.
- **`fetchPresignedUrl` caching**: The presigned URL is fetched fresh for each download. Consider caching the URL until it expires to reduce API calls.
