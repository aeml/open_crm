import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const isGithubActions = process.env.GITHUB_ACTIONS === 'true'
const pagesBase = isGithubActions ? '/' : '/'

export default defineConfig({
  base: pagesBase,
  cacheDir: process.env.VITE_CACHE_DIR || 'node_modules/.vite',
  build: {
    outDir: process.env.VITE_OUT_DIR || 'dist'
  },
  plugins: [react()]
})
