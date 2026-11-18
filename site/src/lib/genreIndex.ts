// src/lib/genreIndex.ts
//
// Derives the two-layer genre tree (group → subtopic) from the lecture
// catalogue. Unlike the Topics tree (admin-edited content in menu.json),
// the genre structure is DERIVED DATA: it is computed from whatever
// genre strings exist in the catalogue at read time, so adding a new
// genre needs no rebuild and no admin edit.
//
// ─── Data model ─────────────────────────────────────────────────────
// A catalogue genre string looks like:  "06a Exposition - Old testament"
//   NNx        → optional two-digit group order + letter (not trusted:
//                see GENRE_ORDER below — we parse but never rely on it)
//   <Group>    → top-level display grouping ("Exposition")
//   -  /  :    → the only two separators expected (see parse)
//   <Subtopic> → leaf display name ("Old Testament" after title-casing)
//
// Each display leaf may be fed by MORE THAN ONE raw string (e.g.
// "06a Exposition - Old testament" and "06a Exposition: Old testament"
// both exist in the wild). Leaf matching is therefore done by TrackList's
// genre mode: the leaf passes ALL of its raw strings and any track
// whose genre equals any of them is included. That is why the leaf
// carries `genre: string[]` rather than a single string.
//
// ─── Lifecycle ──────────────────────────────────────────────────────
// The tree is built lazily on first getGenreTree() call and memoised
// against getCatalogVersion() (Option 1 — lazy, version-keyed memo).
// A background revalidation that installs a new catalogue changes the
// version, which invalidates the memo; the next reader transparently
// rebuilds. catalogStore.ts stays unaware of genres — no coupling in
// either direction.
//
// No track lists are cached here: TrackList re-filters the catalogue
// itself at render time. This module only owns the STRUCTURE.

import { getCachedCatalog, getCatalogVersion } from './catalogStore';

// ─── Public shape ───────────────────────────────────────────────────

export interface GenreLeaf {
  /** Title-cased subtopic, e.g. "Its Role". */
  displayName: string;
  /** Every raw catalogue genre string that maps to this leaf. */
  rawGenres: string[];
}

export interface GenreGroup {
  /** Title-cased group name, e.g. "L'Abri". */
  name: string;
  /** Alphabetically sorted subtopics. */
  subtopics: GenreLeaf[];
}

export interface GenreTree {
  /** Alphabetically sorted groups. */
  groups: GenreGroup[];
}

// ─── Ordering flag ──────────────────────────────────────────────────
// 'alphabetical' — current decision: sort groups and subtopics by name.
// 'numeric'      — dormant alternative: sort by the leading "NNx" code
//                  (group by NN, subtopic by letter). Kept implemented
//                  so user feedback later is a one-line flip. The code
//                  is NOT trusted as the default: it has known gaps
//                  (04e, 08h, 10e skipped) and a history of drift.
const GENRE_ORDER: 'alphabetical' | 'numeric' = 'alphabetical';

// ─── Parsing helpers ────────────────────────────────────────────────

/**
 * Strips an optional leading order code ("01a ", "06A ") and splits the
 * remainder into [group, subtopic] on the FIRST occurrence of " - " or
 * ": ". Returns null for anything unparsable — the caller logs it and
 * skips the track.
 *
 * Notes:
 *  - The separator set is deliberately narrow: the full genre list uses
 *    only these two, and a bare "-" must NOT split (a subtopic could
 *    legitimately contain a hyphen, e.g. "Male & female" — but more
 *    importantly "Sexuality, gender & feminism" style names must never
 *    be chopped mid-word by a loose dash rule).
 *  - Splitting on the FIRST occurrence means a stray later " - " inside
 *    a subtopic stays part of it — the agreed failure-safe direction.
 */
function parseGenre(raw: string): { group: string; subtopic: string } | null {
  // Optional "NNx " prefix — case-insensitive on the letter, tolerant
  // of multiple spaces after the code.
  const withoutCode = raw.replace(/^\s*\d{2}[a-zA-Z]\s+/, '');

  // First separator wins; try " - " (with surrounding spaces) before
  // ": " so "Exposition: Old testament" and "Exposition - Old testament"
  // both reduce to group "Exposition", subtopic "Old testament".
  for (const sep of [' - ', ': ']) {
    const idx = withoutCode.indexOf(sep);
    if (idx > 0) {
      const group = withoutCode.slice(0, idx).trim();
      const subtopic = withoutCode.slice(idx + sep.length).trim();
      if (group && subtopic) return { group, subtopic };
    }
  }
  return null;
}

/**
 * Title-case: uppercase the FIRST character of each word and leave the
 * rest of the word untouched. This is deliberate — a naive "capitalize
 * every word fully" would mangle internal capitals like l'Abri → L'abri,
 * while leaving the rest alone turns "Old testament" → "Old Testament"
 * and keeps "l'Abri" exactly as spelled.
 */
function titleCase(s: string): string {
  return s
    .split(/\s+/)
    .map(word => (word ? word.charAt(0).toUpperCase() + word.slice(1) : word))
    .join(' ');
}

/** Case/separator-insensitive collapse key for a parsed genre. */
function collapseKey(group: string, subtopic: string): string {
  return `${group.toLowerCase()}|${subtopic.toLowerCase()}`;
}

/** Alphabetical comparator, accent/case-insensitive. */
function alphaCompare(a: string, b: string): number {
  return a.localeCompare(b, undefined, { sensitivity: 'base' });
}

/** Numeric comparator on the leading "NNx" code (dormant branch). */
function codeOf(raw: string): string {
  const m = raw.match(/^\s*(\d{2})([a-zA-Z])\s+/);
  // Unparsable codes sort last, deterministically.
  return m ? `${m[1]}${m[2].toLowerCase()}` : 'zzz';
}

// ─── Memo ───────────────────────────────────────────────────────────

let memoVersion: string | null = null;
let memoTree: GenreTree | null = null;
// Serialises concurrent first-calls: two components calling
// getGenreTree() in the same tick must not build the tree twice.
let buildPromise: Promise<GenreTree> | null = null;

// ─── Builder ────────────────────────────────────────────────────────

async function buildTree(): Promise<GenreTree> {
  const { tracks } = await getCachedCatalog();

  // Collapse key → leaf under construction. Insertion-ordered, and the
  // FIRST raw spelling seen for a key wins for the display name (agreed
  // tie-break: catalogue array order is the tie-break source).
  const leafMap = new Map<string, { group: string; subtopic: string; rawGenres: string[] }>();

  for (const t of tracks ?? []) {
    const g = t?.genre;
    // Guard: genre is singular today, but accept an array of strings
    // with the same robustness TrackList applies to category.
    const raws: string[] = Array.isArray(g) ? g : g ? [g] : [];
    for (const raw of raws) {
      if (typeof raw !== 'string' || !raw.trim()) continue;
      const parsed = parseGenre(raw);
      if (!parsed) {
        console.warn(`[genreIndex] Unparsable genre string: "${raw}"`);
        continue;
      }
      const key = collapseKey(parsed.group, parsed.subtopic);
      const existing = leafMap.get(key);
      if (existing) {
        if (!existing.rawGenres.includes(raw)) existing.rawGenres.push(raw);
      } else {
        leafMap.set(key, { group: parsed.group, subtopic: parsed.subtopic, rawGenres: [raw] });
      }
    }
  }

  // Assemble groups: collapse key → group bucket.
  const groupMap = new Map<string, GenreGroup>();
  for (const { group, subtopic, rawGenres } of leafMap.values()) {
    const gKey = group.toLowerCase();
    let bucket = groupMap.get(gKey);
    if (!bucket) {
      bucket = { name: titleCase(group), subtopics: [] };
      groupMap.set(gKey, bucket);
    }
    bucket.subtopics.push({ displayName: titleCase(subtopic), rawGenres });
  }

  const groups = [...groupMap.values()];

  if (GENRE_ORDER === 'numeric') {
    // Dormant ordering: group order by the lowest NN seen in the group,
    // subtopics by their code letter. Uses rawGenres[0]'s code.
    const minCode = (grp: GenreGroup) =>
      grp.subtopics.reduce(
        (min, st) => (codeOf(st.rawGenres[0]) < min ? codeOf(st.rawGenres[0]) : min),
        'zzz'
      );
    groups.sort((a, b) => (minCode(a) < minCode(b) ? -1 : minCode(a) > minCode(b) ? 1 : alphaCompare(a.name, b.name)));
    for (const grp of groups) {
      grp.subtopics.sort((a, b) => codeOf(a.rawGenres[0]).localeCompare(codeOf(b.rawGenres[0])));
    }
  } else {
    groups.sort((a, b) => alphaCompare(a.name, b.name));
    for (const grp of groups) {
      grp.subtopics.sort((a, b) => alphaCompare(a.displayName, b.displayName));
    }
  }

  return { groups };
}

// ─── Public API ─────────────────────────────────────────────────────

/**
 * Returns the genre tree derived from the cached catalogue.
 *
 * - Built lazily on first call; memoised against getCatalogVersion().
 * - A background revalidation that changes the catalogue version
 *   invalidates the memo; the next call transparently rebuilds.
 * - Concurrent first-calls share one build (single-flight promise).
 * - Never caches track lists — TrackList re-filters the catalogue
 *   itself. This module owns structure only.
 */
export async function getGenreTree(): Promise<GenreTree> {
  const version = getCatalogVersion();

  // Memo hit: same catalogue version as the last build.
  if (memoTree && memoVersion === version) return memoTree;

  // Single-flight: an in-progress build is shared by all callers.
  if (!buildPromise) {
    buildPromise = buildTree()
      .then(tree => {
        memoTree = tree;
        memoVersion = version;
        return tree;
      })
      .finally(() => {
        buildPromise = null;
      });
  }
  return buildPromise;
}
