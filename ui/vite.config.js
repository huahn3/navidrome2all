import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

const frontendPort = parseInt(process.env.PORT) || 4533
const backendPort = frontendPort + 100

// index.html is rendered by the Go server, which fills in window.__APP_CONFIG__.
// Vite leaves the template untouched, so without this every server-side setting
// (jukebox, WebP, agents, defaults...) silently falls back to the dev defaults.
function injectAppConfig() {
  return {
    name: 'navidrome-app-config',
    apply: 'serve',
    async transformIndexHtml(html) {
      let appConfig = '{}'
      try {
        const res = await fetch(`http://localhost:${backendPort}/app`)
        const rendered = /window\.__APP_CONFIG__ = ("(?:[^"\\]|\\.)*")/.exec(
          await res.text(),
        )
        if (rendered) {
          appConfig = rendered[1]
        }
      } catch {
        // Backend not up yet: keep the defaults from src/config.js
      }
      return html.replace('{{ .AppConfig }}', appConfig)
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    injectAppConfig(),
    react(),
    VitePWA({
      manifest: manifest(),
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.js',
      injectManifest: {
        maximumFileSizeToCacheInBytes: 3 * 1024 * 1024, // 3 MiB
        // index.html is rendered per-user by the server, so a precached copy
        // would pin one user's config (and auth payload) across logins
        globIgnores: ['index.html'],
      },
      devOptions: {
        enabled: true,
      },
    }),
  ],
  server: {
    host: true,
    port: frontendPort,
    proxy: {
      '^/(auth|api|rest|backgrounds)/.*': 'http://localhost:' + backendPort,
    },
  },
  base: './',
  define: {
    // JSONForms and other libraries use process.env
    'process.env': JSON.stringify({}),
  },
  build: {
    outDir: 'build',
    sourcemap: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/setupTests.js',
    css: true,
    reporters: ['verbose'],
    // reporters: ['default', 'hanging-process'],
    coverage: {
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*'],
      exclude: [],
    },
  },
})

// PWA manifest
function manifest() {
  return {
    name: 'Navidrome',
    short_name: 'Navidrome',
    description:
      'Navidrome, an open source web-based music collection server and streamer',
    categories: ['music', 'entertainment'],
    display: 'standalone',
    start_url: './',
    background_color: 'white',
    theme_color: 'blue',
    icons: [
      {
        src: './android-chrome-192x192.png',
        sizes: '192x192',
        type: 'image/png',
      },
      {
        src: './android-chrome-512x512.png',
        sizes: '512x512',
        type: 'image/png',
      },
    ],
  }
}
