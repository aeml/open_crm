import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createCalendarBookingLink, listCalendarAvailability, listCalendarBookingLinks, updateCalendarAvailability, updateCalendarBookingLink } from '../lib/calendar'
import { listOrganizationUsers } from '../lib/users'
import { usePageTitle } from '../lib/use_page_title'

const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const defaultCalendarCapacity = { maxLinks: 100, maxMembers: 20, maxBlocks: 28 }

function defaultTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

function emptyBookingLinkForm(userId = '') {
  return {
    name: '',
    slug: '',
    description: '',
    durationMinutes: '30',
    bufferMinutes: '0',
    timezone: defaultTimezone(),
    assignmentMode: 'owner',
    isActive: true,
    memberUserIds: userId ? [String(userId)] : []
  }
}

const emptyAvailabilityDraft = {
  dayOfWeek: '1',
  startTime: '09:00',
  endTime: '17:00',
  timezone: defaultTimezone()
}

function formFromBookingLink(link) {
  return {
    name: link.name || '',
    slug: link.slug || '',
    description: link.description || '',
    durationMinutes: String(link.durationMinutes || 30),
    bufferMinutes: String(link.bufferMinutes || 0),
    timezone: link.timezone || defaultTimezone(),
    assignmentMode: link.assignmentMode || 'owner',
    isActive: link.isActive !== false,
    memberUserIds: Array.isArray(link.members) ? link.members.map((member) => String(member.userId)) : []
  }
}

function bookingLinkPayload(form) {
  return {
    name: form.name,
    slug: form.slug,
    description: form.description,
    durationMinutes: Number.parseInt(form.durationMinutes || '30', 10) || 30,
    bufferMinutes: Number.parseInt(form.bufferMinutes || '0', 10) || 0,
    timezone: form.timezone,
    assignmentMode: form.assignmentMode,
    isActive: form.isActive,
    memberUserIds: form.memberUserIds.map((userId) => Number.parseInt(userId, 10)).filter(Boolean)
  }
}

function minutesToTime(minutes) {
  const safeMinutes = Number.isFinite(minutes) ? minutes : 0
  const hours = Math.floor(safeMinutes / 60)
  const mins = safeMinutes % 60
  return `${String(hours).padStart(2, '0')}:${String(mins).padStart(2, '0')}`
}

function timeToMinutes(value) {
  const [hours, minutes] = String(value || '').split(':').map((part) => Number.parseInt(part, 10))
  if (!Number.isFinite(hours) || !Number.isFinite(minutes)) {
    return 0
  }
  return hours * 60 + minutes
}

function memberNames(link) {
  const members = Array.isArray(link.members) ? link.members : []
  if (members.length === 0) {
    return 'Current user'
  }
  return members.map((member) => `${member.firstName || ''} ${member.lastName || ''}`.trim() || member.email).join(', ')
}

function availabilityLabel(block) {
  return `${days[block.dayOfWeek] || 'Day'} ${minutesToTime(block.startMinute)}-${minutesToTime(block.endMinute)} ${block.timezone || 'UTC'}`
}

export function SettingsCalendarRoute() {
  const { session, canWrite: canManage } = useAuth()
  usePageTitle('Booking Links')
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const [links, setLinks] = useState([])
  const [users, setUsers] = useState([])
  const [availability, setAvailability] = useState([])
  const [capacity, setCapacity] = useState(defaultCalendarCapacity)
  const [form, setForm] = useState(() => emptyBookingLinkForm(currentUserId))
  const [editingId, setEditingId] = useState(null)
  const [availabilityDraft, setAvailabilityDraft] = useState(emptyAvailabilityDraft)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSavingLink, setIsSavingLink] = useState(false)
  const [isSavingAvailability, setIsSavingAvailability] = useState(false)
  const selectedMembers = useMemo(() => new Set(form.memberUserIds), [form.memberUserIds])

  async function loadCalendarSettings({ signal } = {}) {
    setIsLoading(true)
    try {
      const [linkCatalog, availabilityCatalog, nextUsers] = await Promise.all([
        listCalendarBookingLinks({ signal }),
        listCalendarAvailability({ signal }),
        listOrganizationUsers({ signal })
      ])
      setLinks(linkCatalog.links)
      setAvailability(availabilityCatalog.blocks)
      setCapacity({
        maxLinks: linkCatalog.capacity.maxLinks || defaultCalendarCapacity.maxLinks,
        maxMembers: linkCatalog.capacity.maxMembers || defaultCalendarCapacity.maxMembers,
        maxBlocks: availabilityCatalog.capacity.maxBlocks || defaultCalendarCapacity.maxBlocks
      })
      setUsers(nextUsers)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load calendar settings.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadCalendarSettings({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  useEffect(() => {
    if (currentUserId && form.memberUserIds.length === 0 && !editingId) {
      setForm((current) => ({ ...current, memberUserIds: [currentUserId] }))
    }
  }, [currentUserId, editingId, form.memberUserIds.length])

  function resetForm() {
    setEditingId(null)
    setForm(emptyBookingLinkForm(currentUserId))
  }

  function startEdit(link) {
    setEditingId(link.id)
    setForm(formFromBookingLink(link))
    setStatus('')
  }

  function toggleMember(userId, checked) {
    const value = String(userId)
    setForm((current) => {
      if (!checked) {
        return { ...current, memberUserIds: current.memberUserIds.filter((entry) => entry !== value) }
      }
      if (current.memberUserIds.includes(value) || current.memberUserIds.length >= capacity.maxMembers) {
        return current
      }
      return { ...current, memberUserIds: [...current.memberUserIds, value] }
    })
  }

  async function handleSaveBookingLink(event) {
    event.preventDefault()
    setIsSavingLink(true)
    setStatus('')
    try {
      const payload = bookingLinkPayload(form)
      if (editingId) {
        const updated = await updateCalendarBookingLink(editingId, payload)
        setLinks((current) => current.map((link) => (link.id === editingId ? updated : link)))
        setStatus('Booking link updated.')
      } else {
        const created = await createCalendarBookingLink(payload)
        setLinks((current) => [...current, created])
        setStatus('Booking link created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save booking link.')
    } finally {
      setIsSavingLink(false)
    }
  }

  function addAvailabilityBlock() {
    if (availability.length >= capacity.maxBlocks) {
      setError(`Availability is limited to ${capacity.maxBlocks} blocks.`)
      return
    }
    const block = {
      dayOfWeek: Number.parseInt(availabilityDraft.dayOfWeek, 10),
      startMinute: timeToMinutes(availabilityDraft.startTime),
      endMinute: timeToMinutes(availabilityDraft.endTime),
      timezone: availabilityDraft.timezone || defaultTimezone()
    }
    if (block.endMinute <= block.startMinute) {
      setError('Availability end time must be after the start time.')
      return
    }
    setAvailability((current) => [...current, block])
    setError('')
  }

  async function saveAvailability() {
    setIsSavingAvailability(true)
    setStatus('')
    try {
      const blocks = await updateCalendarAvailability({ blocks: availability.map((block) => ({
        dayOfWeek: block.dayOfWeek,
        startMinute: block.startMinute,
        endMinute: block.endMinute,
        timezone: block.timezone || defaultTimezone()
      })) })
      setAvailability(blocks)
      setStatus('Availability updated.')
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update availability.')
    } finally {
      setIsSavingAvailability(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Booking links</h2>
              <p>Create meeting links that use your team's saved weekly availability. Public scheduling and calendar sync can build on this foundation.</p>
              <p className="field-hint">{links.length} of {capacity.maxLinks} booking links</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading calendar settings...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadCalendarSettings()} retryLabel="Retry calendar settings" /> : null}
          <div className="record-list" role="list" aria-label="Booking links">
            {!isLoading && links.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No booking links yet.</p>
                  <p className="field-hint">Create a link before adding public scheduling pages.</p>
                </div>
              </article>
            ) : links.map((link) => (
              <article className="record-row" key={link.id} role="listitem">
                <div>
                  <h3>{link.name}</h3>
                  <p className="field-hint">/{link.slug} · {link.durationMinutes} min · {link.assignmentMode === 'round_robin' ? 'Round robin' : 'Single owner'} · {link.isActive ? 'Active' : 'Inactive'}</p>
                  <p className="field-hint">Hosts: {memberNames(link)}</p>
                </div>
                {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(link)}>Edit</Button> : null}
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSaveBookingLink}>
            <div>
              <h2>{editingId ? 'Edit booking link' : 'New booking link'}</h2>
              <p className="field-hint">Round-robin links rotate across selected hosts once public booking is added.</p>
            </div>
            <Field label="Booking name">
              <input className="text-input" maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Discovery call" required />
            </Field>
            <Field label="Booking slug">
              <input className="text-input" maxLength={80} value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} placeholder="discovery-call" />
            </Field>
            <Field label="Description">
              <textarea className="text-input" maxLength={1000} rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <Field label="Duration minutes">
              <input className="text-input" min="5" max="480" type="number" value={form.durationMinutes} onChange={(event) => setForm({ ...form, durationMinutes: event.target.value })} required />
            </Field>
            <Field label="Buffer minutes">
              <input className="text-input" min="0" max="240" type="number" value={form.bufferMinutes} onChange={(event) => setForm({ ...form, bufferMinutes: event.target.value })} />
            </Field>
            <Field label="Timezone">
              <input className="text-input" maxLength={100} value={form.timezone} onChange={(event) => setForm({ ...form, timezone: event.target.value })} required />
            </Field>
            <Field label="Assignment mode">
              <select className="text-input" value={form.assignmentMode} onChange={(event) => setForm({ ...form, assignmentMode: event.target.value })}>
                <option value="owner">Single owner</option>
                <option value="round_robin">Round robin</option>
              </select>
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active booking link
            </label>
            <div className="card-stack">
              <h3>Team members</h3>
              <p className="field-hint">{form.memberUserIds.length} of {capacity.maxMembers} hosts selected</p>
              {users.length === 0 ? <p className="field-hint">No team members loaded. The current user will be used by default.</p> : users.map((user) => (
                <label className="field-hint" key={user.id}>
                  <input type="checkbox" checked={selectedMembers.has(String(user.id))} disabled={!selectedMembers.has(String(user.id)) && form.memberUserIds.length >= capacity.maxMembers} onChange={(event) => toggleMember(user.id, event.target.checked)} /> {user.firstName} {user.lastName} ({user.email})
                </label>
              ))}
            </div>
            <div>
              <Button type="submit" disabled={isSavingLink || (!editingId && links.length >= capacity.maxLinks)}>{isSavingLink ? 'Saving...' : editingId ? 'Save booking link' : 'Create booking link'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
              {!editingId && links.length >= capacity.maxLinks ? <p className="field-hint">Edit an existing link to recover space before creating another.</p> : null}
            </div>
          </form>
        </Card>
      ) : null}

      {canManage ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>Weekly availability</h2>
              <p className="field-hint">These blocks feed booking-link availability until external free/busy sync is added.</p>
              <p className="field-hint">{availability.length} of {capacity.maxBlocks} blocks</p>
            </div>
            <div className="record-list" role="list" aria-label="Weekly availability">
              {availability.length === 0 ? (
                <article className="record-row" role="listitem"><p>No availability blocks saved.</p></article>
              ) : availability.map((block, index) => (
                <article className="record-row" key={`${block.dayOfWeek}-${block.startMinute}-${block.endMinute}-${index}`} role="listitem">
                  <p>{availabilityLabel(block)}</p>
                  <Button className="button-secondary" type="button" onClick={() => setAvailability((current) => current.filter((_, entryIndex) => entryIndex !== index))}>Remove</Button>
                </article>
              ))}
            </div>
            <div className="auth-form">
              <Field label="Availability day">
                <select className="text-input" value={availabilityDraft.dayOfWeek} onChange={(event) => setAvailabilityDraft({ ...availabilityDraft, dayOfWeek: event.target.value })}>
                  {days.map((day, index) => <option key={day} value={index}>{day}</option>)}
                </select>
              </Field>
              <Field label="Availability start">
                <input className="text-input" type="time" value={availabilityDraft.startTime} onChange={(event) => setAvailabilityDraft({ ...availabilityDraft, startTime: event.target.value })} />
              </Field>
              <Field label="Availability end">
                <input className="text-input" type="time" value={availabilityDraft.endTime} onChange={(event) => setAvailabilityDraft({ ...availabilityDraft, endTime: event.target.value })} />
              </Field>
              <Field label="Availability timezone">
                <input className="text-input" maxLength={100} value={availabilityDraft.timezone} onChange={(event) => setAvailabilityDraft({ ...availabilityDraft, timezone: event.target.value })} />
              </Field>
              <div>
                <Button className="button-secondary" type="button" onClick={addAvailabilityBlock} disabled={availability.length >= capacity.maxBlocks}>Add availability block</Button>
                <Button type="button" onClick={saveAvailability} disabled={isSavingAvailability}>{isSavingAvailability ? 'Saving...' : 'Save availability'}</Button>
              </div>
            </div>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
