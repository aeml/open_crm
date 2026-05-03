import { afterEach, describe, expect, it, vi } from 'vitest'
import { updateProfile, getPreferences, updatePreferences } from './profile'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('updateProfile', () => {
  it('sends PATCH request and returns updated user', async () => {
    const user = { id: 1, email: 'a@example.com', firstName: 'Alice', lastName: 'Ng' }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { user } })
    })

    const result = await updateProfile({ firstName: 'Alice', lastName: 'Ng' })
    expect(result).toEqual(user)
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/me/profile'),
      expect.objectContaining({ method: 'PATCH', credentials: 'include' })
    )
  })

  it('throws on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: { message: 'First name required' } })
    })

    await expect(updateProfile({ firstName: '', lastName: '' })).rejects.toMatchObject({
      message: 'First name required'
    })
  })
})

describe('getPreferences', () => {
  it('returns preferences from API', async () => {
    const preferences = { defaultLandingView: '/tasks' }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { preferences } })
    })

    const result = await getPreferences()
    expect(result).toEqual(preferences)
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/me/preferences'),
      expect.objectContaining({ credentials: 'include' })
    )
  })

  it('returns empty object when preferences are missing', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: {} })
    })

    const result = await getPreferences()
    expect(result).toEqual({})
  })

  it('throws on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({ error: { message: 'Server error' } })
    })

    await expect(getPreferences()).rejects.toMatchObject({ message: 'Server error' })
  })
})

describe('updatePreferences', () => {
  it('sends PATCH request and returns updated preferences', async () => {
    const preferences = { defaultLandingView: '/deals' }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { preferences } })
    })

    const result = await updatePreferences({ defaultLandingView: '/deals' })
    expect(result).toEqual(preferences)
    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/me/preferences'),
      expect.objectContaining({ method: 'PATCH', credentials: 'include' })
    )
  })

  it('returns empty object when response data is missing', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: {} })
    })

    const result = await updatePreferences({ defaultLandingView: '' })
    expect(result).toEqual({})
  })

  it('throws on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: { message: 'Invalid landing view' } })
    })

    await expect(updatePreferences({ defaultLandingView: '/invalid' })).rejects.toMatchObject({
      message: 'Invalid landing view'
    })
  })
})
