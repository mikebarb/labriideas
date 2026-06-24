<script lang="ts">
  import { buildTrack } from '../lib/buildTrack.js';

  interface Person {
    speakerName: string;
    items: Array<{
      id: string;
      title: string;
      filename: string;
    }>;
    moreLink?: string;
  }

  interface Props {
    people: Record<string, Person>;
  }

  let { people }: Props = $props();

  // Convert the object to an array
  const peopleList = $derived(Object.values(people));

  // Click handler for playing tracks
  function playTrack(item: { id: string; title: string; filename: string }, speaker: string) {
    const track = buildTrack({ ...item, speaker });
    window.dispatchEvent(new CustomEvent('play-track', { detail: track }));
  }
</script>

<div class="grid grid-cols-1 md:grid-cols-2 gap-8 max-w-5xl mx-auto">
  {#each Object.entries(people) as [slug, person]}
    <div class="space-y-4">
      <h2 class="text-2xl font-bold text-gray-900">{person.speakerName}</h2>
      
      <div class="space-y-2">
        {#each person.items as item}
          <button 
            type="button"
            onclick={() => playTrack(item, person.speakerName)}
            class="block w-full text-left p-3 bg-gray-50 hover:bg-gray-100 rounded-md transition border border-gray-200"
          >
            <span class="font-semibold text-gray-900">{item.title}</span>
          </button>
        {/each}
      </div>

      {#if person.moreLink}
        <a 
          href={`/schaeffer/${slug}/`}
          class="inline-block bg-orange-600 hover:bg-orange-700 text-white font-semibold px-6 py-2 rounded-md transition"
        >
          Listen to More →
        </a>
      {/if}
    </div>
  {/each}
</div>
