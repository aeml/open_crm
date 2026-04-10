import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('tasks flow', () => {
  it('loads tasks, creates a task, and completes it', async () => {
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
            tasks: [
              {
                id: 51,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Call Morgan about rollout timing',
                description: 'Confirm launch window.',
                status: 'open',
                dueAt: '2026-04-15T11:00:00Z',
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
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [
              {
                id: 51,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Call Morgan about rollout timing',
                description: 'Confirm launch window.',
                status: 'open',
                dueAt: '2026-04-15T11:00:00Z',
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
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          data: {
            task: {
              id: 77,
              entityType: 'deal',
              entityId: 12,
              entityLabel: 'Bluebird Rollout',
              title: 'Prepare rollout checklist',
              description: 'Lock owners before kickoff.',
              status: 'open',
              dueAt: '2026-04-16T09:00:00Z',
              completedAt: '',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 201, action: 'task.created', summary: 'Task created' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            task: {
              id: 77,
              entityType: 'deal',
              entityId: 12,
              entityLabel: 'Bluebird Rollout',
              title: 'Prepare rollout checklist',
              description: 'Completed and handed off.',
              status: 'completed',
              dueAt: '2026-04-16T09:00:00Z',
              completedAt: '2026-04-10T14:15:00Z',
              assignedToUserId: 2,
              assignedToUserName: 'Alex Admin',
              createdByUserId: 1,
              createdByUserName: 'Demo Owner'
            },
            activities: [
              { id: 202, action: 'task.completed', summary: 'Task completed' }
            ]
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [
              {
                id: 77,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Prepare rollout checklist',
                description: 'Completed and handed off.',
                status: 'completed',
                dueAt: '2026-04-16T09:00:00Z',
                completedAt: '2026-04-10T14:15:00Z',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 0, completedCount: 1 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^tasks$/i })).toBeInTheDocument()
    expect(await screen.findByText(/call morgan about rollout timing/i)).toBeInTheDocument()
    expect(screen.getAllByText(/open tasks/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/showing 1 of 1 open tasks/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/search tasks/i), { target: { value: 'morgan' } })

    expect(await screen.findByText(/showing 1 of 1 open tasks/i)).toBeInTheDocument()

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&q=morgan/), expect.any(Object))
    })

    expect(screen.queryByLabelText(/entity id/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Prepare rollout checklist' } })
    fireEvent.change(screen.getByLabelText(/entity type/i), { target: { value: 'deal' } })
    fireEvent.change(screen.getByLabelText(/deal/i), { target: { value: '12' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Lock owners before kickoff.' } })
    fireEvent.change(screen.getByLabelText(/assigned to user id/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-16T09:00' } })
    fireEvent.click(screen.getByRole('button', { name: /save task/i }))

    expect(await screen.findByRole('heading', { name: /prepare rollout checklist/i })).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'completed' } })
    fireEvent.change(screen.getByLabelText(/completed at/i), { target: { value: '2026-04-10T14:15' } })
    fireEvent.change(screen.getAllByLabelText(/description/i)[1], { target: { value: 'Completed and handed off.' } })
    fireEvent.click(screen.getByRole('button', { name: /update task/i }))

    expect(await screen.findByText(/task completed/i)).toBeInTheDocument()
    expect(screen.getAllByText(/completed and handed off/i).length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: /show completed/i }))

    expect(await screen.findByRole('heading', { name: /^completed tasks$/i })).toBeInTheDocument()
    expect(screen.getAllByText(/prepare rollout checklist/i).length).toBeGreaterThan(0)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/77$/), expect.objectContaining({ method: 'PATCH' }))
    })
  })
})
