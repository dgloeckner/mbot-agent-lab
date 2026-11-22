import { defineConfig } from 'vitest/config'
import preact from '@preact/preset-vite'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('.', import.meta.url))

// The Go server embeds the build from server/internal/web/dist/app.
// In development Vite serves the page and forwards API and SSE calls to the
// Go server started with `npx nx run server:dev`.
export default defineConfig({
  root,
  plugins: [preact()],
  build: {
    outDir: fileURLToPath(new URL('../server/internal/web/dist/app', import.meta.url)),
    emptyOutDir: true,
    target: 'es2022',
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      '^/t/[^/]+/.+': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
  test: {
    root,
    include: ['src/**/*.test.ts'],
  },
})
