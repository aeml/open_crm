import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'
import { archiveContact, createContact, getContact, listContacts, updateContact } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { createNote } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'

const emptyForm = {
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  jobTitle: '',
  status: 'lead'
}

function formatAddress(contact = {}) {
  const street = [contact.addressLine1, contact.addressLine2].filter(Boolean).join(', ')
  const locality = [contact.city, contact.state, contact.postalCode].filter(Boolean).join(', ')
  return [street, locality, contact.country].filter(Boolean).join(' | ')
}

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}

function fullName(contact) {
  return `${contact.firstName || ''} ${contact.lastName || ''}`.trim()
}

function contactFormValues(contact) {
  return {
    firstName: contact.firstName || '',
    lastName: contact.lastName || '',
    email: contact.email || '',
    phone: contact.phone || '',
    addressLine1: contact.addressLine1 || '',
    addressLine2: contact.addressLine2 || '',
    city: contact.city || '',
    state: contact.state || '',
    postalCode: contact.postalCode || '',
    country: contact.country || '',
    jobTitle: contact.jobTitle || '',
    status: contact.status || 'lead'
  }
}

function formatActivityTimestamp(createdAt) {
  if (!createdAt) {
    return 'Time unavailable'
  }

  const parsed = new Date(createdAt)
  if (Number.isNaN(parsed.getTime())) {
    return 'Time unavailable'
  }

  return parsed.toLocaleString()
}

function duplicateSearchTerm(message, fallback = '') {
  const text = String(message || '')
  const marker = text.toLowerCase().lastIndexOf('duplicate contact:')
  if (marker >= 0) {
    const candidate = text.slice(marker + 'duplicate contact:'.length).split('(')[0].trim()
    if (candidate) {
      return candidate
    }
  }
  return String(fallback || '').trim()
}

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

function relatedPipelineLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return { plural: 'Jobs', singular: 'job' }
  }
  if (businessType === 'product-sales') {
    return { plural: 'Opportunities', singular: 'opportunity' }
  }
  return { plural: 'Deals', singular: 'deal' }
}

export function ContactsRoute() {
  const navigate = useNavigate()
  const { contactId } = useParams()
  const { session, businessProfile } = useAuth()
  const routeContactId = Number.parseInt(contactId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const pipelineLabels = relatedPipelineLabels(businessType)
  const [mode, setMode] = useState('list')
  const [contacts, setContacts] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState('')
  const [selectedContactId, setSelectedContactId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [error, setError] = useState('')
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)

  const selectedContact = detail?.contact || null
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const selectedActivities = detail?.activities || []

  async function loadContacts(nextSearch = '') {
    const data = await listContacts(nextSearch)

    if (Array.isArray(data?.contacts)) {
      setContacts(data.contacts)
      setMeta(data.meta || { page: 1, pageSize: 20, total: data.contacts.length })
      return
    }

    if (data?.contact) {
      const entry = data.contact
      setContacts([entry])
      setMeta({ page: 1, pageSize: 20, total: 1 })
      setDetailCache((current) => ({ ...current, [entry.id]: data }))
      return
    }

    setContacts([])
    setMeta({ page: 1, pageSize: 20, total: 0 })
  }

  async function loadUserOptions() {
    const nextUsers = await listOrganizationUsers()
    setUserOptions(nextUsers)
    setTaskForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      try {
        await Promise.all([loadContacts(''), loadUserOptions()])
        if (!cancelled) {
          setError('')
          setDuplicateSearch('')
          setDuplicateCandidate(null)
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load contacts.')
        }
      }
    }

    run()
    return () => {
      cancelled = true
    }
  }, [])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    try {
      await loadContacts(value)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (loadError) {
      setError(loadError.message || 'Unable to load contacts.')
    }
  }

  async function handleDuplicateSearch() {
    if (!duplicateSearch) {
      return
    }

    setSearch(duplicateSearch)
    try {
      await loadContacts(duplicateSearch)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load contacts.')
    }
  }

  function handleOpenDuplicate() {
    if (!duplicateCandidate?.id) {
      return
    }
    navigate(`/contacts/${duplicateCandidate.id}`)
  }

  function handleOpenDeal(dealID) {
    navigate(`/deals/${dealID}`)
  }

  function handleCreateRelatedDeal() {
    if (!selectedContactId) {
      return
    }
    const params = new URLSearchParams({ primaryContactId: String(selectedContactId) })
    navigate(`/deals?${params.toString()}`)
  }

  async function handleOpenContact(contact) {
    const contactID = contact.id
    const cached = detailCache[contactID]
    if (cached) {
      setSelectedContactId(contactID)
      setDetail(cached)
      setForm(contactFormValues(cached.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/contacts/${contactID}`)
      return
    }

    try {
      const [data, taskData, dealData] = await Promise.all([
        getContact(contactID),
        listTasks({ status: 'open', entityType: 'contact', entityId: contactID }),
        listDeals({ primaryContactId: contactID })
      ])
      const detailData = { ...data, tasks: taskData.tasks || [], deals: dealData.deals || [] }
      setDetailCache((current) => ({ ...current, [contactID]: detailData }))
      setSelectedContactId(contactID)
      setDetail(detailData)
      setForm(contactFormValues(data.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/contacts/${contactID}`)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load contact.')
    }
  }

  useEffect(() => {
    let cancelled = false

    async function openRouteContact() {
      if (!Number.isInteger(routeContactId) || routeContactId <= 0) {
        if (selectedContactId || mode === 'detail') {
          setSelectedContactId(null)
          setDetail(null)
          setForm(emptyForm)
          setNoteBody('')
          setTaskForm(emptyTaskForm)
          setMode('list')
        }
        return
      }

      if (selectedContactId === routeContactId && detail?.contact?.id === routeContactId) {
        return
      }

      const cached = detailCache[routeContactId]
      if (cached) {
        setSelectedContactId(routeContactId)
        setDetail(cached)
        setForm(contactFormValues(cached.contact))
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
        return
      }

      try {
        const [data, taskData, dealData] = await Promise.all([
          getContact(routeContactId),
          listTasks({ status: 'open', entityType: 'contact', entityId: routeContactId }),
          listDeals({ primaryContactId: routeContactId })
        ])
        if (cancelled) {
          return
        }
        const detailData = { ...data, tasks: taskData.tasks || [], deals: dealData.deals || [] }
        setDetailCache((current) => ({ ...current, [routeContactId]: detailData }))
        setSelectedContactId(routeContactId)
        setDetail(detailData)
        setForm(contactFormValues(detailData.contact))
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load contact.')
        }
      }
    }

    openRouteContact()
    return () => {
      cancelled = true
    }
  }, [detail, detailCache, mode, routeContactId, selectedContactId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createContact(form)
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [], deals: [] }
      setDetailCache((current) => ({ ...current, [data.contact.id]: detailData }))
      setContacts((current) => [...current, data.contact])
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedContactId(data.contact.id)
      setDetail(detailData)
      setForm(contactFormValues(detailData.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/contacts/${data.contact.id}`)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to create contact.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedContactId) {
      return
    }

    try {
      const data = await updateContact(selectedContactId, form)
      const detailData = {
        ...data,
        notes: detail?.notes || data.notes || [],
        tasks: detail?.tasks || data.tasks || [],
        deals: detail?.deals || []
      }
      setDetailCache((current) => ({ ...current, [selectedContactId]: detailData }))
      setContacts((current) => current.map((entry) => (entry.id === selectedContactId ? data.contact : entry)))
      setDetail(detailData)
      setForm(contactFormValues(data.contact))
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to update contact.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleArchive() {
    if (!selectedContactId) {
      return
    }

    try {
      await archiveContact(selectedContactId)
      setContacts((current) => current.filter((entry) => entry.id !== selectedContactId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      setDetail((current) => {
        if (!current?.contact?.id) {
          return null
        }
        const next = { ...detailCache }
        delete next[current.contact.id]
        setDetailCache(next)
        return null
      })
      setSelectedContactId(null)
      setForm(emptyForm)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('list')
      navigate('/companies')
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive contact.')
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    if (!selectedContactId || !noteBody.trim()) {
      return
    }

    try {
      const data = await createNote({
        entityType: 'contact',
        entityId: selectedContactId,
        body: noteBody.trim()
      })
      setDetail((current) => {
        if (!current) {
          return current
        }
        const next = {
          ...current,
          notes: [data.note, ...(current.notes || [])],
          activities: [data.activity, ...(current.activities || [])]
        }
        setDetailCache((cache) => ({ ...cache, [selectedContactId]: next }))
        return next
      })
      setNoteBody('')
      setError('')
    } catch (noteError) {
      setError(noteError.message || 'Unable to add note.')
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    if (!selectedContactId || !taskForm.title.trim()) {
      return
    }

    try {
      const data = await createTask({
        entityType: 'contact',
        entityId: selectedContactId,
        title: taskForm.title.trim(),
        description: taskForm.description.trim(),
        status: 'open',
        dueAt: taskForm.dueAt ? `${taskForm.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(taskForm.assignedToUserId, 10) || 0
      })
      setDetail((current) => {
        if (!current) {
          return current
        }
        const next = {
          ...current,
          tasks: [data.task, ...(current.tasks || []).filter((task) => task.id !== data.task.id)],
          activities: [...(data.activities || []), ...(current.activities || [])]
        }
        setDetailCache((cache) => ({ ...cache, [selectedContactId]: next }))
        return next
      })
      setTaskForm(emptyTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  const detailTitle = useMemo(() => fullName(selectedContact || {}), [selectedContact])

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Contacts</h2>
              <p>Keep the right people moving without a bloated CRM mess.</p>
            </div>
            <Button
              onClick={() => {
                navigate('/contacts')
                setMode('create')
                setForm(emptyForm)
                setDetail(null)
                setSelectedContactId(null)
              }}
            >
              Add contact
            </Button>
          </div>
          <Field label="Search contacts">
            <input className="text-input" value={search} onChange={handleSearchChange} />
          </Field>
          {error ? (
            <div className="card-stack">
              <p className="form-error">{error}</p>
              {duplicateCandidate ? (
                <div>
                  <Button className="button-secondary" onClick={handleOpenDuplicate}>
                    Open matching contact
                  </Button>
                </div>
              ) : null}
              {duplicateSearch ? (
                <div>
                  <Button className="button-secondary" onClick={handleDuplicateSearch}>
                    Search existing contacts for {duplicateSearch}
                  </Button>
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="record-list" role="list" aria-label="Contacts list">
            {contacts.map((contact) => (
              <article className="record-row" key={contact.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenContact(contact)}>
                    {fullName(contact)}
                  </button>
                  <p>{contact.jobTitle || 'No title'}</p>
                </div>
                <div>
                  <p>{contact.email || formatAddress(contact) || 'No contact details'}</p>
                  <p>{contact.status}</p>
                </div>
              </article>
            ))}
          </div>
          <p className="field-hint">Showing {contacts.length} of {meta.total} contacts.</p>
        </div>
      </Card>

      {mode === 'create' ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>New contact</h2>
              <p>Add the next person you need to move through the pipeline.</p>
            </div>
            <form className="auth-form" onSubmit={handleCreate}>
              <Field label="First name">
                <input className="text-input" value={form.firstName} onChange={(event) => setForm((current) => ({ ...current, firstName: event.target.value }))} required />
              </Field>
              <Field label="Last name">
                <input className="text-input" value={form.lastName} onChange={(event) => setForm((current) => ({ ...current, lastName: event.target.value }))} required />
              </Field>
              <Field label="Email">
                <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Address line 1">
                <input className="text-input" value={form.addressLine1} onChange={(event) => setForm((current) => ({ ...current, addressLine1: event.target.value }))} />
              </Field>
              <Field label="Address line 2">
                <input className="text-input" value={form.addressLine2} onChange={(event) => setForm((current) => ({ ...current, addressLine2: event.target.value }))} />
              </Field>
              <Field label="City">
                <input className="text-input" value={form.city} onChange={(event) => setForm((current) => ({ ...current, city: event.target.value }))} />
              </Field>
              <Field label="State">
                <input className="text-input" value={form.state} onChange={(event) => setForm((current) => ({ ...current, state: event.target.value }))} />
              </Field>
              <Field label="Postal code">
                <input className="text-input" value={form.postalCode} onChange={(event) => setForm((current) => ({ ...current, postalCode: event.target.value }))} />
              </Field>
              <Field label="Country">
                <input className="text-input" value={form.country} onChange={(event) => setForm((current) => ({ ...current, country: event.target.value }))} />
              </Field>
              <Field label="Job title">
                <input className="text-input" value={form.jobTitle} onChange={(event) => setForm((current) => ({ ...current, jobTitle: event.target.value }))} />
              </Field>
              <Button type="submit">Save contact</Button>
            </form>
          </div>
        </Card>
      ) : null}

      {mode === 'detail' && selectedContact ? (
        <Card>
          <div className="card-stack">
            <div className="section-header">
              <div>
                <h2>{detailTitle}</h2>
                <p>{selectedContact.email || formatAddress(selectedContact) || selectedContact.phone}</p>
              </div>
              <Button className="button-danger" onClick={handleArchive}>
                Archive contact
              </Button>
            </div>
            <form className="auth-form" onSubmit={handleUpdate}>
              <Field label="First name">
                <input className="text-input" value={form.firstName} onChange={(event) => setForm((current) => ({ ...current, firstName: event.target.value }))} required />
              </Field>
              <Field label="Last name">
                <input className="text-input" value={form.lastName} onChange={(event) => setForm((current) => ({ ...current, lastName: event.target.value }))} required />
              </Field>
              <Field label="Email">
                <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Address line 1">
                <input className="text-input" value={form.addressLine1} onChange={(event) => setForm((current) => ({ ...current, addressLine1: event.target.value }))} />
              </Field>
              <Field label="Address line 2">
                <input className="text-input" value={form.addressLine2} onChange={(event) => setForm((current) => ({ ...current, addressLine2: event.target.value }))} />
              </Field>
              <Field label="City">
                <input className="text-input" value={form.city} onChange={(event) => setForm((current) => ({ ...current, city: event.target.value }))} />
              </Field>
              <Field label="State">
                <input className="text-input" value={form.state} onChange={(event) => setForm((current) => ({ ...current, state: event.target.value }))} />
              </Field>
              <Field label="Postal code">
                <input className="text-input" value={form.postalCode} onChange={(event) => setForm((current) => ({ ...current, postalCode: event.target.value }))} />
              </Field>
              <Field label="Country">
                <input className="text-input" value={form.country} onChange={(event) => setForm((current) => ({ ...current, country: event.target.value }))} />
              </Field>
              <Field label="Job title">
                <input className="text-input" value={form.jobTitle} onChange={(event) => setForm((current) => ({ ...current, jobTitle: event.target.value }))} />
              </Field>
              <Field label="Status">
                <select className="text-input" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="lead">Lead</option>
                  <option value="customer">Customer</option>
                  <option value="prospect">Prospect</option>
                </select>
              </Field>
              <Button type="submit">Update contact</Button>
            </form>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <div>
                    <h3>{`Related ${pipelineLabels.plural.toLowerCase()}`}</h3>
                    <p>{`See active ${pipelineLabels.plural.toLowerCase()} tied to this contact.`}</p>
                  </div>
                  <Button className="button-secondary" onClick={handleCreateRelatedDeal}>
                    {`Create ${pipelineLabels.singular}`}
                  </Button>
                </div>
                <div className="record-list" role="list" aria-label="Related deals list">
                  {selectedDeals.length === 0 ? (
                    <article className="record-row" role="listitem">
                      <div>
                        <p>{`No related ${pipelineLabels.plural.toLowerCase()} yet.`}</p>
                      </div>
                    </article>
                  ) : selectedDeals.map((deal) => (
                    <article className="record-row" key={deal.id} role="listitem">
                      <div>
                        <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenDeal(deal.id)}>
                          {deal.name}
                        </button>
                        <p>{deal.stageName || deal.status || 'Unstaged'}</p>
                      </div>
                      <div>
                        <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
                        <p>{deal.companyName || (deal.expectedCloseDate ? `Target ${deal.expectedCloseDate}` : 'No client linked')}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <h3>Notes</h3>
                <form className="auth-form" onSubmit={handleCreateNote}>
                  <Field label="New note">
                    <textarea className="text-input" value={noteBody} onChange={(event) => setNoteBody(event.target.value)} rows={4} />
                  </Field>
                  <Button type="submit">Add note</Button>
                </form>
                <div className="record-list" role="list" aria-label="Notes list">
                  {selectedNotes.map((note) => (
                    <article className="record-row" key={note.id} role="listitem">
                      <div>
                        <p>{note.body}</p>
                        <p className="field-hint">{note.createdByUserName || 'Unknown author'}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <h3>Tasks</h3>
                <form className="auth-form" onSubmit={handleCreateTask}>
                  <Field label="Task title">
                    <input className="text-input" value={taskForm.title} onChange={(event) => setTaskForm((current) => ({ ...current, title: event.target.value }))} required />
                  </Field>
                  <Field label="Task description">
                    <textarea className="text-input" value={taskForm.description} onChange={(event) => setTaskForm((current) => ({ ...current, description: event.target.value }))} rows={3} />
                  </Field>
                  <Field label="Assigned to">
                    <select className="text-input" value={taskForm.assignedToUserId} onChange={(event) => setTaskForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
                      {userOptions.map((user) => (
                        <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                      ))}
                    </select>
                  </Field>
                  <Field label="Due at">
                    <input className="text-input" type="datetime-local" value={taskForm.dueAt} onChange={(event) => setTaskForm((current) => ({ ...current, dueAt: event.target.value }))} />
                  </Field>
                  <Button type="submit">Save task</Button>
                </form>
                <div className="record-list" role="list" aria-label="Contact tasks list">
                  {selectedTasks.map((task) => (
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
                <div className="record-list" role="list" aria-label="Activity list">
                  {selectedActivities.length === 0 ? (
                    <article className="record-row" role="listitem">
                      <div>
                        <p>No activity yet.</p>
                      </div>
                    </article>
                  ) : selectedActivities.map((activity) => (
                    <article className="record-row" key={activity.id} role="listitem">
                      <div>
                        <p>{activity.summary}</p>
                        <p className="field-hint">{formatActivityTimestamp(activity.createdAt)}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
