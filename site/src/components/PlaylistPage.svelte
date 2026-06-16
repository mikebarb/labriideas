<script>
  import menuData from '../public/data/menu.json'; // Direct import
  import { currentTrack } from '../stores/audioPlayer.js';

  // Function to trigger player
  function playTrack(track, albumTitle) {
      // Logic to find full track metadata from catalog.json
      // and add to your global player store
      console.log("Playing:", track.filename);
  }
</script>

<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
  {#each menuData.albums as album}
    <div class="album-card relative h-64 bg-cover bg-center rounded-lg overflow-hidden" 
         style="background-image: url({album.backgroundImage})">
      <div class="absolute inset-0 bg-black bg-opacity-50 p-6 flex flex-col justify-end">
        <h2 class="text-white text-xl font-bold">{album.title}</h2>
        <p class="text-gray-200 text-sm">{album.description}</p>
      </div>
    </div>

    <ul class="track-list space-y-2">
      {#each album.tracks as track}
        <li>
          <button on:click={() => playTrack(track, album.title)} 
                  class="text-blue-600 hover:underline">
            {track.displayTitle}
          </button>
        </li>
      {/each}
    </ul>
  {/each}
</div>
