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

describe('settings email templates route', () => {
  it('lists templates and creates a new one', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(sessionResponse())
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: { templates: [{ id: 3, name: 'Welcome', subject: 'Hi there', body: 'Hello {{first_name}}' }] }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: { template: { id: 5, name: 'Follow up', subject: 'Checking in', body: 'Hi {{first_name}}' } }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/email-templates')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /email templates/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /welcome/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Follow up' } })
    fireEvent.change(screen.getByLabelText(/subject/i), { target: { value: 'Checking in' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Hi {{first_name}}' } })
    fireEvent.click(screen.getByRole('button', { name: /create template/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/email-templates') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({ name: 'Follow up', subject: 'Checking in', body: 'Hi {{first_name}}' })
    })
    expect(await screen.findByRole('heading', { name: /follow up/i })).toBeInTheDocument()
  })
})
