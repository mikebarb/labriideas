// @ts-check
import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  output: 'static',
  //trailingSlash: 'always',   // fixes Cloudflare sub‑page 403/404 , now using middleware!
  integrations: [svelte()],
  vite: {
    plugins: [tailwindcss()]
  }
});
