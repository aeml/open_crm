import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { readFileSync } from 'node:fs'

const isGithubActions = process.env.GITHUB_ACTIONS === 'true'
const pagesBase = isGithubActions ? '/' : '/'

function thirdPartyNotices() {
  return {
    name: 'open-crm-third-party-notices',
    generateBundle() {
      this.emitFile({
        type: 'asset',
        fileName: 'THIRD_PARTY_NOTICES.md',
        source: readFileSync(new URL('../../THIRD_PARTY_NOTICES.md', import.meta.url), 'utf8')
      })
    }
  }
}

export default defineConfig({
  base: pagesBase,
  cacheDir: process.env.VITE_CACHE_DIR || 'node_modules/.vite',
  build: {
    outDir: process.env.VITE_OUT_DIR || 'dist'
  },
  plugins: [react(), thirdPartyNotices()]
})
