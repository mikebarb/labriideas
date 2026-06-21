// src/lib/rankedEngine.ts
import Fuse from 'fuse.js';
import type { Track } from './types';

export function rankedSearch(tracks: Track[], query: string): Track[] {
  if (!query.trim()) return tracks;

  // Fuse configuration with Weighted Scoring
  const fuse = new Fuse(tracks, {
    keys: [
      { name: 'speaker', weight: 0.5 },     // Speaker matches are gold
      { name: 'category', weight: 0.3 },    // Category matches are silver
      { name: 'title', weight: 0.2 },       // Title matches are bronze
      { name: 'keywords', weight: 0.1 }     // Keyword matches are good but broad
    ],
    threshold: 0.3,
    includeScore: true, // We need scores to sort
    ignoreLocation: true
  });

  const results = fuse.search(query);
  
  // Return the tracks, sorted by score (Fuse returns score 0 as perfect match)
  return results.map(r => r.item);
}
