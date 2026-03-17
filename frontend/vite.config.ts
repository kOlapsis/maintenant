import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

// Build revision used to isolate caches between deployments.
// Each build produces unique cache names so old caches are orphaned
// and cleaned up by Workbox on SW activation.
const buildRevision = Date.now().toString(36)

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    ...(process.env.NODE_ENV !== 'production' ? [vueDevTools()] : []),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      manifest: false, // Use external manifest.webmanifest
      workbox: {
        cleanupOutdatedCaches: true,
        skipWaiting: true,
        clientsClaim: true,
        navigateFallbackDenylist: [/\/api\//, /\/ping\//, /\/status\//],
        runtimeCaching: [
          {
            urlPattern: /\/api\/v1\/(?!containers\/events|status\/smtp|channels|webhooks)/,
            handler: 'NetworkFirst',
            options: {
              cacheName: `api-${buildRevision}`,
              expiration: { maxEntries: 50, maxAgeSeconds: 300 },
              networkTimeoutSeconds: 5,
            },
          },
          {
            urlPattern: /\.(js|css|woff2?|png|jpg|svg|ico)$/,
            handler: 'CacheFirst',
            options: {
              cacheName: `static-${buildRevision}`,
              expiration: { maxEntries: 100, maxAgeSeconds: 60 * 60 * 24 * 30 },
            },
          },
        ],
      },
    }),
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/status/api': 'http://localhost:8080',
      '/status/events': 'http://localhost:8080',
      '/status/feed.atom': 'http://localhost:8080',
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})
