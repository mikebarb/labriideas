// src/lib/menuDraftStore.ts
//
// Draft persistence for the menu.json admin editor — the "Draft Layer"
// of the GitOps menu pipeline (see ARD: Menu Configuration).
//
// WHY IndexedDB and not localStorage: menu.json is a structured object
// that will grow as playlists/featured content expand, and the editor
// needs to save whole objects, not strings. IndexedDB stores structured
// clones natively and has no practical size limit for this use. The
// draft is deliberately NOT the file system and NOT the server — it is
// a private, per-browser staging copy that lives until the admin either
// commits it to GitHub (Step C) or discards it.
//
// Lifecycle (per design):
//   1. Admin opens editor → existing draft loaded if present, else the
//      build-time master (deployed state) is used as the starting point.
//   2. Edits accumulate in the draft across page refreshes / tab closes
//      (this file's job).
//   3. "Save & Deploy" (Step C) commits the draft to GitHub → CI/CD
//      rebuilds the site → draft is cleared (the deployed master is
//      now the source of truth again).
//
// Storage layout: one database ('labri-admin'), one object store
// ('menu_draft'), one record ('current'). A single-record store keeps
// the API trivial and leaves the schema free to grow (e.g. a 'history'
// store of past drafts) without migrations of live data.

const DB_NAME = 'labri-admin';
const DB_VERSION = 1;
const STORE_NAME = 'menu_draft';
const RECORD_KEY = 'current';

// What gets stored alongside the draft menu itself.
export interface MenuDraft {
  menu: unknown;       // the draft menu.json object
  savedAt: string;     // ISO timestamp of the last save
  note?: string;      // optional admin note (future use)
}

// ─── Low-level IDB plumbing ───

function openDraftDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    // NOTE: IndexedDB is guaranteed available in the browsers this PWA
    // targets; a missing IDB would already have broken OPFS-era features.
    const req = indexedDB.open(DB_NAME, DB_VERSION);

    // Upgrade fires only on first open (or version bump) — the store is
    // created here, never in request handlers.
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        // keyPath matches the fixed RECORD_KEY so put() upserts the one record.
        db.createObjectStore(STORE_NAME, { keyPath: 'id' });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

// withStore runs a transaction against the draft store and resolves with
// the IDBRequest's result. Centralises the open → tx → request → await
// boilerplate so each public function stays a few lines.
function withStore<T>(
  mode: IDBTransactionMode,
  operation: (store: IDBObjectStore) => IDBRequest
): Promise<T> {
  return openDraftDB().then(
    db =>
      new Promise<T>((resolve, reject) => {
        const tx = db.transaction(STORE_NAME, mode);
        const req = operation(tx.objectStore(STORE_NAME));
        req.onsuccess = () => resolve(req.result as T);
        req.onerror = () => reject(req.error);
        // Close on transaction completion so we never leak connections.
        tx.oncomplete = () => db.close();
      })
  );
}

// ─── Public API ───

/**
 * Loads the current draft, or null when none exists (first visit, or
 * after a successful deploy cleared it).
 */
export function loadMenuDraft(): Promise<MenuDraft | null> {
  return withStore<MenuDraft | null>('readonly', store =>
    store.get(RECORD_KEY)
  ).then(draft => (draft && draft.menu !== undefined ? draft : null));
}

/**
 * Persists (upserts) the draft. Called on every "Save Draft" — cheap,
 * synchronous-feeling for the UI, and makes refresh-proofing trivial.
 */
export async function saveMenuDraft(menu: unknown): Promise<void> {
  const record: MenuDraft & { id: string } = {
    id: RECORD_KEY,
    menu,
    savedAt: new Date().toISOString(),
  };
  await withStore('readwrite', store => store.put(record));
}

/**
 * Deletes the draft. Called after a successful deploy (the master is
 * truth again) or on explicit discard.
 */
export async function clearMenuDraft(): Promise<void> {
  await withStore('readwrite', store => store.delete(RECORD_KEY));
}

/**
 * Existence check without deserialising the whole draft — used by the
 * future preview integration (isAdmin && draftExists → render draft)
 * and by the editor to show the "draft in progress" badge.
 */
export function menuDraftExists(): Promise<boolean> {
  return withStore<MenuDraft | null>('readonly', store =>
    store.getKey(RECORD_KEY)
  ).then(key => key !== undefined);
}
