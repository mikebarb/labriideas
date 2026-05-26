<script>
  // These will be passed in from your CatalogViewer when a user clicks a track
  export let trackFilename = "";
  export let trackTitle = "No Track Selected";

  let audioUrl = "";
  let isLoading = false;
  let status = "";

  // We use a reactive statement: whenever the trackFilename changes, fetch a new URL
  $: if (trackFilename) {
    loadAudioUrl(trackFilename);
  }

  async function loadAudioUrl(filename) {
    isLoading = true;
    status = "Loading audio stream...";

    try {
      // 1. Ask our Go API for the signed URL (Reusing your exact logic!)
      const response = await fetch(`http://localhost:8080/api/download?file=${encodeURIComponent(filename)}`);
      
      if (!response.ok) {
        throw new Error(`Server error: ${response.status}`);
      }

      const data = await response.json();

      // 2. If we got the URL, assign it to the audio player
      if (data.url) {
        audioUrl = data.url;
        status = ""; // Clear status once ready
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

<div class="bg-slate-800 p-4 rounded-lg border border-slate-700 shadow-xl fixed bottom-0 left-0 w-full">
  <div class="max-w-4xl mx-auto flex flex-col gap-2">
    
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-bold text-cyan-400 truncate">
        {#if isLoading}
          ⏳ {status}
        {:else if trackTitle}
          🎵 {trackTitle}
        {:else}
          Select a track to play
        {/if}
      </h3>
    </div>

    <!-- The HTML5 Audio Element -->
    <!-- autoplay will force the browser to start playing as soon as the URL is set -->
    <!-- controls provides the native play/pause/seek/volume UI -->
    {#if audioUrl}
      <audio 
        src={audioUrl} 
        controls 
        autoplay 
        class="w-full h-10 rounded-md">
          Your browser does not support the audio element.
      </audio>
    {:else}
      <div class="bg-slate-700 h-10 rounded-md flex items-center justify-center text-slate-400 text-sm">
        Player Idle
      </div>
    {/if}

  </div>
</div>
