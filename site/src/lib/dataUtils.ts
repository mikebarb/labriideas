// src/lib/dataUtils.ts
import type { Track } from './types';

export function sanitizeKeywords(raw: unknown): string[] {
  if (!raw) return [];
  
  let list: string[] = [];
  
  // Case 1: It's an array (could be strings, or arrays of arrays)
  if (Array.isArray(raw)) {
    list = raw.filter((item): item is string => typeof item === 'string');
  } 
  // Case 2: It's a comma-separated string
  else if (typeof raw === 'string' && raw.trim() !== '') {
    list = raw.split(',').map(s => s.trim());
  }
  // Case 3: It's a nested object (e.g., { en: ["prayer", "faith"] })
  else if (raw && typeof raw === 'object') {
    Object.values(raw).forEach(value => {
      if (Array.isArray(value)) {
        list.push(...value.filter((v): v is string => typeof v === 'string'));
      } else if (typeof value === 'string') {
        list.push(value);
      }
    });
  }

  return list
    .map(k => k.trim().toLowerCase())
    .filter(k => k.length >= 2 && /^[a-z0-9\s\-]+$/.test(k));
}

export function extractAllKeywords(tracks: Track[]): string[] {
  const kwSet = new Set<string>();
  tracks.forEach((track) => {
    const cleanKeywords = sanitizeKeywords(track.keywords);
    cleanKeywords.forEach(k => kwSet.add(k));
  });
  return Array.from(kwSet).sort();
}
