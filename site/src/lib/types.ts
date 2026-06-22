// src/lib/types.ts

export interface Track {
  filename: string;            // The unique absolute reference
  title?: string;              // Optional display title
  speaker?: string;            // Optional speaker name
  category?: string | string[]; // Can be one category or a list
  keywords?: string[];         // Optional array of tags
  duration?: string;           // Optional duration string
  localPreviewUrl?: string;
  hash?: string;
  metadata?: Record<string, any>;
  playbackRate?: number;        // <-- ADD: optional, defaults to 1.0
}

export interface SearchParams {
  title: string;
  speaker: string;
  category: string;
  keywords: string[];
  isFuzzy: boolean;
}

// Used by the URL layer
export type SearchParamKey = 'title' | 'speaker' | 'category' | 'keywords' | 'fuzzy';
