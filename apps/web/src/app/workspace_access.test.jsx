import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { AppProviders, useAuth, workspaceAllowsWrites } from './providers'
import { AppRouter } from './router'

afterEach(() => {
  vi.unstubAllGlobals()
})

const session = {
  user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
  organization: { id: 42, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
  membership: { role: 'owner' }
}

function jsonResponse(data, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => data }
}

function AccessProbe() {
  const { workspaceWritable } = useAuth()
  return <p>{workspaceWritable ? 'Writable' : 'Read only'}</p>
}

describe('hosted workspace access', () => {
  it('updates the shared access decision when an API write is blocked', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ data: {
      ...session,
      workspaceAccess: { state: 'writable' }
    } })))

    render(<AppProviders><AccessProbe /></AppProviders>)
    expect(await screen.findByText('Writable')).toBeInTheDocument()

    act(() => {
      window.dispatchEvent(new CustomEvent('workspace:access-changed', { detail: { state: 'read_only' } }))
    })
    expect(await screen.findByText('Read only')).toBeInTheDocument()
  })

  it('keeps reads and billing recovery visible while hiding dashboard write controls', async () => {
    const fetchMock = vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path === '/auth/me') {
        return jsonResponse({ data: {
          ...session,
          workspaceAccess: { state: 'read_only' }
        } })
      }
      if (path === '/api/notifications/unread-count') return jsonResponse({ data: { unreadCount: 0 } })
      if (path === '/api/dashboard/summary') {
        return jsonResponse({ data: {
          forecast: {
            currency: 'USD',
            members: [{ userId: 1, userName: 'Demo Owner', quotaAmount: '1000.00' }],
            stages: []
          },
          recentActivities: []
        } })
      }
      throw new Error(`Unexpected fetch: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    const banner = await screen.findByRole('alert')
    expect(banner).toHaveTextContent(/workspace is read-only/i)
    expect(banner).toHaveTextContent(/reads and csv exports remain available/i)
    expect(screen.getAllByRole('link', { name: /review plan & billing|plan & billing/i }).length).toBeGreaterThan(0)
    expect(await screen.findByRole('button', { name: /apply forecast period/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save quota for demo owner/i })).not.toBeInTheDocument()
  })

  it('allows only absent, unmanaged, and explicitly writable states', () => {
    expect(workspaceAllowsWrites(null)).toBe(true)
    expect(workspaceAllowsWrites({ state: 'unmanaged' })).toBe(true)
    expect(workspaceAllowsWrites({ state: 'writable' })).toBe(true)
    expect(workspaceAllowsWrites({ state: 'read_only' })).toBe(false)
    expect(workspaceAllowsWrites({ state: 'unavailable' })).toBe(false)
    expect(workspaceAllowsWrites({ state: 'unexpected' })).toBe(false)
  })
})
