<script>
  import SparkMD5 from 'spark-md5';

  export let catalog = { tracks: [] }; // Passed in from parent

  let selectedFile = null;
  let fileHash = "";
  let isCalculatingHash = false;
  let duplicateWarning = "";

  let metadata = {};
  let isUploading = false;
  let uploadProgress = 0;
  let statusMessage = "";

  // 1. Handle File Selection & Calculate MD5
  async function handleFileChange(event) {
    selectedFile = event.target.files[0];
    if (!selectedFile) return;

    duplicateWarning = "";
    statusMessage = "Calculating file hash...";
    isCalculatingHash = true;

    // Calculate MD5 using SparkMD5
    fileHash = await calculateMD5(selectedFile);

    isCalculatingHash = false;
    statusMessage = "";

    // 2. Duplicate Check (Filename & Hash)
    const isDuplicate = catalog.tracks.some(track => 
      track.filename === selectedFile.name || track.hash === fileHash
    );

    if (isDuplicate) {
      duplicateWarning = "⚠️ Warning: A track with this filename or hash already exists in the catalog!";
    }

    // 3. Pre-fill dynamic metadata form
    metadata = {
      id: selectedFile.name,
      filename: selectedFile.name,
      hash: fileHash,
      title: selectedFile.name.replace(/\.[^/.]+$/, ""), // strip extension for default title
      artist: ""
    };
  }

  function calculateMD5(file) {
    return new Promise((resolve) => {
      const blobSlice = File.prototype.slice || File.prototype.mozSlice || File.prototype.webkitSlice;
      const chunkSize = 2097152; // Read in 2MB chunks
      const spark = new SparkMD5.ArrayBuffer();
      const fileReader = new FileReader();
      let currentChunk = 0;

      fileReader.onload = (e) => {
        spark.append(e.target.result);
        currentChunk++;
        if (currentChunk * chunkSize < file.size) {
          loadNext();
        } else {
          resolve(spark.end());
        }
      };

      function loadNext() {
        const start = currentChunk * chunkSize;
        const end = ((start + chunkSize) >= file.size) ? file.size : start + chunkSize;
        fileReader.readAsArrayBuffer(blobSlice.call(file, start, end));
      }

      loadNext();
    });
  }

  // 4. Handle Submit & Upload with Progress
  async function handleUpload() {
    if (!selectedFile || duplicateWarning) return;

    isUploading = true;
    uploadProgress = 0;
    statusMessage = "Uploading to R2...";

    // Prepare metadata payload (strip system fields)
    const metadataPayload = { ...metadata };
    delete metadataPayload.id;
    delete metadataPayload.filename;
    delete metadataPayload.hash;

    const formData = new FormData();
    formData.append("audioFile", selectedFile);
    formData.append("metadata", JSON.stringify({
      filename: metadata.filename,
      metadata: metadataPayload
    }));

    // Use XMLHttpRequest for upload progress tracking
    const xhr = new XMLHttpRequest();
    
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) {
        uploadProgress = Math.round((e.loaded / e.total) * 100);
      }
    });

    xhr.onload = () => {
      if (xhr.status === 200) {
        statusMessage = "✅ Upload complete! Catalog updated.";
      } else {
        statusMessage = "❌ Upload failed.";
      }
      isUploading = false;
    };

    xhr.onerror = () => {
      statusMessage = "❌ Network error during upload.";
      isUploading = false;
    };

    xhr.open("POST", "http://localhost:8080/api/upload-track");
    xhr.send(formData);
  }
</script>

<div class="bg-slate-800 p-6 rounded-lg border border-slate-700 shadow-xl">
  <h3 class="text-xl font-bold text-cyan-400 mb-4">Upload New Track</h3>

  <!-- 1. File Input -->
  <div class="mb-4">
    <input type="file" accept="audio/mpeg" on:change={handleFileChange} class="block w-full text-sm text-slate-400 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-semibold file:bg-cyan-500 file:text-slate-900 hover:file:bg-cyan-400 cursor-pointer"/>
  </div>

  {#if isCalculatingHash}
    <p class="text-yellow-400 text-sm mb-4">{statusMessage}</p>
  {/if}

  {#if duplicateWarning}
    <div class="bg-red-900/50 border border-red-500 text-red-300 p-3 rounded mb-4 text-sm">
      {duplicateWarning}
    </div>
  {/if}

  <!-- 3. Dynamic Metadata Form -->
  {#if metadata.filename && !duplicateWarning}
    <div class="grid gap-4 mb-6 bg-slate-900 p-4 rounded">
      {#each Object.entries(metadata) as [key, value]}
        {#if key !== 'id' && key !== 'hash'}
          <label class="block">
            <span class="text-xs text-slate-400 uppercase">{key}</span>
            <input 
              type="text" 
              bind:value={metadata[key]} 
              class="w-full bg-slate-800 border border-slate-600 rounded p-2 text-white"
            />
          </label>
        {/if}
      {/each}
    </div>

    <!-- 4. Upload Button & Progress -->
    <button 
      on:click={handleUpload} 
      disabled={isUploading}
      class="w-full bg-cyan-500 hover:bg-cyan-400 disabled:opacity-50 text-slate-900 font-bold py-2 px-4 rounded transition-colors">
      {isUploading ? `Uploading... ${uploadProgress}%` : '🚀 Upload Track'}
    </button>

    {#if isUploading}
      <div class="w-full bg-slate-700 rounded-full h-2.5 mt-4">
        <div class="bg-cyan-500 h-2.5 rounded-full transition-all duration-200" style="width: {uploadProgress}%"></div>
      </div>
    {/if}

    {#if statusMessage}
      <p class="mt-4 text-center text-sm font-medium text-cyan-400">{statusMessage}</p>
    {/if}
  {/if}
</div>
