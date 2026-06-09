// @ts-check
import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';
import tailwindcss from '@tailwindcss/vite';
import cloudflare from '@astrojs/cloudflare';

// https://astro.build/config
export default defineConfig({
  output: 'server',
  //output: 'static', // Astro generates pure HTML
  adapter: cloudflare({
    mode: 'directory', // THIS forces the single _worker.js file
  }),
  integrations: [svelte()],

  vite: {
    plugins: [tailwindcss()]
  }
});

