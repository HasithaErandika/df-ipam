import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  build: {
    rolldownOptions: {
      output: {
        // Libraries change less often than the app, so browsers keep them across deploys.
        codeSplitting: {
          groups: [
            { name: 'react', test: /node_modules[\\/].*[\\/](react|react-dom|react-router|scheduler)[\\/]/ },
            { name: 'supabase', test: /node_modules[\\/].*[\\/]@supabase[\\/]/ },
            { name: 'forms', test: /node_modules[\\/].*[\\/](zod|react-hook-form|@hookform)[\\/]/ },
          ],
        },
      },
    },
  },
  test: { include: ['src/**/*.test.ts'] },
})
