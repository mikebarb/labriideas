<script>
  let fileName = "text.txt"; // Default value for easy testing
  let status = "";
  let isLoading = false;

  async function handleDownload() {
    if (!fileName.trim()) {
      status = "Please enter a file name.";
      return;
    }

    isLoading = true;
    status = "Requesting signed URL from Go server...";

    try {
      // 1. Ask our Go API for the signed URL
      const response = await fetch(`http://localhost:8080/api/download?file=${encodeURIComponent(fileName)}`);
      
      if (!response.ok) {
        throw new Error(`Server error: ${response.status}`);
      }

      const data = await response.json();

      // 2. If we got the URL, trigger the download
      if (data.url) {
        status = "Signed URL received! Starting download...";
        
        // Create a hidden <a> tag to force the browser to download the file
        const a = document.createElement('a');
        a.href = data.url;
        a.download = fileName; // Suggests a filename to the browser
        document.body.appendChild(a);
        a.click();
        
        // Clean up the hidden tag
        document.body.removeChild(a);
        
        status = "Download initiated! Check youe browser's download bar.";
      } else {
        throw new Error("No URL returned from server.");
      }
    } catch (error) {
      console.error(error);
      status = `Error: ${error.message}`;
    } finally {
      isLoading = false;
    }
  }
</script>

<div class="bg-slate-800 p-6 rounded-lg border border-slate-700 shadow-xl max-w-md mx-auto">
  <h2 class="text-2xl font-bold text-cyan-400 mb-6 text-center">R2 Downloader</h2>
  
  <div class="flex flex-col gap-4">
    <input 
      type="text" 
      bind:value={fileName} 
      class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-white focus:outline-none focus:border-cyan-400" 
      placeholder="Enter file name (e.g., song.mp3)"
    />

    <button 
      on:click={handleDownload}
      disabled={isLoading}
      class="bg-cyan-500 hover:bg-cyan-400 disabled:opacity-50 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors"
    >
      {isLoading ? 'Working...' : '⬇️ Download File'}
    </button>

    {#if status}
      <p class="text-sm text-slate-300 text-center mt-2">{status}</p>
    {/if}
  </div>
</div>
