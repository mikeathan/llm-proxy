import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  base: '/admin/', 
  server: {
    proxy: {
      '/admin/api': {
        target: 'http://127.0.0.1:4001',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: '../backend/internal/transport/http/frontend_dist',
    emptyOutDir: true
  }
})
