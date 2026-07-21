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
    outDir: process.env.VITE_OUT_DIR || 'dist',
    rollupOptions: {
      output: {
        onlyExplicitManualChunks: true,
        manualChunks(id) {
          if (['/src/components/ui/button.jsx', '/src/components/ui/card.jsx', '/src/components/ui/field.jsx', '/src/components/ui/inline_error.jsx', '/src/lib/use_page_title.js'].some((path) => id.includes(path))) {
            return 'ui'
          }
          if (id.includes('/src/routes/mailbox.jsx') || id.includes('/src/routes/team_inbox.jsx') || id.includes('/src/components/email_thread.jsx') || id.includes('/src/lib/email_messages.js')) {
            return 'email-inbox'
          }
        }
      }
    }
  },
  plugins: [react(), thirdPartyNotices()]
})
