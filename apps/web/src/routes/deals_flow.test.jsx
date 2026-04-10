import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('deals flow', () => {
  it('filters deals with the shared search pattern', async () => {
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
              { id: 11, name: 'Northstar Expansion', stageId: 2, stageName: 'Qualified', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 },
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 2, openCount: 2, wonCount: 0, pipelineValue: '108000.00' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 6, name: 'Bluebird Health', domain: 'bluebird.example', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }
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
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
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

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(await screen.findByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getByText(/showing 2 of 2 deals/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search deals/i), { target: { value: 'bluebird' } })

    expect(await screen.findByText(/showing 1 of 1 deals/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\?q=bluebird$/), expect.any(Object))
    })
  })

  it('loads stages and deals, creates a deal, and moves it to another stage', async () => {
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
              { id: 2, name: 'Qualified', position: 2, isClosed: false, isWon: false },
              { id: 3, name: 'Proposal', position: 3, isClosed: false, isWon: false }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 11, name: 'Northstar Expansion', stageId: 3, stageName: 'Proposal', companyId: 5, companyName: 'Northstar Logistics', primaryContactId: 7, primaryContactName: 'Morgan Lee', status: 'open', valueAmount: '48000.00', valueCurrency: 'USD', expectedCloseDate: '2026-04-19', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '48000.00' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            companies: [
              { id: 6, name: 'Bluebird Health', domain: 'bluebird.example', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }
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
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            deal: { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 },
            activities: [],
            notes: [],
            tasks: []
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            task: {
              id: 92,
              entityType: 'deal',
              entityId: 12,
              entityLabel: 'Bluebird Rollout',
              title: 'Draft rollout kickoff agenda',
              description: 'Include legal and operations handoff.',
              status: 'open',
              dueAt: '2026-04-21T13:00:00Z',
              completedAt: '',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 101, action: 'task.created', summary: 'Task created' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            note: {
              id: 51,
              entityType: 'deal',
              entityId: 12,
              body: 'Legal requested updated indemnity language.',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner',
              createdAt: '2026-04-10T12:00:00Z',
              updatedAt: '2026-04-10T12:00:00Z'
            },
            activity: {
              id: 100,
              action: 'note.created',
              summary: 'Note added',
              createdAt: '2026-04-10T12:00:00Z'
            }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deal: { id: 12, name: 'Bluebird Rollout', stageId: 3, stageName: 'Proposal', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 },
            activities: [
              { id: 99, action: 'deal.stage_changed', summary: 'Deal moved to Proposal' }
            ]
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/deals')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /deals/i })).toBeInTheDocument()
    expect(screen.getByText(/pipeline value/i)).toBeInTheDocument()
    expect(await screen.findByText(/northstar expansion/i)).toBeInTheDocument()
    expect(screen.getAllByText('$48,000.00').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/proposal/i).length).toBeGreaterThan(0)

    expect(screen.queryByLabelText(/company id/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/primary contact id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^company$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/primary contact/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/deal name/i), { target: { value: 'Bluebird Rollout' } })
    fireEvent.change(screen.getByLabelText(/stage/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/^company$/i), { target: { value: '6' } })
    fireEvent.change(screen.getByLabelText(/primary contact/i), { target: { value: '8' } })
    fireEvent.change(screen.getByLabelText(/value amount/i), { target: { value: '60000.00' } })
    fireEvent.change(screen.getByLabelText(/value currency/i), { target: { value: 'USD' } })
    fireEvent.change(screen.getByLabelText(/expected close date/i), { target: { value: '2026-05-02' } })
    fireEvent.click(screen.getByRole('button', { name: /save deal/i }))

    expect((await screen.findAllByText(/bluebird rollout/i)).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/qualified/i).length).toBeGreaterThan(0)

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Draft rollout kickoff agenda' } })
    fireEvent.change(screen.getByLabelText(/task description/i), { target: { value: 'Include legal and operations handoff.' } })
    fireEvent.change(screen.getByLabelText(/assigned to user id/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-21T13:00' } })
    fireEvent.click(screen.getByRole('button', { name: /^save task$/i }))

    expect(await screen.findByText(/draft rollout kickoff agenda/i)).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks$/), expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"entityType":"deal"')
      }))
    })

    fireEvent.change(screen.getByLabelText(/new note/i), { target: { value: 'Legal requested updated indemnity language.' } })
    fireEvent.click(screen.getByRole('button', { name: /add note/i }))

    expect(await screen.findByText(/legal requested updated indemnity language/i)).toBeInTheDocument()
    expect(screen.getByText(/note added/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/move stage/i), { target: { value: '3' } })
    fireEvent.click(screen.getByRole('button', { name: /move to stage/i }))

    expect(await screen.findByText(/deal moved to proposal/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12\/stage$/), expect.objectContaining({ method: 'PATCH' }))
    })
  })
})
