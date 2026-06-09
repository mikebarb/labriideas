// @ts-check
import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';
import tailwindcss from '@tailwindcss/vite';
import cloudflare from '@astrojs/cloudflare';


// https://astro.build/config
export default defineConfig({
  output: 'server',
  adapter: cloudflare({ // Disable features that require external Cloudflare Bindings
    imageService: 'passthrough', // Disables the "IMAGES" binding requirement
    platformProxy: {
        enabled: false,         // Disables trying to connect to KV/Sessions locally
    }),
  integrations: [svelte()],

  vite: {
    plugins: [tailwindcss()]
  }
});
