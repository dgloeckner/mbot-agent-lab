import { defineConfig } from 'vitest/config'
import type { Plugin } from 'vite'
import preact from '@preact/preset-vite'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// Blockly's cursors, sprites and icons are served from our own host: lab PCs
// load nothing from other hosts. The tool editor points Blockly's `media` here.
const blocklyMedia = fileURLToPath(new URL('../node_modules/blockly/media/', import.meta.url))
const MEDIA_PATH = 'assets/blockly-media/'
function blocklyMediaPlugin(): Plugin {
  return {
    name: 'blockly-media',
    generateBundle() {
      for (const file of readdirSync(blocklyMedia)) {
        if (!file.endsWith('.mp3')) this.emitFile({ type: 'asset', fileName: MEDIA_PATH + file, source: readFileSync(blocklyMedia + file) })
      }
    },
    configureServer(server) {
      server.middlewares.use('/' + MEDIA_PATH, (req, res, next) => {
        const file = (req.url ?? '').replace(/^\//, '').split('?')[0]
        if (!/^[\w.-]+$/.test(file)) return next()
        try {
          res.end(readFileSync(blocklyMedia + file))
        } catch {
          next()
        }
      })
    },
  }
}

const root = fileURLToPath(new URL('.', import.meta.url))

// The Go server embeds the build from server/internal/web/dist/app.
// In development Vite serves the page and forwards API and SSE calls to the
// Go server started with `npx nx run server:dev`.
export default defineConfig({
  root,
  plugins: [preact(), blocklyMediaPlugin()],
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
      '^/api/': { target: 'http://127.0.0.1:8080', changeOrigin: false },
      '^/admin/api/': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
  test: {
    root,
    include: ['src/**/*.test.ts'],
  },
})
