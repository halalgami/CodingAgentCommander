import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve } from 'path'

export default defineConfig({
  plugins: [svelte()],
  build: {
    // Raid/Fleet art must stay real files: the heroine starter kit (Go) reads them
    // from dist/assets, and an inlined data: URI never lands there.
    assetsInlineLimit: (file) => (/assets\/(raid|fleet)\//.test(file) ? false : undefined),
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'index.html'),
      },
    },
  },
})
