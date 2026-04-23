import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  base: process.env.VITE_BASE_URL || '', // Dynamic base path for proxy support
  server: {
    host: '0.0.0.0', // Necessary for the Docker-Go proxy
    port: parseInt(process.env.VITE_PORT || process.env.PORT || '5173'),
    allowedHosts: true, // Allow proxying from the backend container
    fs: {
      allow: ['..', '/app'], // Allow access to shared node_modules in parent
    }
  }
})
