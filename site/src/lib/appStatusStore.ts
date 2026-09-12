// src/lib/appStatusStore.ts
import { writable, derived } from 'svelte/store';
import { authClient } from './authClient';

// Connectivity remains from your previous setup
export const isOnline = writable(true); 

// --- Admin state ---
// Starts false; only becomes true after the server confirms the token.
export const isAdmin = writable(false);

// True once the boot-time verification has completed. Guards against
// rendering admin UI (or flashing redirects) before we know the answer.
export const authChecked = writable(false);

export const appStatus = derived(
    [isOnline, isAdmin],
    ([$isOnline, $isAdmin]) => ({
        isOnline: $isOnline,
        isAdmin: $isAdmin
    })
);

// Verify the current token against the server and update the store.
// Returns the resolved admin state for callers that need it immediately.
export async function refreshAuth(): Promise<boolean> {
  const result = await authClient.status();
  isAdmin.set(result.isAdmin);
  authChecked.set(true);
  return result.isAdmin;
}
