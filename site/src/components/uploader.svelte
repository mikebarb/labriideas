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

async function calculateAudioHash_alternative(file) {
    // 1. Peek at the first 10 bytes to check for ID3
    const header = await file.slice(0, 10).arrayBuffer();
    const view = new DataView(header);
    let skip = 0;

    if (String.fromCharCode(...new Uint8Array(header.slice(0, 3))) === 'ID3') {
        // Calculate synchsafe size
        skip = (view.getUint8(6) << 21) | (view.getUint8(7) << 14) | 
               (view.getUint8(8) << 7) | view.getUint8(9);
        skip += 10;
    }

    // 2. Hash the rest using SubtleCrypto
    const audioBlob = file.slice(skip);
    const buffer = await audioBlob.arrayBuffer();
    const hashBuffer = await crypto.subtle.digest('MD5', buffer);
    
    // Convert to hex string
    return Array.from(new Uint8Array(hashBuffer))
        .map(b => b.toString(16).padStart(2, '0')).join('');
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



  async function handleUpload() {
    if (!file) {
      status = "Please select a file first.";
      return;
    }

    isLoading = true;
    status = "Uploading to Go server...";

    // 1. Build standard Multi-part Form Data
    //const formData = new FormData();
    //formData.append("file", file);
    //formData.append("title", title);
    //formData.append("artist", artist);

    try {
      // STEP 1: Calculate the Audio Hash locally in the browser!
      status = "Fingerprinting audio...";
      const audioHash = await calculateAudioHash(file);

       // STEP 2: Build standard Multi-part Form Data
      status = "Uploading to Go server...";
      const formData = new FormData();
      formData.append("file", file);
      formData.append("title", title);
      formData.append("artist", artist);
      formData.append("audio-hash", audioHash); // INJECT THE FINGERPRINT!
      
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
