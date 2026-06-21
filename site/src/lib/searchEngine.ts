// src/lib/searchEngine.ts
import Fuse from 'fuse.js';
import type { Track, SearchParams } from './types';
import { sanitizeKeywords } from './dataUtils.js'

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

// ===== The Main Filter Pipeline =====

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
