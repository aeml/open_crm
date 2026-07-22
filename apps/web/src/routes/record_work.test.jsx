import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RecordWorkCards } from './record_work'

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function recordWorkProps(entityId) {
  return {
  activities: [],
  activityMeta: { hasMore: false, nextCursor: '' },
    canWrite: true,
    entityId,
    entityType: 'deal',
    noteBody: '',
  notes: [],
  noteMeta: { hasMore: false, nextCursor: '' },
    onCreateNote: vi.fn((event) => event.preventDefault()),
  onCreateTask: vi.fn((event) => event.preventDefault()),
  onLoadOlderActivities: vi.fn(),
  onLoadOlderNotes: vi.fn(),
    onOpenTasks: vi.fn(),
    onSetNoteBody: vi.fn(),
    onSetTaskForm: vi.fn(),
    taskForm: { title: '', description: '', assignedToUserId: '2', dueAt: '' },
    tasks: [],
    users: []
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('record collaboration controls', () => {
  it('discloses and triggers older note and activity history', () => {
    const props = {
      ...recordWorkProps(11),
      activityMeta: { hasMore: true, nextCursor: 'activity-cursor' },
      noteMeta: { hasMore: true, nextCursor: 'note-cursor' }
    }
    render(<RecordWorkCards {...props} />)

    fireEvent.click(screen.getByRole('button', { name: 'Load older notes' }))
    fireEvent.click(screen.getByRole('button', { name: 'Load older activity' }))

    expect(props.onLoadOlderNotes).toHaveBeenCalledTimes(1)
    expect(props.onLoadOlderActivities).toHaveBeenCalledTimes(1)
  })

  it('keeps record work mutations unavailable until the active snapshot has loaded', () => {
    const props = recordWorkProps(11)
    const { rerender } = render(<RecordWorkCards {...props} isLoading />)

    expect(screen.getByRole('status')).toHaveTextContent('Loading notes, tasks, and activity')
    expect(screen.getByRole('button', { name: 'Followers' })).toBeDisabled()
    expect(screen.queryByLabelText('New note')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Task title')).not.toBeInTheDocument()

    rerender(<RecordWorkCards {...props} isLoading={false} />)

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Followers' })).toBeEnabled()
    expect(screen.getByLabelText('New note')).toBeInTheDocument()
    expect(screen.getByLabelText('Task title')).toBeInTheDocument()
  })

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

  it('rejects a follower response for a record that is no longer active', async () => {
    const alphaFollowers = deferred()
    const fetchMock = vi.fn((input) => {
      const url = new URL(String(input), 'http://localhost')
      if (url.searchParams.get('entityId') === '11') return alphaFollowers.promise
      return Promise.resolve({ ok: true, json: async () => ({ data: { following: true, followers: [{ userId: 3 }] } }) })
    })
    vi.stubGlobal('fetch', fetchMock)
    const { rerender } = render(<RecordWorkCards {...recordWorkProps(11)} />)

    fireEvent.click(screen.getByRole('button', { name: 'Followers' }))
    rerender(<RecordWorkCards {...recordWorkProps(12)} />)
    fireEvent.click(screen.getByRole('button', { name: 'Followers' }))
    expect(await screen.findByText('1 active follower')).toBeInTheDocument()

    await act(async () => {
      alphaFollowers.resolve({ ok: true, json: async () => ({ data: { following: true, followers: [{ userId: 4 }, { userId: 5 }] } }) })
      await alphaFollowers.promise
    })
    expect(screen.getByText('1 active follower')).toBeInTheDocument()
    expect(screen.queryByText('2 active followers')).not.toBeInTheDocument()
  })
})
