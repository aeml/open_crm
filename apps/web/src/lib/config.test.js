import { describe, expect, it } from 'vitest'
import { API_BASE_URL } from './config'

describe('frontend config', () => {
  it('defaults the API base URL to the production backend host', () => {
    expect(API_BASE_URL).toBe('https://crmserver.mendola.tech')
  })
})
