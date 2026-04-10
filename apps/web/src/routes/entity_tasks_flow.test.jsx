import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('entity task visibility', () => {
  it('shows company tasks on company detail and loads them with entity filters', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            contacts: [
              { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'Head of RevOps', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            company: { id: 5, name: 'Northstar Logistics', domain: 'northstar.example', industry: 'Logistics', phone: '555-0200', website: 'https://northstar.example', status: 'prospect' },
            linkedContacts: [],
            activities: []
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { notes: [] } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [
              {
                id: 88,
                entityType: 'company',
                entityId: 5,
                entityLabel: 'Northstar Logistics',
                title: 'Collect warehouse onboarding contacts',
                description: 'Need ops and procurement owners.',
                status: 'open',
                dueAt: '2026-04-18T15:00:00Z',
                completedAt: '',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /companies/i })).toBeInTheDocument()
    expect(await screen.findByText('northstar.example')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /northstar logistics/i }))

    expect(await screen.findByText(/collect warehouse onboarding contacts/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=company&entityId=5$/), expect.any(Object))
    })
  })

  it('shows deal tasks on deal detail and loads them with entity filters', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            stages: [
              { id: 1, name: 'Lead', position: 1, isClosed: false, isWon: false },
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '60000.00' }
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { companies: [{ id: 6, name: 'Bluebird Health', domain: 'bluebird.example', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }], meta: { page: 1, pageSize: 20, total: 1 } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { contacts: [{ id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { notes: [] } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [
              {
                id: 91,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Send revised rollout SOW',
                description: 'Legal and finance need final draft.',
                status: 'open',
                dueAt: '2026-04-19T17:00:00Z',
                completedAt: '',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(await screen.findByText(/bluebird rollout/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /bluebird rollout/i }))

    expect(await screen.findByText(/send revised rollout sow/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&entityType=deal&entityId=12$/), expect.any(Object))
    })
  })
})
