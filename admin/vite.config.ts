import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  base: '/admin/',
  plugins: [svelte()],
  server: {
    host: '127.0.0.1',
    port: 5173
  }
});
