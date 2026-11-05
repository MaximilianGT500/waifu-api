import { defineConfig, loadEnv, type Connect, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

const CLASSIC_COOKIE = /(?:^|;\s*)waifu_design=classic(?:;|$)/

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

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, path.resolve(import.meta.dirname, '..'), '')
  const apiTarget = env.VITE_DEV_API_PROXY || 'http://localhost:8080'

  return {
    plugins: [designSwitch(), react(), tailwindcss()],
    envDir: '../',
    build: {
      sourcemap: false,
      assetsInlineLimit: (file: string) => (file.endsWith('.woff2') ? false : undefined),
      rollupOptions: {
        input: {
          index: path.resolve(import.meta.dirname, 'index.html'),
          classic: path.resolve(import.meta.dirname, 'classic.html'),
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
