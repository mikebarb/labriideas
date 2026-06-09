// @ts-check
import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';
import tailwindcss from '@tailwindcss/vite';
import cloudflare from '@astrojs/cloudflare';

// https://astro.build/config
export default defineConfig({
  output: 'server',
  adapter: cloudflare({
    mode: 'advanced', // <--- ADD THIS: Forces a single _worker.js output
  }),
  integrations: [svelte()],

  vite: {
    plugins: [tailwindcss()]
  }
});
