// src/stores/stores.js
import { writable } from 'svelte/store';

// Triggered when a user clicks "Play" in the CatalogViewer
export const playRequest = writable(null);

// Shared catalog data so the Player knows the full track list (required for Next/Prev to work)
export const catalogStore = writable([]);
