import { describe, expect, it } from 'vitest'
import {
  emptyTaskListMessage,
  sortCompletedTasks,
  sortOpenTasks,
  taskCountLabel,
  taskLabels,
  taskListHeading
} from './task_view'

describe('task view model', () => {
  it('keeps general and service wording exact', () => {
    const general = taskLabels('general')
    const services = taskLabels('services')

    expect(general).toMatchObject({
      collection: 'Tasks',
      createDescription: 'Assign work against a contact, company, or deal.',
      titlePlural: 'Tasks',
      companyLabel: 'Company',
      dealOption: 'Deal'
    })
    expect(services).toMatchObject({
      collection: 'Service Tasks',
      createDescription: 'Assign work against a contact, client, or job.',
      titlePlural: 'Service tasks',
      companyLabel: 'Client',
      dealOption: 'Job'
    })
  })

  it('derives headings, counts, and empty messages from the same view', () => {
    const labels = taskLabels('general')

    expect(taskListHeading('open', 'overdue', labels)).toBe('Overdue tasks')
    expect(taskCountLabel('open', 'overdue', labels)).toBe('overdue tasks')
    expect(emptyTaskListMessage('open', 'overdue', labels)).toBe('No overdue tasks match the current filters.')
    expect(emptyTaskListMessage('open', 'all', labels)).toBe('No open tasks yet.')
  })

  it('orders open and completed work with stable missing-date fallbacks', () => {
    const tasks = [
      { id: 3, dueAt: '', completedAt: '' },
      { id: 2, dueAt: '2026-07-24T09:00:00Z', completedAt: '2026-07-21T09:00:00Z' },
      { id: 1, dueAt: '2026-07-23T09:00:00Z', completedAt: '2026-07-22T09:00:00Z' }
    ]

    expect(sortOpenTasks(tasks).map(({ id }) => id)).toEqual([1, 2, 3])
    expect(sortCompletedTasks(tasks).map(({ id }) => id)).toEqual([1, 2, 3])
  })
})
