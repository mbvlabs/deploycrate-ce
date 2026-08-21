import { fileURLToPath, URL } from 'node:url'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./resources/js', import.meta.url)),
    },
  },
  base: '/assets/dist/',
  build: {
    manifest: 'vite/manifest.json',
    assetsDir: '',
    outDir: 'assets/dist',
    rollupOptions: {
      input: 'resources/js/app.ts',
      output: {
        assetFileNames: '[name]-v2-[hash][extname]',
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
  },
})
