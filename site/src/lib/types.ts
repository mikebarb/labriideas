// src/lib/types.ts

/**
 * A Track represents both the immutable catalog metadata and the
 * mutable runtime state for an audio track.
 * 
 * Runtime fields (position, duration, url, urlExpiresAt) are populated
 * by the Player component after the track is first loaded.
 */
export interface Track {
  filename: string;            // The unique absolute reference
  id?: string;                  // Optional identifier for this track (filename or catalog ID)
  title?: string;              // Optional display title
  speaker?: string;            // Optional speaker name
  category?: string | string[]; // Can be one category or a list
  keywords?: string[];         // Optional array of tags
  localPreviewUrl?: string;
  hash?: string;
  metadata: {
    title: string;
    artist: string;
    speaker: string;
  };
  playbackRate?: number;        // optional, defaults to 1.0
  
  // Mutable runtime state (populated by Player)
  position?: number;        // last known playback position in seconds
  duration?: number;        // track duration in seconds (0 until metadata loads)
  url?: string;             // presigned S3 URL ('' until first load)
  urlExpiresAt?: number;    // timestamp when URL expires (0 until first load)
  isDownloaded?: boolean;   // true if the track is cached in OPFS
  loading?: boolean;        // Used to track download/buffering state

  /**
   * The "Queue Bookmark" flag.
   * True if this track was the last one played from the queue.
   * Used by the Player to restore the active track on page reload 
   * or when returning from a "Detour" (streaming) session.
   */
  isActive?: boolean;       
}

// Previous value
//metadata?: Record<string, any>;

export interface SearchParams {
  title: string;
  speaker: string;
  category: string;
  keywords: string[];
  isFuzzy: boolean;
}

// Used by the URL layer
export type SearchParamKey = 'title' | 'speaker' | 'category' | 'keywords' | 'fuzzy';
