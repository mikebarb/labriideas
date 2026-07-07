// src/lib/searchEngine.ts
import Fuse from 'fuse.js';
import type { Track, SearchParams } from './types';
import { sanitizeKeywords } from './dataUtils.js';
import { rankedSearch } from './rankedEngine.js';

// ===== V3 Types (NEW) =====

export type Dimension = 'speakers' | 'categories' | 'keywords';

export interface MultiSearchParams {
  query: string;
  speakers: string[];
  categories: string[];
  keywords: string[];
}

// ===== URL Parsing =====

export function createSearchParamsFromUrl(): SearchParams {
  // Defensive: handle SSR or missing window
  if (typeof window === 'undefined') {
    return emptySearchParams();
  }

  const params = new URLSearchParams(window.location.search);
  return {
    title: (params.get('title') || '').trim(),
    speaker: (params.get('speaker') || '').trim(),
    category: (params.get('category') || '').trim(),
    keywords: parseKeywords(params.get('keywords')),
    isFuzzy: params.get('fuzzy') === 'true'
  };
}

function parseKeywords(raw: string | null): string[] {
  if (!raw) return [];
  return raw
    .split(',')
    .map(k => k.trim())
    .filter((k): k is string => k.length > 0);
}

export function emptySearchParams(): SearchParams {
  return {
    title: '',
    speaker: '',
    category: '',
    keywords: [],
    isFuzzy: false
  };
}

// ===== The Main Filter Pipeline (V1/V2) =====

export function filterTracks(tracks: Track[], searchParams: SearchParams): Track[] {
  let workingSet: Track[] = tracks;

  // Stage 1: Title filter (fuzzy or substring)
  if (searchParams.title) {
    workingSet = applyTitleFilter(workingSet, searchParams.title, searchParams.isFuzzy);
  }

  // Stage 2: Other filters (AND logic)
  workingSet = workingSet.filter((track) => {
    if (searchParams.speaker && !trackMatchesSpeaker(track, searchParams.speaker)) {
      return false;
    }
    if (searchParams.category && !trackMatchesCategory(track, searchParams.category)) {
      return false;
    }
    if (searchParams.keywords.length > 0 && !trackMatchesKeywords(track, searchParams.keywords)) {
      return false;
    }
    return true;
  });

  // Stage 3: Sort alphabetically
  return sortTracksAlphabetically(workingSet);
}

// ===== Individual Filter Helpers =====

function applyTitleFilter(tracks: Track[], query: string, isFuzzy: boolean): Track[] {
  if (isFuzzy) {
    return fuzzySearch(tracks, query, 'title');
  }
  const lowerQuery = query.toLowerCase();
  return tracks.filter((track) => {
    const title = (track.title || '').toLowerCase();
    const filename = (track.filename || '').toLowerCase();
    return title.includes(lowerQuery) || filename.includes(lowerQuery);
  });
}

function trackMatchesSpeaker(track: Track, speaker: string): boolean {
  return track.speaker === speaker;
}

function trackMatchesCategory(track: Track, category: string): boolean {
  const target = category.toLowerCase();
  if (Array.isArray(track.category)) {
    return track.category.some(c => c.toLowerCase() === target);
  }
  if (typeof track.category === 'string') {
    return track.category.toLowerCase() === target;
  }
  return false;
}

function trackMatchesKeywords(track: Track, keywords: string[]): boolean {
  const trackKeywords = sanitizeKeywords(track.keywords);
  if (trackKeywords.length === 0) return false;
  
  return keywords.every((searchKw) =>
    trackKeywords.some(trackKw => trackKw.includes(searchKw.toLowerCase()))
  );
}

function sortTracksAlphabetically(tracks: Track[]): Track[] {
  return [...tracks].sort((a, b) => {
    const titleA = (a.title || a.filename).toLowerCase();
    const titleB = (b.title || b.filename).toLowerCase();
    return titleA.localeCompare(titleB);
  });
}

// ===== Fuzzy Search =====

function fuzzySearch<T extends object>(
  items: T[],
  query: string,
  key: keyof T
): T[] {
  if (!query.trim()) return items;
  const fuse = new Fuse(items, {
    keys: [key as string],
    threshold: 0.4,
    includeScore: false,
    ignoreLocation: true
  });
  return fuse.search(query).map(result => result.item);
}

// ===== URL Writing Helper =====

export function writeSearchParamsToUrl(params: SearchParams): void {
  if (typeof window === 'undefined') return;

  const urlParams = new URLSearchParams();
  if (params.title) urlParams.set('title', params.title);
  if (params.speaker) urlParams.set('speaker', params.speaker);
  if (params.category) urlParams.set('category', params.category);
  if (params.keywords.length > 0) urlParams.set('keywords', params.keywords.join(','));
  if (params.isFuzzy) urlParams.set('fuzzy', 'true');

  const queryString = urlParams.toString();
  const newUrl = queryString 
    ? `${window.location.pathname}?${queryString}` 
    : window.location.pathname;

  window.history.replaceState({}, '', newUrl);
}

// ===== V3 Helpers (NEW) =====

/**
 * Private helper that applies the multi-value constraints (speakers,
 * categories, keywords) to a track list. Used by both filterTracksMulti
 * and getNarrowedPool so the two stay in sync.
 * 
 * If params has a dimension with an empty array, that dimension is 
 * treated as "no constraint" (i.e. matches everything).
 */
function _applyConstraints(tracks: Track[], params: MultiSearchParams): Track[] {
  const q = params.query.trim().toLowerCase();
  
  return tracks.filter(track => {
    // Speaker
    const sMatch = params.speakers.length === 0 || 
                   params.speakers.includes(track.speaker || '');
    
    // Category
    const trackCats = Array.isArray(track.category) 
      ? track.category 
      : (track.category ? [track.category] : []);
    const cMatch = params.categories.length === 0 || 
                   trackCats.some(c => params.categories.includes(c));
    
    // Keyword
    const trackKws = sanitizeKeywords(track.keywords);
    const kMatch = params.keywords.length === 0 || 
                   params.keywords.some(kw => trackKws.some(tk => tk.includes(kw.toLowerCase())));
    
    // Query (NEW): substring across title, filename, speaker, category, keywords
    const qMatch = q === '' || (() => {
      const haystack = [
        track.title || '',
        track.filename || '',
        track.speaker || '',
        ...trackCats,
        ...trackKws
      ].join(' ').toLowerCase();
      return haystack.includes(q);
    })();
    
    return sMatch && cMatch && kMatch && qMatch;
  });
}


/**
 * V3 main filter pipeline.
 * - Applies the multi-value constraints.
 * - If query is non-empty, ranks the results with rankedSearch.
 * - Otherwise, sorts alphabetically.
 */
export function filterTracksMulti(
  tracks: Track[], 
  params: MultiSearchParams
): Track[] {
  const filtered = _applyConstraints(tracks, params);
  
  if (params.query.trim()) {
    return rankedSearch(filtered, params.query);
  }
  return sortTracksAlphabetically(filtered);
}

/**
 * V3 pool derivation.
 * Returns the set of unique values available for the given dimension,
 * given the OTHER constraints. The dimension's own current selection
 * is ignored (so the user can see options to deselect).
 * 
 * Example: getNarrowedPool(tracks, params, 'speakers') returns all
 * speakers that would be valid if the current speaker selection 
 * were empty.
 */
export function getNarrowedPool(
  tracks: Track[], 
  params: MultiSearchParams, 
  dimension: Dimension
): string[] {
  // Build params with the requested dimension zeroed out
  const paramsWithoutDimension: MultiSearchParams = {
    query: params.query,
    speakers: dimension === 'speakers' ? [] : params.speakers,
    categories: dimension === 'categories' ? [] : params.categories,
    keywords: dimension === 'keywords' ? [] : params.keywords
  };
  
  // Find tracks that pass the OTHER constraints
  const subset = _applyConstraints(tracks, paramsWithoutDimension);
  
  // Extract unique values for the requested dimension
  const set = new Set<string>();
  subset.forEach(t => {
    if (dimension === 'speakers' && t.speaker) {
      set.add(t.speaker);
    } else if (dimension === 'categories') {
      const cats = Array.isArray(t.category) 
        ? t.category 
        : (t.category ? [t.category] : []);
      cats.forEach((c: string) => set.add(c));
    } else if (dimension === 'keywords') {
      sanitizeKeywords(t.keywords).forEach((k: string) => set.add(k));
    }
  });
  
  return Array.from(set).sort();
}

/**
 * Merges the "available pool" with the currently selected values.
 * Returns a sorted list where items in `selected` but NOT in `pool`
 * are marked isStale: true.
 * 
 * Used by SearchFilter3 to feed FilterPopover. The popover uses the
 * isStale flag to grey out items that no longer match but should
 * remain visible so the user can deselect them.
 */
export function mergeOptions(
  pool: string[], 
  selected: string[]
): { value: string; isStale: boolean }[] {
  const merged = new Set<string>([...pool, ...selected]);
  return Array.from(merged)
    .sort()
    .map(value => ({
      value,
      isStale: selected.includes(value) && !pool.includes(value)
    }));
}
