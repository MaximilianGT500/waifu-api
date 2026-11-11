import { defineConfig, loadEnv, type Connect, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { constants as zlib, gzipSync } from 'node:zlib'

const CLASSIC_COOKIE = /(?:^|;\s*)waifu_design=classic(?:;|$)/

const VENDOR_REACT = /[\\/]node_modules[\\/](?:react|react-dom|scheduler)[\\/]/
const VENDOR_ROUTER = /[\\/]node_modules[\\/](?:react-router|cookie|set-cookie-parser)[\\/]/
const VENDOR_QUERY = /[\\/]node_modules[\\/]@tanstack[\\/]/
const VENDOR_HTTP = /[\\/]node_modules[\\/]axios[\\/]/

const routeDesign: Connect.NextHandleFunction = (req, _res, next) => {
  const wantsPage = req.method === 'GET' && (req.headers.accept ?? '').includes('text/html')
  const pathname = (req.url ?? '/').split('?')[0]
  if (wantsPage && !pathname.includes('.') && CLASSIC_COOKIE.test(req.headers.cookie ?? '')) {
    req.url = '/classic.html'
  }
  next()
}

const designSwitch = (): Plugin => ({
  name: 'design-switch',
  configureServer(server) {
    server.middlewares.use(routeDesign)
  },
  configurePreviewServer(server) {
    server.middlewares.use(routeDesign)
  },
})

const PRECOMPRESS = /\.(?:js|css|svg|json)$/
const precompress = (): Plugin => {
  let outDir = path.resolve(import.meta.dirname, 'dist')
  return {
    name: 'precompress',
    apply: 'build',
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir)
    },
    closeBundle() {
      const compress = (file: string) => {
        const source = readFileSync(file)
        if (source.length < 1024) return
        const gz = gzipSync(source, { level: zlib.Z_BEST_COMPRESSION })
        if (gz.length < source.length * 0.9) writeFileSync(`${file}.gz`, gz)
      }
      for (const name of readdirSync(path.join(outDir, 'assets'))) {
        if (PRECOMPRESS.test(name)) compress(path.join(outDir, 'assets', name))
      }
      for (const name of ['index.html', 'classic.html']) compress(path.join(outDir, name))
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, path.resolve(import.meta.dirname, '..'), '')
  const apiTarget = env.VITE_DEV_API_PROXY || 'http://localhost:8080'

  return {
    plugins: [designSwitch(), react(), tailwindcss(), precompress()],
    envDir: '../',
    build: {
      sourcemap: false,
      assetsInlineLimit: (file: string) => (file.endsWith('.woff2') ? false : undefined),
      rolldownOptions: {
        input: {
          index: path.resolve(import.meta.dirname, 'index.html'),
          classic: path.resolve(import.meta.dirname, 'classic.html'),
        },
        output: {
          codeSplitting: {
            groups: [
              { name: 'react', test: VENDOR_REACT, priority: 40 },
              { name: 'router', test: VENDOR_ROUTER, priority: 30 },
              { name: 'query', test: VENDOR_QUERY, priority: 30 },
              { name: 'http', test: VENDOR_HTTP, priority: 30 },
            ],
          },
        },
      },
    },
    server: {
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          secure: false,
          rewrite: (p) => p.replace(/^\/api(?=\/|$)/, '') || '/',
        },
      },
    },
  }
})
