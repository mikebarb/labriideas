// src/lib/menuDataStore.ts
//
// Reactive menu source — the "Draft Preview" layer of the GitOps menu
// pipeline (see ARD: Menu Configuration).
//
// THE PROBLEM IT SOLVES: menu.json is imported statically at build time
// by MegaMenu and TopicsTree. The admin editor needs uncommitted DRAFT
// changes visible in the live site (preview) before committing them to
// GitHub. A static import can't change at runtime — a store can.
//
// THE RULE (per design): if isAdmin AND a draft exists → the menu shows
// the draft. Otherwise → the deployed master. Non-admins and logged-out
// visitors ALWAYS see the master; the draft is private to the admin's
// browser (IndexedDB, per menuDraftStore).
//
// Reactivity chain:
//   menuDraftStore (IndexedDB)  →  this store  →  MegaMenu / TopicsTree
//   MenuEditor calls applyMenuDraft()/revertMenuToMaster() on every
//   save/revert/discard, so the preview tracks the editor exactly.

import { writable } from 'svelte/store';
import masterMenu from '../data/menu.json';
import { loadMenuDraft } from './menuDraftStore';
import { isAdmin } from './appStatusStore';

// The menu object every consumer renders. Initialized synchronously
// with the bundled master — the exact bytes the build shipped — so the
// first paint is identical to today for every visitor.
export const menuData = writable<any>(masterMenu);

// Which copy is live: 'master' (deployed) or 'draft' (admin preview).
// Components may show a subtle "preview" indicator off this.
export const menuPreviewSource = writable<'master' | 'draft'>('master');

// Structural gate for drafts. A draft that reaches this store is
// already parse-valid (MenuEditor refuses to save unparseable drafts)
// and schema-validated (Step B) — but belt-and-braces: the two
// components downstream historically threw on a missing Topics
// hierarchy. NEVER let a structurally-broken draft into the store;
// a public page must degrade to master, not crash.
function menuLooksValid(menu: any): boolean {
  return (
    !!menu &&
    Array.isArray(menu.subMenus) &&
    menu.subMenus.some(
      (s: any) => s && s.subMenu === 'Topics' && s.hierarchy
    )
  );
}

// ─── Bootstrap: run once, on first browser evaluation ───
// Auto-invoked at module load (guarded for SSR/prerender). Any page
// that renders a menu component loads this module, so the preview
// works on the home page without visiting the editor first.
let bootstrapped = false;
export function bootstrapMenuPreview(): void {
  if (bootstrapped || typeof window === 'undefined') return;
  bootstrapped = true;

  // Subscribe to admin status. Fires immediately with the current
  // value, and again on every login/logout transition:
  //   admin=true  → load draft (if any), apply if structurally valid
  //   admin=false → force master (logout must NEVER leave a draft live)
  isAdmin.subscribe(async (admin) => {
    if (admin) {
      try {
        const draft = await loadMenuDraft();
        if (draft && menuLooksValid(draft.menu)) {
          menuData.set(draft.menu);
          menuPreviewSource.set('draft');
        } else if (draft) {
          console.warn(
            '[menuDataStore] Draft exists but is structurally invalid — keeping master.'
          );
        }
      } catch (err) {
        console.warn('[menuDataStore] Draft load failed — keeping master.', err);
      }
    } else {
      menuData.set(masterMenu);
      menuPreviewSource.set('master');
    }
  });
}

bootstrapMenuPreview();

// ─── Editor control surface ───
// Called by MenuEditor so the preview tracks the editor's state exactly.

/** Apply a just-saved draft to the live preview (immediate visual feedback). */
export function applyMenuDraft(menu: unknown): void {
  if (menuLooksValid(menu)) {
    menuData.set(menu);
    menuPreviewSource.set('draft');
  } else {
    console.warn('[menuDataStore] Rejected structurally invalid draft preview.');
  }
}

/** Drop the preview — back to the deployed master (revert/discard). */
export function revertMenuToMaster(): void {
  menuData.set(masterMenu);
  menuPreviewSource.set('master');
}
