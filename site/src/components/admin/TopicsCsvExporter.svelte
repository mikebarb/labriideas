<!-- src/components/admin/TopicsCsvExporter.svelte -->
<script lang="ts">
  // Admin tool: exports the Topics hierarchy from the active menu state
  // ($menuData) into a flattened, Excel-compatible CSV file.
  //
  // Flattening rules:
  //   - MajorTheme and SubCategory values are repeated down the rows
  //     for every leaf lecture item they contain.
  //   - Columns: MajorTheme, SubCategory, Subtopic, AltName, Category.
  //   - Excel compatibility: UTF-8 BOM prepended, string fields wrapped
  //     in double quotes with internal quotes escaped.
  // Row structure:
  //   - featured row: ItemType='featured' with FeaturedFilename, FeaturedSpeaker, FeaturedTitle populated.
  //   - lectures row: ItemType='lectures' with AltName, Category, Subtopic populated.

  import { menuData } from '../../lib/menuDataStore';

  let exporting = $state(false);
  let exportMessage = $state('');

  // Helper to safely escape CSV field values (wraps in quotes, escapes internal quotes)
  function escapeCsvField(val: unknown): string {
    if (val === null || val === undefined) return '""';
    const str = String(val);
    return `"${str.replace(/"/g, '""')}"`;
  }

  async function handleExportTopics() {
    exporting = true;
    exportMessage = '';

    try {
      // 1. Read active menu data from store
      let currentMenu: any;
      const unsubscribe = menuData.subscribe((val: any) => {
        currentMenu = val;
      });
      unsubscribe();

      const hierarchy = currentMenu?.Topics?.hierarchy;
      if (!hierarchy || typeof hierarchy !== 'object') {
        throw new Error('No Topics hierarchy found in the current menu state.');
      }

      // 2. Flatten the 3D hierarchy into 2D rows
      const rows: Array<{
        MajorTheme: string;
        SubCategory: string;
        ItemType: string;
        AltName: string;
        Category: string;
        Subtopic: string;
        FeaturedFilename: string;
        FeaturedSpeaker: string;
        FeaturedTitle: string;
      }> = [];

      for (const [majorTheme, subCategories] of Object.entries(hierarchy)) {
        if (!subCategories || typeof subCategories !== 'object') continue;

        for (const [subCategoryName, node] of Object.entries(subCategories as Record<string, any>)) {
          if (subCategoryName === 'featured') continue;

          // Normalize node shape (handles both legacy bare arrays and v2 {lectures, featured?} nodes)
          const nodeObj = Array.isArray(node) ? { lectures: node, featured: [] } : node;
          const lectures: any[] = nodeObj?.lectures ?? [];
          const featured: any[] = nodeObj?.featured ?? [];

          // 1. Export lectures rows
          for (const lecture of lectures) {
            rows.push({
              MajorTheme: majorTheme,
              SubCategory: subCategoryName,
              ItemType: 'lectures',
              AltName: lecture.altName ?? '',
              Category: lecture.category ?? '',
              Subtopic: lecture.subtopic ?? '',
              FeaturedFilename: '',
              FeaturedSpeaker: '',
              FeaturedTitle: ''
            });
          }

          // 2. Export featured rows
          for (const feat of featured) {
            rows.push({
              MajorTheme: majorTheme,
              SubCategory: subCategoryName,
              ItemType: 'featured',
              AltName: '',
              Category: '',
              Subtopic: '',
              FeaturedFilename: feat.filename ?? '',
              FeaturedSpeaker: feat.speaker ?? '',
              FeaturedTitle: feat.title ?? ''
            });
          }
        }
      }

      if (rows.length === 0) {
        throw new Error('The Topics hierarchy contains no items to export.');
      }

      const headers = [
        'MajorTheme',
        'SubCategory',
        'ItemType',
        'AltName',
        'Category',
        'Subtopic',
        'FeaturedFilename',
        'FeaturedSpeaker',
        'FeaturedTitle'
      ];
      
      const csvLines: string[] = [];

      // Add header row
      csvLines.push(headers.map(escapeCsvField).join(','));

      // Add data rows
      for (const row of rows) {
        csvLines.push([
          escapeCsvField(row.MajorTheme),
          escapeCsvField(row.SubCategory),
          escapeCsvField(row.ItemType),
          escapeCsvField(row.AltName),
          escapeCsvField(row.Category),
          escapeCsvField(row.Subtopic),
          escapeCsvField(row.FeaturedFilename),
          escapeCsvField(row.FeaturedSpeaker),
          escapeCsvField(row.FeaturedTitle)
        ].join(','));
      }

      // Prepend UTF-8 BOM (\uFEFF) so Excel automatically recognizes character encoding
      const csvContent = '\uFEFF' + csvLines.join('\r\n');

      // 4. Trigger file save (FileSystem Access API with fallback)
      const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
      const filename = `topics-export-${new Date().toISOString().slice(0, 10)}.csv`;

      if ('showSaveFilePicker' in window) {
        try {
          const handle = await (window as any).showSaveFilePicker({
            suggestedName: filename,
            types: [{
              description: 'CSV Files',
              accept: { 'text/csv': ['.csv'] }
            }]
          });
          const writable = await handle.createWritable();
          await writable.write(blob);
          await writable.close();
          exportMessage = `✅ Successfully exported ${rows.length} rows (lectures & featured).`;
          exporting = false;
          return;
        } catch (pickerErr: any) {
          // User cancelled the picker — do nothing, abort cleanly
          if (pickerErr.name === 'AbortError') {
            exporting = false;
            return;
          }
          // Fall back to standard download if picker failed for another reason
        }
      }

      // Fallback download method
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      exportMessage = `✅ Successfully exported ${rows.length} rows (lectures & featured).`;
    } catch (err) {
      console.error('Topics export failed:', err);
      exportMessage = `❌ Export failed: ${(err as Error).message}`;
    } finally {
    exporting = false;
    }
  }
</script>

<div class="bg-white border border-slate-200 rounded-lg p-4 shadow-xs flex items-center justify-between gap-4">
  <div>
    <h3 class="font-bold text-slate-800 text-sm">Export Topics to CSV</h3>
    <p class="text-xs text-slate-500">
      Download Topics (lectures and featured items) as a flat spreadsheet for Excel.
    </p>
    {#if exportMessage}
      <p class="text-xs mt-1 font-medium text-slate-700">{exportMessage}</p>
    {/if}
  </div>
  <button
    type="button"
    onclick={handleExportTopics}
    disabled={exporting}
    class="bg-slate-800 hover:bg-slate-900 text-white text-sm font-semibold px-4 py-2 rounded transition-colors disabled:opacity-50 shrink-0"
  >
    {exporting ? 'Exporting...' : 'Export Topics CSV'}
  </button>
</div>
