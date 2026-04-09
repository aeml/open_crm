import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const isGithubActions = process.env.GITHUB_ACTIONS === 'true'
const pagesBase = isGithubActions ? '/' : '/'

export default defineConfig({
  base: pagesBase,
  plugins: [react()]
})
