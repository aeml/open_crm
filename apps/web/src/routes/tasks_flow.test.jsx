import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
                title: 'Confirm installer arrival window',
                description: 'Need final arrival confirmation.',
                status: 'open',
                dueAt: '2099-04-16T09:00:00Z',
                completedAt: '',
                assignedToUserId: 1,
                assignedToUserName: 'Demo Owner',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              },
              {
                id: 52,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Call Morgan about rollout timing',
                description: 'Confirm launch window.',
                status: 'open',
                dueAt: '2026-04-10T11:00:00Z',
                completedAt: '',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              },
              {
                id: 53,
                entityType: 'contact',
                entityId: 8,
                entityLabel: 'Ava Stone',
                title: 'Send onboarding packet',
                description: 'Share intake forms.',
                status: 'open',
                dueAt: '',
                completedAt: '',
                assignedToUserId: 2,
                assignedToUserName: 'Alex Admin',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 3, openCount: 3, completedCount: 0 }
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
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
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
                id: 51,
                entityType: 'deal',
                entityId: 12,
                entityLabel: 'Bluebird Rollout',
                title: 'Call Morgan about rollout timing',
                description: 'Confirm launch window.',
                status: 'open',
                dueAt: '2026-04-10T11:00:00Z',
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
              entityType: 'contact',
              entityId: 8,
              entityLabel: 'Ava Stone',
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
                id: 78,
                entityType: 'contact',
                entityId: 8,
                entityLabel: 'Ava Stone',
                title: 'Collect signed agreement',
                description: 'Received yesterday.',
                status: 'completed',
                dueAt: '2026-04-09T10:00:00Z',
                completedAt: '2026-04-09T16:30:00Z',
                assignedToUserId: 1,
                assignedToUserName: 'Demo Owner',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              },
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
            meta: { page: 1, pageSize: 20, total: 2, openCount: 0, completedCount: 2 }
          }
        })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^tasks$/i })).toBeInTheDocument()
    expect(await screen.findByText(/call morgan about rollout timing/i)).toBeInTheDocument()
    expect(screen.getAllByText(/open tasks/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/showing 3 of 3 open tasks/i)).toBeInTheDocument()

    const initialTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(initialTaskList).getAllByRole('button').map((button) => button.textContent)).toEqual([
      'Call Morgan about rollout timing',
      'Confirm installer arrival window',
      'Send onboarding packet'
    ])

    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'contact' } })

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const contactTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(contactTaskList).getByText(/send onboarding packet/i)).toBeInTheDocument()
    expect(within(contactTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(within(contactTaskList).queryByText(/confirm installer arrival window/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: '1' } })

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const ownerTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(ownerTaskList).getByText(/confirm installer arrival window/i)).toBeInTheDocument()
    expect(within(ownerTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/task view/i), { target: { value: 'upcoming' } })

    expect(await screen.findByRole('heading', { name: /^upcoming tasks$/i })).toBeInTheDocument()
    const taskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(taskList).getByText(/confirm installer arrival window/i)).toBeInTheDocument()
    expect(within(taskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(screen.getByText(/showing 1 of 3 upcoming tasks/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task view/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/search tasks/i), { target: { value: 'morgan' } })

    expect(await screen.findByText(/showing 1 of 1 open tasks/i)).toBeInTheDocument()

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&q=morgan/), expect.any(Object))
    })

    expect(screen.queryByLabelText(/entity id/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^deal$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/entity type/i), { target: { value: 'company' } })
    expect(screen.getByLabelText(/^company$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^deal$/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/entity type/i), { target: { value: 'contact' } })
    expect(screen.getByLabelText(/^contact$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^company$/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Prepare rollout checklist' } })
    fireEvent.change(screen.getByLabelText(/^contact$/i), { target: { value: '8' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Lock owners before kickoff.' } })
    fireEvent.change(screen.getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-16T09:00' } })
    fireEvent.click(screen.getByRole('button', { name: /save task/i }))

    expect(await screen.findByRole('heading', { name: /prepare rollout checklist/i })).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    expect(screen.queryAllByLabelText(/assigned to user id/i)).toHaveLength(0)
    expect(screen.getAllByLabelText(/^assigned to$/i).length).toBeGreaterThan(0)

    fireEvent.change(screen.getAllByLabelText(/^assigned to$/i)[1], { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'completed' } })
    fireEvent.change(screen.getByLabelText(/completed at/i), { target: { value: '2026-04-10T14:15' } })
    fireEvent.change(screen.getAllByLabelText(/description/i)[1], { target: { value: 'Completed and handed off.' } })
    fireEvent.click(screen.getByRole('button', { name: /update task/i }))

    expect(await screen.findByText(/task completed/i)).toBeInTheDocument()
    expect(screen.getAllByText(/completed and handed off/i).length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: /show completed/i }))

    expect(await screen.findByRole('heading', { name: /^completed tasks$/i })).toBeInTheDocument()
    expect(screen.getAllByText(/prepare rollout checklist/i).length).toBeGreaterThan(0)
    const completedTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(completedTaskList).getAllByRole('button').map((button) => button.textContent)).toEqual([
      'Prepare rollout checklist',
      'Collect signed agreement'
    ])

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/77$/), expect.objectContaining({ method: 'PATCH' }))
    })
  })

  it('uses service task wording for service businesses', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'services' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
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
        json: async () => ({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^service tasks$/i })).toBeInTheDocument()
    expect(screen.getAllByText(/open service tasks/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/completed service tasks/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/search service tasks/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /new service task/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/linked record filter/i)).toBeInTheDocument()
    expect(screen.getAllByRole('option', { name: /^job$/i }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('option', { name: /^client$/i }).length).toBeGreaterThan(0)
  })
})
