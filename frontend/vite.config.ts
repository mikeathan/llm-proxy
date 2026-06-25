import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import svgLoader from 'vite-svg-loader'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), svgLoader()],
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
