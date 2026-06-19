// src/lib/fuzzySearch.ts
import Fuse from 'fuse.js';

export function fuzzyFilter(tracks: any[], query: string, field: string = 'title'): any[] {
  if (!query || query.trim() === '') return tracks;
  
  const fuse = new Fuse(tracks, {
    keys: [field],           // Which field to search in
    threshold: 0.4,          // 0.0 = exact match, 1.0 = match anything
    includeScore: false,
    ignoreLocation: true,    // Search anywhere in the string
  });
  
  const results = fuse.search(query);
  return results.map(r => r.item);
}
