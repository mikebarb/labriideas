# Design Document: Search & Discover System

## 1. Overview
The Search & Discover system provides a robust interface for users to browse a catalog of 2,000–3,000 academic/theological lectures. The system emphasizes mobile-first usability, quick interaction, and tight integration with the global persistent audio player.

## 2. Core Requirements
- **Performance:** All filtering performed in-memory using `catalog.json`.
- **Accessibility:** Large touch targets (44px+) and mobile-friendly stacking.
- **Reactivity:** Instant results via Svelte 5 runes.
- **State Management:** State persisted in URL query parameters for bookmarking and sharing.

## 3. Data Structures & Flow
- **Catalog Source:** `catalog.json` (Array of track objects).
- **UI State:** Bound to a single reactive object reflecting URL parameters (`title`, `speaker`, `category`, `keyword`, `isFuzzy`).
- **Filtering Algorithm:**
  1. **Stage 1:** Fuzzy matching (optional, via [Fuse.js](https://fusejs.io/)) on search terms.
  2. **Stage 2:** Boolean logic filters (Title `AND` Speaker `AND` Category `AND` Keywords).
  3. **Stage 3:** Alphabetical sorting by `title`.

## 4. Component Architecture
- **`search/index.astro`**: Layout shell. Sets top-level padding and positions sticky headers.
- **`SearchFilter.svelte`**: A sticky, collapsible filter bar.
  - *Title/Speaker*: Options for Exact, Contains, Fuzzy.
  - *Category*: Custom "strict-match" Combobox.
  - *Keywords*: "Chip/Tag" based input supporting AND logic.
- **`SearchResults.svelte`**: 
  - Render: Compact list mode by default for quick scanning.
  - Interaction: Accordion-style cards (Row expands on tap/click to reveal expanded controls/metadata).
- **Persistence**: The `Player.svelte` component is positioned globally in `LabriLayout.astro` as a sticky bottom bar, ensuring uninterrupted navigation while searching.

## 5. Architectural Decisions (Agreed Approaches)
- **Routing Strategy**: Pages are dedicated to specific collections (`/search`, `/schaeffer`, `/topics`). We avoid "slug-guessing" logic to prevent cross-pollination bugs.
- **Type Safety**: Shared `catalogStore.js` and TypeScript `interface` definitions ensure the filter logic and `TrackList` components remain in sync.
- **Component Logic**: Rendering and logic are encapsulated within Svelte components (using `$derived` for reactive filtering) to avoid Astro frontmatter parsing complexities.
- **UX Pattern**: Search filters are URL-driven. This makes the state shareable and allows users to "Refresh" the page without losing their current search results.

## 6. Layout & Responsive Strategy
- **Sticky Search Bar**: Moves with the content on desktop; pins to the top on mobile (just below the navigation).
- **Global Player**: Stays at `fixed bottom-0`. Layouts (wrappers) must provide `padding-bottom` (approx `80px`) to prevent UI overlap.
- **Touch UX**: Result cards expand on tap; play buttons are distinct and easily accessible.



===================================================

# Search & Discover Design Specifications

## 1. Interaction Design & UX Logic

### Category Dropdown (Strict Combobox)
- **Data Source:** Derived from the unique set of all `category` fields found in `catalog.json`.
- **Behavior:** 
  - Typing filters the available dropdown options.
  - **Strictness:** The field will not accept free-text input. Upon `blur` (exit), any entry that does not match an item in the Category Universe is cleared to prevent invalid filter states.

### Keyword Matching
- **Input Logic:** A tag-based system (single-word inputs).
- **Matching Depth:** Word-contains matching restricted to the `keywords` array field.
- **Boolean Logic:** AND-based filtering between all active filters (e.g., Title AND Speaker AND Category AND Keyword).

### Result List (Compact Expandable)
- **Compact View:** Default display is a single-row "List View" optimized for high-density scanning.
- **Interactive Expansion:** Clicking an item expands the row into a "Card" layout, revealing the play button and extra metadata, without requiring a page navigation.
- **Touch Optimization:** Row height is set to a minimum of 44px to comply with mobile touch-target guidelines.

### Search Strategy
- **Modes:** Exact (strict), Contains (substring), and Fuzzy (using Fuse.js).
- **Toggle:** Fuzzy search is an opt-in toggle (default: OFF) to allow users to trade precision for flexibility.

---

## 2. Technical Implementation Details

### Implementation Comparison
| Aspect | Desktop | Mobile |
| :--- | :--- | :--- |
| **Filter visibility** | Always visible in sidebar | Drawer that slides down from top |
| **Touch targets** | 36px minimum | 44px minimum (Apple HIG) |
| **Result list** | Multi-column grid (if applicable) | Single column, full-width cards |
| **Sticky elements** | Filter sidebar sticks on scroll | "X results" count sticks to top |
| **Keyboard** | Full keyboard navigation | Touch-optimized, no hover states |

### Technical Architecture
- **State Management**: Bound to a single reactive object reflecting URL parameters (`title`, `speaker`, `category`, `keyword`, `isFuzzy`).
- **Persistence**: Using URL query parameters ensures that search states are bookmarkable, shareable, and refresh-safe.
- **Persistent sticky player**: `Player.svelte` is positioned globally in `LabriLayout.astro` as a `fixed bottom-0` bar. Global content wrappers apply `pb-20` (padding-bottom) to ensure content visibility behind the player.

### Data Lifecycle
1. **Load:** `catalogStore.js` fetches `catalog.json` as the baseline.
2. **State:** Inputs bound to `URLSearchParams` store.
3. **Filter:** An `$derived` value computes the subset of tracks based on inputs + fuzzy logic.
4. **Render:** The component loops over the result and renders the "Expandable Card".

=======================================

# Architectural Specification: URL-Driven State Management

## 1. Overview
The application utilizes a "Browser-as-a-Database" architectural pattern to manage the state of the search and filtering system. By treating the browser URL as the primary "Source of Truth," we ensure that the application state is bookmarkable, shareable, and persistence-aware without requiring server-side reloads.

## 2. The Communication Mechanism
The system operates as a decoupling of **State Storage** (the URL) and **Event Notification** (Svelte Custom Events).

### A. The Message Board (State Storage)
- **Primary Mechanism:** URL Query Parameters (e.g., `?speaker=Francis+Schaeffer&fuzzy=true`).
- **Storage Logic:** When a user interacts with a filter, the UI updates the browser's address bar using `window.history.replaceState()`.
- **Persistence:** This process does not trigger a server request or page reload. It is an internal browser memory update.
- **Benefits:**
    - **Native History:** Users can use the browser "Back" button to undo filter changes.
    - **Refresh Safety:** State is preserved if the page is reloaded.
    - **Shareability:** Search states can be copied as links and sent to other users.

### B. The Notification Bell (Event Dispatching)
- **Primary Mechanism:** `window.dispatchEvent(new Event('search-updated'))`.
- **Notification Logic:** Since updating the URL does not inherently trigger reactive UI updates across different Svelte "islands," we use a custom event to notify components.
- **Decoupling Strategy:**
    - **The Sender (`SearchFilter`):** Does not need to know which components are listening. It simply updates the "Message Board" (URL) and rings the "Notification Bell" (Event).
    - **The Receiver (`SearchResults`):** Does not need to know how the UI state was changed. It simply listens for the bell, reads the Message Board, and re-filters the displayed list.

## 3. Data Lifecycle Diagram
The following cycle occurs every time a filter interaction takes place:

1. **User Interaction**: User toggles a filter or types a query.
2. **State Commit**: UI updates the URL Query Parameters via the Web History API.
3. **Signal Blast**: A `search-updated` event is dispatched across the `window` object.
4. **Data Sync**: The `SearchResults` component receives the event, parses the URL parameters, and updates its local reactive state.
5. **UI Re-render**: Svelte’s reactivity triggers a DOM update to reflect the new set of search results.

## 4. Why This Architecture?
| Feature | Old Web (Server-Side) | Modern Web (This Repo) |
| :--- | :--- | :--- |
| **URL Change** | Reloads Page / Hits Server | **Instant Client-Side Update** |
| **Search State** | Lost on refresh | **Persisted via URL** |
| **Back Button** | Usually breaks or restarts | **Natively Handled** |
| **Modularity** | Monolithic HTML | **Decoupled Svelte Islands** |

## 5. Security & Safety Notes
- **Sanitization:** All inputs read from the URL are sanitized and trimmed to prevent malformed queries.
- **Validity:** The "Strict Combobox" logic enforces category validity on exit, ensuring the URL cannot contain invalid filter states.
- **Performance:** Filtering is performed in-memory on the client using the pre-loaded `catalog.json`, ensuring the system scales to thousands of entries without server latency.



