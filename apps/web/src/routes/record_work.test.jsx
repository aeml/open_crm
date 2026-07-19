import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RecordWorkCards } from './record_work'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('record collaboration controls', () => {
  it('loads followers, follows idempotently, and inserts explicit teammate mentions', async () => {
    const fetchMock = vi.fn(async (input, options = {}) => {
      const url = String(input)
      if (url.includes('/api/record-followers') && options.method === 'PUT') {
        return { ok: true, json: async () => ({ data: { following: true, followers: [{ userId: 2, userName: 'Casey Example', userEmail: 'casey@example.test' }] } }) }
      }
      if (url.includes('/api/record-followers')) {
        return { ok: true, json: async () => ({ data: { following: false, followers: [] } }) }
      }
      throw new Error(`Unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    const setNoteBody = vi.fn()

    render(
      <RecordWorkCards
        activities={[]}
        canWrite
        entityId={17}
        entityType="contact"
        noteBody="Please review"
        notes={[]}
        onCreateNote={vi.fn((event) => event.preventDefault())}
        onCreateTask={vi.fn((event) => event.preventDefault())}
        onOpenTasks={vi.fn()}
        onSetNoteBody={setNoteBody}
        onSetTaskForm={vi.fn()}
        taskForm={{ title: '', description: '', assignedToUserId: '2', dueAt: '' }}
        tasks={[]}
        users={[{ id: 2, email: 'casey@example.test', firstName: 'Casey', lastName: 'Example' }]}
      />
    )

    expect(screen.getByText(/follow this record to receive note updates/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '@casey@example.test' }))
    expect(setNoteBody).toHaveBeenCalledWith('Please review @casey@example.test ')

    fireEvent.click(screen.getByRole('button', { name: 'Followers' }))
    expect(await screen.findByRole('button', { name: 'Follow' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Follow' }))
    expect(await screen.findByRole('button', { name: 'Following' })).toBeInTheDocument()
    expect(screen.getByText(/1 active follower/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/record-followers/me?'), expect.objectContaining({ method: 'PUT' }))
    })
  })
})
