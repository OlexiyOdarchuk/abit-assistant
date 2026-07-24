import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Wails build config: emit into ./dist, which the desktop binary embeds
// (//go:embed all:frontend/dist). No /api proxy here — the desktop shell talks
// to the Go core over Wails bindings (window.go), not HTTP. The web server
// build keeps using vite.config.js (which emits into ../internal/web/dist).
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
