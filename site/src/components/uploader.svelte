<script>
  let file = null;
  let title = "";
  let artist = "";
  let status = "";
  let isLoading = false;

  function handleFileChange(event) {
    file = event.target.files[0];
    if (file) {
      // Auto-fill title from filename if empty
      if (!title) title = file.name.replace(/\.[^/.]+$/, "");
    }
  }

  async function handleUpload() {
    if (!file) {
      status = "Please select a file first.";
      return;
    }

    isLoading = true;
    status = "Uploading to Go server...";

    // 1. Build standard Multi-part Form Data
    const formData = new FormData();
    formData.append("file", file);
    formData.append("title", title);
    formData.append("artist", artist);

    try {
      // 2. Send to our Go API
      const response = await fetch("http://localhost:8080/api/upload", {
        method: "POST",
        body: formData, // Fetch automatically sets the correct headers for FormData!
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || "Upload failed");
      }

      status = `✅ Successfully uploaded ${data.file}!`;
      file = null;
      title = "";
      artist = "";
      
    } catch (error) {
      console.error(error);
      status = `❌ Error: ${error.message}`;
    } finally {
      isLoading = false;
    }
  }
</script>

<div class="bg-slate-800 p-6 rounded-lg border border-slate-700 shadow-xl max-w-md mx-auto">
  <h2 class="text-2xl font-bold text-cyan-400 mb-6 text-center">R2 Uploader</h2>
  
  <div class="flex flex-col gap-4">
    <input 
      type="text" 
      bind:value={title} 
      class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-white focus:outline-none focus:border-cyan-400" 
      placeholder="Song Title"
    />
    
    <input 
      type="text" 
      bind:value={artist} 
      class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-white focus:outline-none focus:border-cyan-400" 
      placeholder="Artist Name"
    />

    <input 
      type="file" 
      accept="audio/*" 
      on:change={handleFileChange} 
      class="text-slate-300 text-sm file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-semibold file:bg-cyan-500 file:text-slate-900 hover:file:bg-cyan-400"
    />

    <button 
      on:click={handleUpload}
      disabled={isLoading}
      class="bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors"
    >
      {isLoading ? 'Working...' : '⬆️ Upload File'}
    </button>

    {#if status}
      <p class="text-sm text-slate-300 text-center mt-2">{status}</p>
    {/if}
  </div>
</div>
