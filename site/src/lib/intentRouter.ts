// src/lib/intentRouter.ts
import Fuse from 'fuse.js';
import type { Track } from './types';
import { sanitizeKeywords } from './dataUtils.js';

export interface SearchIntent {
  field: 'speaker' | 'category' | 'keyword' | 'title';
  value: string;
  matchCount: number;
  displayLabel: string;
}

export function findIntents(
  tracks: Track[], 
  query: string, 
  limit: number = 8
): SearchIntent[] {
  if (!query.trim()) return [];

  const lowerQuery = query.toLowerCase();
  const intents: SearchIntent[] = [];

  // 1. Search Speakers (Fuzzy match - speaker names have typos)
  const speakers = [...new Set(
    tracks.map(t => t.speaker).filter((s): s is string => typeof s === 'string' && s.length > 0)
  )];
  const speakerFuse = new Fuse(speakers.map(s => ({ name: s })), {
    keys: ['name'],
    threshold: 0.4,
    ignoreLocation: true
  });
  speakerFuse.search(query).forEach(result => {
    const speakerName = result.item.name;
    intents.push({
      field: 'speaker',
      value: speakerName,
      matchCount: tracks.filter(t => t.speaker === speakerName).length,
      displayLabel: speakerName
    });
  });

  // 2. Search Categories (Exact match - categories are controlled vocabulary)
  const categories = [...new Set(
    tracks.flatMap(t => {
      if (Array.isArray(t.category)) return t.category;
      return t.category ? [t.category] : [];
    }).filter((c): c is string => typeof c === 'string' && c.length > 0)
  )];
  categories
    .filter(c => c.toLowerCase().includes(lowerQuery))
    .forEach(category => {
      intents.push({
        field: 'category',
        value: category,
        matchCount: tracks.filter(t => {
          if (Array.isArray(t.category)) return t.category.includes(category);
          return t.category === category;
        }).length,
        displayLabel: category
      });
    });

  // 3. Search Keywords (Exact match - keywords are controlled)
  const keywords = [...new Set(
    tracks.flatMap(t => sanitizeKeywords(t.keywords))
  )];
  keywords
    .filter(k => k.includes(lowerQuery))
    .forEach(keyword => {
      intents.push({
        field: 'keyword',
        value: keyword,
        matchCount: tracks.filter(t => 
          sanitizeKeywords(t.keywords).includes(keyword)
        ).length,
        displayLabel: `#${keyword}`
      });
    });

  // 4. Search Titles (Fuzzy match - titles can have typos)
  const titleFuse = new Fuse(tracks, {
    keys: ['title'],
    threshold: 0.4,
    ignoreLocation: true
  });
  const titleResults = titleFuse.search(query);
  if (titleResults.length > 0) {
    // Group by unique titles and count occurrences
    const titleCounts = new Map<string, number>();
    titleResults.forEach(r => {
      const title = r.item.title || r.item.filename;
      titleCounts.set(title, (titleCounts.get(title) || 0) + 1);
    });
    titleCounts.forEach((count, title) => {
      intents.push({
        field: 'title',
        value: title,
        matchCount: count,
        displayLabel: title
      });
    });
  }

  // 5. Sort by match count (descending) and limit
  return intents
    .sort((a, b) => b.matchCount - a.matchCount)
    .slice(0, limit);
}

// Apply selected intents to a track set (AND logic between intents)
export function applyIntents(tracks: Track[], activeIntents: SearchIntent[]): Track[] {
  if (activeIntents.length === 0) return tracks;

  return tracks.filter(track => {
    return activeIntents.every(intent => {
      switch (intent.field) {
        case 'speaker':
          return track.speaker === intent.value;
        case 'category':
          if (Array.isArray(track.category)) return track.category.includes(intent.value);
          return track.category === intent.value;
        case 'keyword':
          return sanitizeKeywords(track.keywords).includes(intent.value.toLowerCase());
        case 'title':
          return (track.title || '').toLowerCase().includes(intent.value.toLowerCase()) ||
                 (track.filename || '').toLowerCase().includes(intent.value.toLowerCase());
        default:
          return true;
      }
    });
  });
}
