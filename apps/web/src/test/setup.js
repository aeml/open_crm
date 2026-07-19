import '@testing-library/jest-dom/vitest'
import { cleanup, configure } from '@testing-library/react'
import { afterEach } from 'vitest'

// Route modules are lazy-loaded in production. Under the bounded four-worker
// full suite, transformation can legitimately take just over DOM Testing
// Library's one-second default even though focused flows complete immediately.
configure({ asyncUtilTimeout: 3000 })

afterEach(() => {
  cleanup()
})
