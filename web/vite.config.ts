import { execSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Shown under the menu: the version in package.json, and the commit it was built
// from (passed in by make for Docker builds, read from git otherwise).
const version = (JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')) as { version: string }).version
function commit(): string {
  if (process.env.APP_COMMIT) return process.env.APP_COMMIT
  try {
    return execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim()
  } catch {
    return ''
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(version),
    __APP_COMMIT__: JSON.stringify(commit()),
  },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080' } },
  build: {
    rolldownOptions: {
      output: {
        // Libraries change less often than the app, so browsers keep them across deploys.
        codeSplitting: {
          groups: [
            { name: 'react', test: /node_modules[\\/].*[\\/](react|react-dom|react-router|scheduler)[\\/]/ },
            { name: 'forms', test: /node_modules[\\/].*[\\/](zod|react-hook-form|@hookform)[\\/]/ },
          ],
        },
      },
    },
  },
  test: { include: ['src/**/*.test.ts'] },
})
