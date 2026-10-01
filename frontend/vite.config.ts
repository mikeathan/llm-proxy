import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import svgLoader from 'vite-svg-loader'
import { APP_TITLE } from './src/config/brand'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    svgLoader(),
    // index.html's <title> comes from config/brand.ts — no second copy of the name (D25).
    { name: 'app-title', transformIndexHtml: (html) => html.replaceAll('%APP_TITLE%', APP_TITLE) },
  ],
  base: '/admin/',
  server: {
    proxy: {
      '/admin/api': {
        target: 'http://127.0.0.1:4001',
        changeOrigin: true
      },
    }
  },
  build: {
    outDir: '../backend/internal/transport/http/frontend_dist',
    emptyOutDir: true
  }
})
