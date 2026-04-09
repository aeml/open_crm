import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('deals flow', () => {
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
        status: 201,
        json: async () => ({
          data: {
            deal: { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 },
            activities: []
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

    fireEvent.change(screen.getByLabelText(/deal name/i), { target: { value: 'Bluebird Rollout' } })
    fireEvent.change(screen.getByLabelText(/stage/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/company id/i), { target: { value: '6' } })
    fireEvent.change(screen.getByLabelText(/primary contact id/i), { target: { value: '8' } })
    fireEvent.change(screen.getByLabelText(/value amount/i), { target: { value: '60000.00' } })
    fireEvent.change(screen.getByLabelText(/value currency/i), { target: { value: 'USD' } })
    fireEvent.change(screen.getByLabelText(/expected close date/i), { target: { value: '2026-05-02' } })
    fireEvent.click(screen.getByRole('button', { name: /save deal/i }))

    expect((await screen.findAllByText(/bluebird rollout/i)).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/qualified/i).length).toBeGreaterThan(0)

    fireEvent.change(screen.getByLabelText(/move stage/i), { target: { value: '3' } })
    fireEvent.click(screen.getByRole('button', { name: /move to stage/i }))

    expect(await screen.findByText(/deal moved to proposal/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/deals\/12\/stage$/), expect.objectContaining({ method: 'PATCH' }))
    })
  })
})
