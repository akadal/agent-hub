import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: true,
    port: 27342,
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY || 'http://localhost:27341',
        changeOrigin: true,
        // Terminal bridge is WebSocket; without this, stream/classic stay "connecting…".
        ws: true,
      },
      '/health': {
        target: process.env.VITE_API_PROXY || 'http://localhost:27341',
        changeOrigin: true,
      },
    },
  },
})
