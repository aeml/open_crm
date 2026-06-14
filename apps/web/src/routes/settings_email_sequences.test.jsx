import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function sessionResponse() {
  return {
    ok: true,
    json: async () => ({
      data: {
        user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
        organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
        membership: { role: 'owner' }
      }
    })
  }
}

describe('settings email sequences route', () => {
  it('lists sequences and creates a new draft sequence', async () => {
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/email-sequences') && method === 'POST') {
        return {
          ok: true,
          json: async () => ({
            data: {
              sequence: {
                id: 5,
                name: 'Trial nurture',
                description: 'Warm new trials',
                status: 'draft',
                steps: [{ id: 10, stepOrder: 1, delayDays: 2, subject: 'Checking in', body: 'Hi {{first_name}}' }]
              }
            }
          })
        }
      }
      if (path.endsWith('/api/email-sequences')) {
        return {
          ok: true,
          json: async () => ({
            data: {
              sequences: [
                { id: 3, name: 'Welcome cadence', description: 'First-touch follow-up', status: 'draft', steps: [{ id: 7, stepOrder: 1, delayDays: 0, subject: 'Welcome', body: 'Hello' }] }
              ]
            }
          })
        }
      }
      return { ok: true, json: async () => ({ data: { unreadCount: 0 } }) }
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-sequences')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email sequences/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /welcome cadence/i })).toBeInTheDocument()
    expect(screen.getByText(/draft · 1 step/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/sequence name/i), { target: { value: 'Trial nurture' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Warm new trials' } })
    fireEvent.change(screen.getByLabelText(/step 1 delay days/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/step 1 subject/i), { target: { value: 'Checking in' } })
    fireEvent.change(screen.getByLabelText(/step 1 body/i), { target: { value: 'Hi {{first_name}}' } })
    fireEvent.click(screen.getByRole('button', { name: /create sequence/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-sequences') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Trial nurture',
        description: 'Warm new trials',
        status: 'draft',
        steps: [{ delayDays: 2, subject: 'Checking in', body: 'Hi {{first_name}}' }]
      })
    })
    expect(await screen.findByRole('heading', { name: /trial nurture/i })).toBeInTheDocument()
  })
})
