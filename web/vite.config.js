import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The build is emitted straight into internal/web/dist, where Go embeds it
// (//go:embed all:dist) so the whole app ships as one binary.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  // In dev (`npm run dev`), proxy the JSON API to the Go server on :8080 so
  // the SPA and API share an origin.
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
