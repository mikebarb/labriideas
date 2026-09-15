<script lang="ts">
  // Small admin tool: export the full catalog metadata to a CSV file,
  // saved to the browser's Downloads folder.
  //
  // Freshness guarantee (two parts):
  //   1. Connectivity gate: verifyConnectivity() proves the Go server is
  //      reachable BEFORE trusting any export. Admin tools are online-only
  //      by design — offline admin has no meaning.
  //   2. getCatalog() BLOCKS on the server ETag check (304 = local copy
  //      verified current; 200 = fresh bytes downloaded and cached). This
  //      is the admin-grade catalog getter per catalogStore's own contract
  //      — getCachedCatalog() is the stale-tolerant display path and is
  //      deliberately NOT used here.
  //
  // CSV details:
  //   - A UTF-8 BOM is prepended so Excel opens the file with correct
  //     encoding (Excel otherwise guesses the codepage and mangles any
  //     non-ASCII metadata).
  //   - CRLF line endings (RFC 4180 — Excel's native dialect).
  //   - Fields containing commas, quotes, or newlines are quoted with
  //     doubled inner quotes, per RFC 4180.
  import { getCatalog, clearCatalogMemoryCache } from '../../lib/catalogStore';
  import { verifyConnectivity } from '../../lib/connectivityStore';

  let exporting = $state(false);
  let statusMessage = $state('');
  let errorMessage = $state('');
  let lastExportSummary = $state('');

  // Schema-ordered base columns. filename is FIRST and is the catalog's
  // unique key (R2-enforced), so the CSV can be re-imported or diffed
  // against bulk-uploader CSVs by filename. Mirror of CatalogSchema in
  // publisher/pkg/schema/schema.go — KEEP IN SYNC.
  const BASE_COLUMNS = [
    'filename',
    'title',
    'artist',
    'speaker',
    'category',
    'keywords',
    'year',
    'topten',
    'audio-hash',
  ];

  // RFC 4180 field escaping: wrap in quotes if the value contains a
  // comma, quote, CR or LF; double any embedded quotes.
  function csvEscape(value: unknown): string {
    const s = value === null || value === undefined ? '' : String(value);
    if (/[",\r\n]/.test(s)) {
      return '"' + s.replace(/"/g, '""') + '"';
    }
    return s;
  }

  // Resolve the column list: the schema-ordered base columns, plus any
  // extra fields present in the catalog but not in BASE_COLUMNS (in
  // first-seen order). If CatalogSchema grows in Go, new fields flow
  // through here automatically as extras — exports never silently drop
  // data. (Union across ALL tracks so optional fields aren't lost.)
  function resolveColumns(tracks: any[]): string[] {
    const cols = [...BASE_COLUMNS];
    for (const t of tracks) {
      if (t && typeof t === 'object') {
        for (const key of Object.keys(t)) {
          if (!cols.includes(key)) cols.push(key);
        }
      }
    }
    return cols;
  }

  async function handleExport() {
    if (exporting) return;

    exporting = true;
    errorMessage = '';
    statusMessage = 'Checking server connection...';
    lastExportSummary = '';

    try {
      // 0. Connectivity gate FIRST (before clearing anything, so a failed
      //    gate doesn't needlessly nuke the app's warm cache). Proves the
      //    Go server is reachable — admin tools are online-only by design.
      const connected = await verifyConnectivity();
      if (!connected) {
        throw new Error(
          'Server unreachable — cannot verify the catalog is current. ' +
          'Export aborted (admin tools are online-only by design).'
        );
      }

      statusMessage = 'Verifying catalog against server...';

      // 1. FORCE a genuine server check. Without this, getCatalog() returns
      //    the warm in-memory copy instantly (its branch 1) with NO ETag
      //    round-trip — possibly stale (edited since it loaded). Clearing
      //    the memory pointer makes getCatalog fall through to
      //    loadCatalogInternal(), which performs the blocking ETag check:
      //      304 → local copy verified current
      //      200 → fresh bytes downloaded from R2 via the server
      //    getCatalog then repopulates window.__LABRI_CATALOG__, so the
      //    whole app is left holding fresh data (self-healing).
      clearCatalogMemoryCache();

      const tracks = await getCatalog();
      if (!tracks || tracks.length === 0) {
        throw new Error('The catalog is empty — nothing to export.');
      }

      statusMessage = 'Building CSV...';

      // 2. Columns, then rows: header, then one line per track.
      const columns = resolveColumns(tracks);
      const lines: string[] = [columns.map(csvEscape).join(',')];
      for (const track of tracks) {
        lines.push(
          columns
            .map(col => csvEscape(track && typeof track === 'object' ? (track as any)[col] : ''))
            .join(',')
        );
      }
      // BOM first so Excel detects UTF-8; CRLF joins per RFC 4180.
      const csv = '\uFEFF' + lines.join('\r\n') + '\r\n';

      // 3. Download: Blob → objectURL → anchor.click() → revoke. The
      //    click stays synchronous inside the user gesture.
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `catalog-export-${new Date().toISOString().slice(0, 10)}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      lastExportSummary = `${tracks.length} tracks, ${columns.length} columns exported.`;
      statusMessage = 'Export complete — file saved to your Downloads folder.';
    } catch (err) {
      console.error('Catalog export failed:', err);
      statusMessage = '';
      errorMessage = err instanceof Error ? err.message : 'Export failed.';
    } finally {
      exporting = false;
    }
  }
</script>


<div class="w-full max-w-2xl">
  <div class="bg-slate-800 p-6 rounded-lg border border-cyan-500/50 shadow-xl">
    <h2 class="text-lg font-bold text-cyan-400 mb-2">Export Catalog to CSV</h2>
    <p class="text-slate-300 text-sm mb-6">
      Downloads every track's metadata as a CSV file. The catalog is checked
      against the server first, so the export always reflects the latest
      published state.
    </p>

    <button
      onclick={handleExport}
      disabled={exporting}
      class="w-full bg-cyan-500 hover:bg-cyan-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
      {#if exporting}
        Exporting...
      {:else}
        Export Catalog (CSV)
      {/if}
    </button>

    {#if statusMessage}
      <p class="mt-4 text-center text-sm text-emerald-400">{statusMessage}</p>
    {/if}

    {#if lastExportSummary}
      <p class="mt-1 text-center text-xs text-slate-400">{lastExportSummary}</p>
    {/if}

    {#if errorMessage}
      <p class="mt-4 text-center text-sm font-medium text-red-400">❌ {errorMessage}</p>
    {/if}
  </div>
</div>
