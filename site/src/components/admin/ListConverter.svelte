<script>
  let inputText = "";
  let status = "";
  let downloadUrl = "";
  let trackCount = 0;

  async function generateCompressedCatalog() {
    if (!inputText.trim()) {
      status = "Please paste a list first.";
      return;
    }

    status = "Parsing list...";
    
    try {
      // 1. Parse the comma-delimited list
      const rows = inputText.split('\n').filter(line => line.trim() !== '');
      const tracks = [];
      const lines = rows.slice(1);                // Remove the header line
      for (let i = 0; i < lines.length; i++) {
        const parts = lines[i].split(',').map(p => p.trim());
        
        // --- ADJUST THIS LOGIC BASED ON YOUR SPECIFIC CSV FORMAT ---
        // Assuming format: Title, Artist, Filename
        tracks.push({
          id: `track-${i + 1}`,
          title: parts[0] || 'Unknown Title',
          artist: parts[1] || 'Unknown Artist',
          filename: parts[2] || `${parts[0] || 'file'}.mp3`
        });
        // -----------------------------------------------------------
      }

      // 2. Build the new Catalog structure
      const catalog = {
        version: `web-import-${Date.now()}`,
        count: tracks.length,
        tracks: tracks
      };

      trackCount = tracks.length;
      status = "Compressing to Gzip...";

      // 3. Convert Catalog Object to JSON String
      const jsonString = JSON.stringify(catalog, null, 2);

      // 4. Compress using native Browser API (The exact opposite of our Inflate function!)
      const blob = new Blob([jsonString]);
      const ds = new CompressionStream('gzip');
      const compressedStream = blob.stream().pipeThrough(ds);
      const compressedBlob = await new Response(compressedStream).blob();

      // 5. Create a Download Link for the .gz file
      if (downloadUrl) URL.revokeObjectURL(downloadUrl); // Clean up old link
      
      downloadUrl = URL.createObjectURL(compressedBlob);
      status = `✅ Generated ${trackCount} tracks. Ready to download and then for you to upload to R2!`;

    } catch (error) {
      console.error("Generation error:", error);
      status = `❌ Error: ${error.message}`;
    }
  }
</script>

<div class="text-white p-6 bg-slate-800 rounded-lg border border-slate-700 shadow-xl max-w-2xl mx-auto">
  <h2 class="text-2xl font-bold text-cyan-400 mb-4">Convert List to Compressed Catalog.json</h2>
  <p class="text-slate-300 text-sm mb-4">Paste your comma-delimited list below (e.g., Title, Artist, Filename). It will generate a <code class="bg-slate-700 px-1 rounded">catalog.json.gz</code> file ready for R2.</p>

  <textarea 
    bind:value={inputText}
    rows="8"
    class="w-full bg-slate-900 border border-slate-600 rounded p-3 text-white font-mono text-sm focus:outline-none focus:border-cyan-400 mb-4"
    placeholder="Song 1, Artist 1, song1.mp3&#10;Song 2, Artist 2, song2.mp3"
  ></textarea>

  <div class="flex items-center gap-4">
    <button 
      on:click={generateCompressedCatalog}
      class="bg-cyan-500 hover:bg-cyan-400 text-slate-900 font-bold py-2 px-6 rounded transition-colors"
      >
      Generate & Compress
    </button>

    {#if downloadUrl}
      <a 
        href={downloadUrl}
        download="catalog.json.gz"
        class="bg-emerald-500 hover:bg-emerald-400 text-slate-900 font-bold py-2 px-6 rounded transition-colors no-underline"
      >
        ⬇️ Download .gz
      </a>
    {/if}
  </div>

  {#if status}
    <p class="mt-4 text-sm text-slate-300">{status}</p>
  {/if}
</div>
