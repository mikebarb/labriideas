<script>
  import { BookOpen, Star, ListMusic, Search } from 'lucide-svelte';
</script>

<div class="max-w-5xl mx-auto py-12 px-4">
  <div class="grid grid-cols-2 md:grid-cols-4 gap-8 text-center">
    
    <a href="/topics" class="flex flex-col items-center hover:text-orange-600 group">
      <BookOpen class="w-10 h-10 mb-3 text-gray-800 group-hover:text-orange-600" />
      <span class="text-sm font-bold uppercase tracking-wider">Topics</span>
    </a>

    <a href="/featured" class="flex flex-col items-center hover:text-orange-600 group">
      <Star class="w-10 h-10 mb-3 text-gray-800 group-hover:text-orange-600" />
      <span class="text-sm font-bold uppercase tracking-wider">Featured</span>
    </a>

    <a href="/playlists" class="flex flex-col items-center hover:text-orange-600 group">
      <ListMusic class="w-10 h-10 mb-3 text-gray-800 group-hover:text-orange-600" />
      <span class="text-sm font-bold uppercase tracking-wider">Playlists</span>
    </a>

    <a href="/search/search3/" class="flex flex-col items-center hover:text-orange-600 group">
      <Search class="w-10 h-10 mb-3 text-gray-800 group-hover:text-orange-600" />
      <span class="text-sm font-bold uppercase tracking-wider">Search</span>
    </a>

  </div>
</div>
