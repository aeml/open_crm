import { useLayoutEffect, useRef, useState } from 'react'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { ControlledTextField, Field } from '../components/ui/field'
import { getRecordFollowers, setRecordFollowing } from '../lib/collaboration'

export function RecordWorkCards({
  activities,
  activityMeta = { hasMore: false, nextCursor: '' },
  activityAria = 'Activity list',
  canWrite,
  entityId,
  entityType,
  isCreatingNote = false,
  isCreatingTask = false,
  isLoading = false,
  isLoadingOlderActivities = false,
  isLoadingOlderNotes = false,
  noteBody,
  notes,
  noteMeta = { hasMore: false, nextCursor: '' },
  notesAria = 'Notes list',
  onCreateNote,
  onCreateTask,
  onLoadOlderActivities,
  onLoadOlderNotes,
  onOpenTasks,
  onSetNoteBody,
  onSetTaskForm,
  taskForm,
  tasks,
  tasksAria = 'Contact tasks list',
  users
}) {
  const activeRecordRef = useRef({ entityId, entityType })
  if (activeRecordRef.current.entityId !== entityId || activeRecordRef.current.entityType !== entityType) {
    activeRecordRef.current = { entityId, entityType }
  }
  const [followerState, setFollowerState] = useState({ following: false, followers: [] })
  const [followersLoaded, setFollowersLoaded] = useState(false)
  const [followError, setFollowError] = useState('')
  const [isUpdatingFollow, setIsUpdatingFollow] = useState(false)

  useLayoutEffect(() => {
    setFollowerState({ following: false, followers: [] })
    setFollowersLoaded(false)
    setFollowError('')
    setIsUpdatingFollow(false)
  }, [entityId, entityType])

  async function toggleFollowing() {
    const record = activeRecordRef.current
    setIsUpdatingFollow(true)
    try {
      if (!followersLoaded) {
        const result = await getRecordFollowers(record)
        if (activeRecordRef.current !== record) return
        if (result.entityId !== record.entityId || result.entityType !== record.entityType) throw new Error('Unable to load record followers.')
        setFollowerState(result)
        setFollowersLoaded(true)
        setFollowError('')
        return
      }
      const result = await setRecordFollowing({ ...record, following: !followerState.following })
      if (activeRecordRef.current !== record) return
      if (result.entityId !== record.entityId || result.entityType !== record.entityType) throw new Error('Unable to update following.')
      setFollowerState(result)
      setFollowError('')
    } catch (error) {
      if (activeRecordRef.current === record) setFollowError(error.message || 'Unable to update following.')
    } finally {
      if (activeRecordRef.current === record) setIsUpdatingFollow(false)
    }
  }

  function insertMention(email) {
    const separator = noteBody && !noteBody.endsWith(' ') ? ' ' : ''
    onSetNoteBody(`${noteBody}${separator}@${email} `)
  }

  return (
    <>
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h3>Notes</h3>
              <p className="field-hint">
                {!followersLoaded
                  ? 'Follow this record to receive note updates'
                  : followerState.followers.length === 0
                  ? 'No active followers'
                  : `${followerState.followers.length} active follower${followerState.followers.length === 1 ? '' : 's'}`}
              </p>
            </div>
            <Button className="button-secondary" type="button" onClick={toggleFollowing} disabled={isLoading || isUpdatingFollow || !entityId}>
              {isUpdatingFollow ? 'Loading…' : !followersLoaded ? 'Followers' : followerState.following ? 'Following' : 'Follow'}
            </Button>
          </div>
          {isLoading ? <p className="field-hint" role="status">Loading notes, tasks, and activity…</p> : null}
          {followError ? <p className="field-hint" role="alert">{followError}</p> : null}
          {canWrite && !isLoading ? (
            <form className="auth-form" onSubmit={onCreateNote}>
              <Field label="New note">
                <textarea className="text-input" value={noteBody} onChange={(event) => onSetNoteBody(event.target.value)} rows={4} />
              </Field>
              {users.length > 0 ? (
                <div>
                  <p className="field-hint">Mention a teammate</p>
                  <div className="button-row" aria-label="Mention a teammate">
                    {users.map((user) => (
                      <button className="button-ghost" key={user.id} type="button" onClick={() => insertMention(user.email)}>
                        @{user.email}
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}
              <Button type="submit" disabled={isCreatingNote}>{isCreatingNote ? 'Adding…' : 'Add note'}</Button>
            </form>
          ) : null}
          <div className="record-list" role="list" aria-label={notesAria}>
            {notes.map((note) => (
              <article className="record-row" key={note.id} role="listitem">
                <div>
                  <p>{note.body}</p>
                  <p className="field-hint">{note.createdByUserName || 'Unknown author'}</p>
                </div>
              </article>
            ))}
          </div>
          {noteMeta?.hasMore ? (
            <Button className="button-secondary" type="button" onClick={onLoadOlderNotes} disabled={isLoadingOlderNotes}>
              {isLoadingOlderNotes ? 'Loading older notes…' : 'Load older notes'}
            </Button>
          ) : null}
        </div>
      </Card>
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <h3>Tasks</h3>
            <Button className="button-secondary" type="button" onClick={onOpenTasks}>Open in tasks</Button>
          </div>
          {canWrite && !isLoading ? (
            <form className="auth-form" onSubmit={onCreateTask}>
              <ControlledTextField form={taskForm} label="Task title" name="title" required setForm={onSetTaskForm} />
              <ControlledTextField form={taskForm} label="Task description" multiline name="description" rows={3} setForm={onSetTaskForm} />
              <Field label="Assigned to">
                <select className="text-input" value={taskForm.assignedToUserId} onChange={(event) => onSetTaskForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
                  {users.map((user) => <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>)}
                </select>
              </Field>
              <ControlledTextField form={taskForm} label="Due at" name="dueAt" setForm={onSetTaskForm} type="datetime-local" />
              <Button type="submit" disabled={isCreatingTask}>{isCreatingTask ? 'Saving…' : 'Save task'}</Button>
            </form>
          ) : null}
          <div className="record-list" role="list" aria-label={tasksAria}>
            {tasks.map((task) => (
              <article className="record-row" key={task.id} role="listitem">
                <div>
                  <p>{task.title}</p>
                  <p className="field-hint">{task.assignedToUserName || 'Unassigned'}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
      <Card>
        <div className="card-stack">
          <h3>Activity</h3>
          <ActivityTimeline activities={activities} ariaLabel={activityAria} />
          {activityMeta?.hasMore ? (
            <Button className="button-secondary" type="button" onClick={onLoadOlderActivities} disabled={isLoadingOlderActivities}>
              {isLoadingOlderActivities ? 'Loading older activity…' : 'Load older activity'}
            </Button>
          ) : null}
        </div>
      </Card>
    </>
  )
}
