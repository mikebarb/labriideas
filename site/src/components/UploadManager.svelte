<script>
  import SparkMD5 from 'spark-md5';
  import { onMount } from 'svelte';

  export let catalog = { tracks: [] }; // Passed in from parent

  // Make sure catalog is initialized
  //let catalog = { tracks: [] }; 
  let isCatalogLoading = true; // NEW: Start as true

  let selectedFile = null;
  let fileAudioHash = "";
  let isCalculatingHash = false;
  let duplicateWarning = "";

  let metadata = {};
  let isUploading = false;
  let uploadProgress = 0;
  let statusMessage = "";

  // Fetch the catalog as soon as the component loads
  onMount(async () => {
    try {
      const response = await fetch('http://localhost:8080/api/catalog');
      if (!response.ok) throw new Error('Failed to fetch catalog');
      
      // If your server returns gzip, the browser handles it automatically here
      catalog = await response.json(); 
      console.log("Catalog loaded with", catalog.tracks.length, "tracks.");
      } catch (error) {
      console.error("Error fetching catalog:", error);
      duplicateWarning = "⚠️ Could not load existing catalog. Duplicate checks are disabled.";
    } finally {
      isCatalogLoading = false; // Mark as loaded (even if it failed)
    }
  });

  // 1. Handle File Selection & Calculate MD5
  async function handleFileChange(event) {
    console.log("File input changed");
    selectedFile = event.target.files[0];
    if (!selectedFile) return;

    console.log("Selected file:", selectedFile.name);

    duplicateWarning = "";
    let isDuplicate = false;
     try {
      statusMessage = "Calculating file hash...";
      isCalculatingHash = true;

      // Calculate MD5 using SparkMD5
      fileAudioHash = await calculateAudioHash(selectedFile);

      isCalculatingHash = false;
      statusMessage = "";

      // 2. Duplicate Check (Filename & Hash)
      //    Check if catalog is loaded first to prevent errors
      
      // Inform the user if the catalog isn't ready yet!
      if (isCatalogLoading) {
        duplicateWarning = "⏳ The catalog is still loading. Duplicate check skipped.";
      } else if (catalog.tracks && catalog.tracks.length > 0) {
        //isDuplicate = catalog.tracks.some(track => 
        //  track.filename === selectedFile.name || track['audio-hash'] === fileAudioHash
        //);

        // Check for exact filename match first
        const matchedByFilename = catalog.tracks.find(track => track.filename === selectedFile.name);
        
        // Check for audio content match
        let matchedByHash = null;
        if (!matchedByFilename && fileAudioHash !== "error") {
          matchedByHash = catalog.tracks.find(track => track['audio-hash'] === fileAudioHash);
        }

        // Provide specific, actionable feedback
        if (matchedByFilename) {
          duplicateWarning = `⚠️ Warning: A file named "${selectedFile.name}" already exists in the catalog!`;
        } else if (matchedByHash) {
          duplicateWarning = `⚠️ Warning: The audio content of this file is identical to an existing track: "${matchedByHash.filename}"!`;
        }






        console.log("Duplicate check:", isDuplicate);
      }
      if (isDuplicate) {
        duplicateWarning = "⚠️ Warning: A track with this filename or hash already exists in the catalog!";
      }

    } catch (error) {
      console.error("❌ ERROR during file processing:", error);
      isCalculatingHash = false;
      statusMessage = "";
      duplicateWarning = "⚠️ Could not calculate audio hash. Duplicate check skipped.";
      fileAudioHash = "error"; // Set a fallback so metadata still populates
    
    } finally {
       // ALWAYS reset the loading state
      isCalculatingHash = false;
      //duplicateWarning = ""; // clear for testing.

      // 3. Pre-fill dynamic metadata form (Guaranteed to run!)
      metadata = {
        id: selectedFile.name,
        filename: selectedFile.name,
        hash: fileAudioHash || "unknown",
        title: selectedFile.name.replace(/\.[^/.]+$/, ""), // strip extension for default title
        artist: ""
      };
    }
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

  async function calculateAudioHash(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = function(e) {
        const buffer = e.target.result;
        const view = new Uint8Array(buffer);
        
        let startOffset = 0;
        let endOffset = buffer.byteLength;

        // 1. Skip ID3v2 header (Front)
        if (String.fromCharCode(view[0], view[1], view[2]) === 'ID3') {
          // Calculate synchsafe integer size
          const size = (view[6] << 21) | (view[7] << 14) | (view[8] << 7) | view[9];
          startOffset = size + 10;
        }

        // 2. Skip ID3v1 tag (Back - 128 bytes)
        if (buffer.byteLength > 128) {
          const tagStart = buffer.byteLength - 128;
          if (String.fromCharCode(view[tagStart], view[tagStart+1], view[tagStart+2]) === 'TAG') {
            endOffset = buffer.byteLength - 128;
          }
        }

        // 3. Extract only the audio slice
        const audioBuffer = buffer.slice(startOffset, endOffset);

        // 4. Hash the audio slice using SparkMD5
        const spark = new SparkMD5.ArrayBuffer();
        spark.append(audioBuffer);
        const hash = spark.end();
        
        resolve(hash);
      };
      reader.onerror = reject;
      reader.readAsArrayBuffer(file);
    });
  }
  
  // Handle Submit & Upload with Progress
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
    <input 
      type="file" 
      accept="audio/mpeg"
      on:change={handleFileChange}
      disabled={isCatalogLoading} 
      class="block w-full text-sm text-slate-400 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-semibold file:bg-cyan-500 file:text-slate-900 hover:file:bg-cyan-400 cursor-pointer"
    />
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
    <!-- Make filename read-only so it can be viewed/copied but not changed -->
    <!-- Apply locked styling only to the filename -->
     <!-- class="w-full bg-slate-800 border border-slate-600 rounded p-2 text-white" -->
   {#if metadata.filename}
    <div class="grid gap-4 mb-6 bg-slate-900 p-4 rounded">
      {#each Object.entries(metadata) as [key, value]}
        {#if key !== 'id' && key !== 'hash'}
          <label class="block">
            <span class="text-xs text-slate-400 uppercase">{key}</span>
            <input 
              type="text" 
              bind:value={metadata[key]} 
                readonly={key === 'filename'} 
                class="w-full border rounded p-2 {key === 'filename' 
                  ? 'bg-slate-950 border-slate-700 text-slate-400 cursor-default' 
                  : 'bg-slate-800 border-slate-600 text-white'}"
            />
          </label>
        {/if}
      {/each}
    </div>

    <!-- 4. Upload Button & Progress -->
    <button 
      on:click={handleUpload} 
      disabled={isUploading || isCatalogLoading || duplicateWarning}
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
