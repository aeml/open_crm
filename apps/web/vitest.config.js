import { configDefaults, defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.js',
    testTimeout: 10000,
    // Route tests mount full jsdom applications. Bound file concurrency so
    // the suite stays deterministic on small runners and many-core hosts.
    maxWorkers: 4,
    exclude: [...configDefaults.exclude, '**/e2e/**']
  }
})
