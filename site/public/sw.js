// public/sw.js

// We use a simple versioned cache name. 
// When we push updates, we just change this number (e.g., v1 -> v2),
// and the `activate` event will clean up the old cache.
const CACHE_NAME = 'labri-pwa-shell-v1';

// We use a fallback page for any navigation that fails.
const FALLBACK_HTML = '/offline.html';

// Add any critical files here. Astro generates hashed JS/CSS in /_astro/
// We can let Workbox precache those automatically if we use a build script,
// but for a manual SW, we use a "Stale-While-Revalidate" strategy for assets
// and "Network First, falling back to Cache" for HTML.

self.addEventListener('install', (event) => {
  // Pre-cache the offline fallback immediately
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll([FALLBACK_HTML]);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  // Clean up old caches
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.filter((name) => name !== CACHE_NAME)
                  .map((name) => caches.delete(name))
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const { request } = event;

  // 1. Ignore API calls and non-GET requests entirely
  if (request.method !== 'GET' || request.url.includes('/api/')) {
    return;
  }

  // 2. Ignore audio files - they are handled by our OPFS logic
  if (request.url.match(/\.(mp3|m4a|wav|ogg)$/i)) {
    return;
  }

  // 3. For HTML navigations: Network First, fall back to Cache, fall back to offline.html
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .then((response) => {
          // If we got a real response, cache it and return it
          const responseClone = response.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(request, responseClone);
          });
          return response;
        })
        .catch(() => {
          // Network failed. Try cache.
          return caches.match(request).then((cachedResponse) => {
            return cachedResponse || caches.match(FALLBACK_HTML);
          });
        })
    );
    return;
  }

  // 4. For JS, CSS, images, fonts: Cache First, fall back to Network
  event.respondWith(
    caches.match(request).then((cachedResponse) => {
      return cachedResponse || fetch(request).then((response) => {
        // Cache the asset for next time
        if (response.ok) {
          const responseClone = response.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(request, responseClone);
          });
        }
        return response;
      });
    })
  );
});
