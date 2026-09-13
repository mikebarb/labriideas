<script>
  import SparkMD5 from 'spark-md5';
  import { onMount, onDestroy } from 'svelte';
  import MetadataForm from '../MetadataForm.svelte';
  import { getCatalog, clearCatalogMemoryCache } from '../../lib/catalogStore';
  // NEW: authenticated transport. /api/upload-track is behind authMiddleware,
  // so the upload MUST carry the Bearer token or it 401s at the gate.
  import { authClient } from '../../lib/authClient';

  // Catalog is internal state (the .astro page passes nothing in) —
  // loaded via the store on mount, used for duplicate checks and as the
  // metadata-form template.
  let catalog = $state({ tracks: [] });
  let isCatalogLoading = $state(true);

  let fileInput; // Reference to the HTML file input

  let selectedFile = $state(null);
  let fileAudioHash = $state("");
  let isCalculatingHash = $state(false);
  let duplicateWarning = $state("");

  let metadata = $state({});
  let isUploading = $state(false);
  let uploadProgress = $state(0);
  let statusMessage = $state("");

  let previewUrl = $state(""); // Stores the temporary local blob URL
  let previewTrack = $state(null); // Stores the temporary track object for the player

  // Fetch the catalog as soon as the component loads
  onMount(async () => {
    try {
      // Use the store! It handles GZIP, Memory Cache, LocalStorage, and ETags.
      const tracksArray = await getCatalog(); 
      catalog = { tracks: tracksArray }; 
      console.log("Catalog loaded with", catalog.tracks.length, "tracks.");
    } catch (error) {
      console.error("Error fetching catalog:", error);
      duplicateWarning = "⚠️ Could not load existing catalog. Duplicate checks are disabled.";
    } finally {
      isCatalogLoading = false;
    }
  });

  // On destroy, stop audio and revoke the URL so you don't leak memory
  onDestroy(() => {
    if (previewTrack) {
      window.dispatchEvent(new CustomEvent('preview-discarded', {
        detail: { filename: previewTrack.filename }
      }));
    }
    if (previewUrl) URL.revokeObjectURL(previewUrl);
  });

  // Helper to generate a blank metadata object with ALL schema keys
  function generateBlankMetadata(filename, audioHash) {
    const blank = {
      filename: filename,
      'audio-hash': audioHash,
      title: filename.replace(/\.[^/.]+$/, ""),
      artist: ""
    };

    // If catalog is loaded, copy its keys to ensure we show ALL possible fields
    if (catalog.tracks && catalog.tracks.length > 0) {
      const templateTrack = catalog.tracks[0];
      for (const key of Object.keys(templateTrack)) {
        if (!(key in blank)) {
          blank[key] = ""; // Initialize missing schema fields as empty
        }
      }
    }
    
    // Remove internal system fields we don't want in the form at all
    delete blank.id;
    delete blank.hash; 

    return blank;
  }

  // Reset the uploader to its initial state
  function handleCancelUpload() {
    // Stop the preview BEFORE revoking: an already-loaded element keeps
    // playing a revoked blob (the player handles the explicit stop).
    if (previewTrack) {
      window.dispatchEvent(new CustomEvent('preview-discarded', {
        detail: { filename: previewTrack.filename }
      }));
    }

    // Release the preview blob URL BEFORE clearing state. If a preview is
    // playing, its source dies here and playback stops — intended: the
    // user has discarded the file. (Player will show its generic error
    // state briefly; the next play action replaces it.)
    if (previewUrl) URL.revokeObjectURL(previewUrl);
    previewUrl = "";
    previewTrack = null;

    selectedFile = null;
    metadata = {};
    duplicateWarning = "";
    statusMessage = "";
    isCalculatingHash = false;

    // Reset the native HTML file input so it's ready for a new selection
    if (fileInput) {
      fileInput.value = '';
    }
  }

  // 1. Handle File Selection & Calculate MD5
  async function handleFileChange(event) {
    console.log("File input changed");
    selectedFile = event.target.files[0];
    if (!selectedFile) return;

    console.log("Selected file:", selectedFile.name);

    duplicateWarning = "";
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
              }
    } catch (error) {
      console.error("❌ ERROR during file processing:", error);
      statusMessage = "";
      duplicateWarning = "⚠️ Could not calculate audio hash. Duplicate check skipped.";
      fileAudioHash = "error"; // Set a fallback so metadata still populates
    
    } finally {
       // ALWAYS reset the loading state
      isCalculatingHash = false;

      // 3. Pre-fill dynamic metadata form (Guaranteed to run!)
       metadata = generateBlankMetadata(selectedFile.name, fileAudioHash || "unknown");

      // Create a temporary Blob URL for the local preview.
      // References the in-memory File directly — zero-copy, instant,
      // seekable. Revoking the previous URL (if a preview was already
      // playing) stops that audio: the blob source dies, Player's error
      // listener shows the generic playback-error state. That is the
      // CORRECT outcome — the form has moved on to a new file.
      if (previewUrl) URL.revokeObjectURL(previewUrl); // Clean up old one if exists
      previewUrl = URL.createObjectURL(selectedFile);

      // Tell the player to stop the OLD preview before we kill its URL.
      // Revocation alone doesn't stop an already-loaded element (see
      // Player's preview-discarded handler), so the stop is explicit.
      if (previewTrack) {
        window.dispatchEvent(new CustomEvent('preview-discarded', {
          detail: { filename: previewTrack.filename }
        }));
      }
      if (previewUrl) URL.revokeObjectURL(previewUrl); // Clean up old one if exists
      previewUrl = URL.createObjectURL(selectedFile);

      // Construct the Virtual Track for the global player's preview.
      //
      // CONTRACT (see Player.loadTrack Tier 0 + the preview design doc):
      //   - filename: 'preview-' prefix. Player's playTrack() dispatches
      //     on filename equality (CASE 1 toggle / CASE 2 queue-hit); the
      //     prefix can never collide with a catalog or queued track, so
      //     the preview always lands in CASE 3 (streaming detour) —
      //     queue untouched, bookmark untouched, nothing persisted.
      //   - localBlob: the audio source. Tier 0 consumes it and returns
      //     before any OPFS/presigned logic runs.
      //   - NO hash: both OPFS tiers and the background cache are
      //     `if (track.hash)` gated — an unuploaded file must never
      //     enter the offline cache under any hash.
      //   - NO position/playbackRate: nothing to resume.
      //   - UploadManager owns the URL lifecycle: revoke on new-file
      //     select, cancel, or unmount (onDestroy). NOT on upload
      //     success — the File stays valid and the admin may keep
      //     listening while preparing the next upload.
      previewTrack = {
        filename: `preview-${selectedFile.name}`,
        title: metadata.title || selectedFile.name,
        speaker: metadata.artist || 'Local preview',
        localBlob: previewUrl
      };

      // Create a temporary Blob URL for local playback
      //if (previewUrl) URL.revokeObjectURL(previewUrl); // Clean up old one if exists
      //previewUrl = URL.createObjectURL(selectedFile);

      // Construct a temporary Track object to send to the player
      //previewTrack = {
      //  id: metadata.filename,
      //  filename: metadata.filename,
      //  hash: fileAudioHash || "unknown",
      //  title: metadata.title,
      //  artist: metadata.artist,
      //  metadata: metadata,
      //  localPreviewUrl: previewUrl // <-- The magic key!
      //};
    }
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
        resolve(spark.end());
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
    // No need to delete id/hash as generateBlankMetadata already removed them

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
         // This forces the store to re-fetch the fresh catalog from the server
        clearCatalogMemoryCache(); 
      } else if (xhr.status === 401) {
        // NEW: the session died mid-work (TTL expiry or server restart).
        // Clear the token so the guard redirects on next navigation, and
        // tell the admin plainly — a generic "upload failed" here would
        // send them debugging their file instead of their session.
        authClient.clearToken();
        statusMessage = "🔒 Session expired — sign in again. Your metadata is still filled in; re-login then press Upload.";
      } else {
        statusMessage = `❌ Upload failed (server returned ${xhr.status}).`;
      }
      isUploading = false;
    };

    xhr.onerror = () => {
      statusMessage = "❌ Network error during upload.";
      isUploading = false;
    };

    // NEW: open() first, then set headers — setRequestHeader throws if
    // called before the connection is opened. The Bearer token here is
    // what carries the request through authMiddleware.
    xhr.open("POST", `${import.meta.env.PUBLIC_API_BASE_URL}/api/upload-track`);
    for (const [key, value] of Object.entries(authClient.authHeader())) {
      xhr.setRequestHeader(key, value);
    }
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
      onchange={handleFileChange}
      disabled={isCatalogLoading}
      bind:this={fileInput}
      class="block w-full text-sm text-slate-400 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-semibold file:bg-cyan-500 file:text-slate-900 hover:file:bg-cyan-400 cursor-pointer"
    />
  </div>
   <!-- Preview Button -->
  {#if previewTrack}
    <div class="mb-4">
      <button 
        onclick={() => window.dispatchEvent(new CustomEvent('play-track', { detail: previewTrack }))}
        class="bg-emerald-500 hover:bg-emerald-400 px-4 py-2 rounded text-slate-900 font-bold text-sm transition-colors">
        ▶ Preview Audio Locally
      </button>
    </div>
  {/if}

  {#if isCalculatingHash}
    <p class="text-yellow-400 text-sm mb-4">{statusMessage}</p>
  {/if}

  {#if duplicateWarning}
    <div class="bg-red-900/50 border border-red-500 text-red-300 p-3 rounded mb-4 text-sm">
      {duplicateWarning}
    </div>
  {/if}

  <!-- 3. Dynamic Metadata Form -->
  <!-- Filename and audio-hash are visible but locked. id/hash are hidden. -->
  {#if metadata.filename}
    <!-- SHARED COMPONENT -->
    <div class="mb-6">
      <MetadataForm 
        bind:metadata 
        hiddenFields={['id', 'hash']} 
        readonlyFields={['filename', 'audio-hash']} 
      />
    </div>
  
    <!-- 4. Upload & Cancel Buttons & Progress -->
    <div class="flex gap-4">
       <button 
        onclick={handleCancelUpload}
        disabled={isUploading}
        class="flex-1 bg-slate-700 hover:bg-slate-600 text-slate-200 font-bold py-2 px-4 rounded transition-colors">
        Cancel
      </button>

      <button 
        onclick={handleUpload}
        disabled={isUploading || isCatalogLoading || duplicateWarning}
        class="flex-1 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-50 text-slate-900 font-bold py-2 px-4 rounded transition-colors">
        {isUploading ? `Uploading... ${uploadProgress}%` : '🚀 Upload Track'}
      </button>
    </div>

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
