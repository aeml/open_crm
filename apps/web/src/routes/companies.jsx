import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { archiveCompany, createCompany, getCompany, listCompanies, updateCompany } from '../lib/companies'
import { createNote, listNotes } from '../lib/notes'
import { listTasks } from '../lib/tasks'

const emptyForm = {
  name: '',
  domain: '',
  industry: '',
  phone: '',
  website: '',
  status: 'prospect',
  linkedContactIDs: ''
}

function parseLinkedContactIDs(value) {
  return value
    .split(',')
    .map((entry) => Number.parseInt(entry.trim(), 10))
    .filter((entry) => Number.isInteger(entry) && entry > 0)
}

export function CompaniesRoute() {
  const [mode, setMode] = useState('list')
  const [companies, setCompanies] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState('')
  const [selectedCompanyId, setSelectedCompanyId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [form, setForm] = useState(emptyForm)
  const [noteBody, setNoteBody] = useState('')
  const [error, setError] = useState('')

  const selectedCompany = detail?.company || null
  const linkedContacts = detail?.linkedContacts || []
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedActivities = detail?.activities || []

  async function loadCompanies(nextSearch = '') {
    const data = await listCompanies(nextSearch)

    if (Array.isArray(data?.companies)) {
      setCompanies(data.companies)
      setMeta(data.meta || { page: 1, pageSize: 20, total: data.companies.length })
      return
    }

    if (data?.company) {
      const entry = data.company
      setCompanies([entry])
      setMeta({ page: 1, pageSize: 20, total: 1 })
      setDetailCache((current) => ({ ...current, [entry.id]: data }))
      return
    }

    setCompanies([])
    setMeta({ page: 1, pageSize: 20, total: 0 })
  }

  function fillFormFromDetail(data) {
    setForm({
      name: data.company.name || '',
      domain: data.company.domain || '',
      industry: data.company.industry || '',
      phone: data.company.phone || '',
      website: data.company.website || '',
      status: data.company.status || 'prospect',
      linkedContactIDs: (data.linkedContacts || []).map((contact) => contact.id).join(',')
    })
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      try {
        await loadCompanies('')
        if (!cancelled) {
          setError('')
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load companies.')
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
      await loadCompanies(value)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load companies.')
    }
  }

  async function handleOpenCompany(company) {
    const cached = detailCache[company.id]
    if (cached) {
      setSelectedCompanyId(company.id)
      setDetail(cached)
      fillFormFromDetail(cached)
      setNoteBody('')
      setMode('detail')
      return
    }

    try {
      const [data, notes, taskData] = await Promise.all([
        getCompany(company.id),
        listNotes('company', company.id),
        listTasks({ status: 'open', entityType: 'company', entityId: company.id })
      ])
      const detailData = { ...data, notes, tasks: taskData.tasks || [] }
      setDetailCache((current) => ({ ...current, [company.id]: detailData }))
      setSelectedCompanyId(company.id)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setMode('detail')
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load company.')
    }
  }

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createCompany({
        name: form.name,
        domain: form.domain,
        industry: form.industry,
        phone: form.phone,
        website: form.website,
        status: form.status,
        linkedContactIDs: parseLinkedContactIDs(form.linkedContactIDs)
      })
      const detailData = { ...data, notes: data.notes || [] }
      setDetailCache((current) => ({ ...current, [data.company.id]: detailData }))
      setCompanies((current) => [...current, data.company])
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedCompanyId(data.company.id)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setMode('detail')
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to create company.')
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedCompanyId) {
      return
    }

    try {
      const data = await updateCompany(selectedCompanyId, {
        name: form.name,
        domain: form.domain,
        industry: form.industry,
        phone: form.phone,
        website: form.website,
        status: form.status,
        linkedContactIDs: parseLinkedContactIDs(form.linkedContactIDs)
      })
      const detailData = { ...data, notes: detail?.notes || [], tasks: detail?.tasks || [] }
      setDetailCache((current) => ({ ...current, [selectedCompanyId]: detailData }))
      setCompanies((current) => current.map((entry) => (entry.id === selectedCompanyId ? data.company : entry)))
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update company.')
    }
  }

  async function handleArchive() {
    if (!selectedCompanyId) {
      return
    }

    try {
      await archiveCompany(selectedCompanyId)
      setCompanies((current) => current.filter((entry) => entry.id !== selectedCompanyId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      setDetail((current) => {
        if (!current?.company?.id) {
          return null
        }
        const next = { ...detailCache }
        delete next[current.company.id]
        setDetailCache(next)
        return null
      })
      setSelectedCompanyId(null)
      setForm(emptyForm)
      setNoteBody('')
      setMode('list')
      setError('')
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive company.')
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    if (!selectedCompanyId || !noteBody.trim()) {
      return
    }

    try {
      const data = await createNote({
        entityType: 'company',
        entityId: selectedCompanyId,
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
        setDetailCache((cache) => ({ ...cache, [selectedCompanyId]: next }))
        return next
      })
      setNoteBody('')
      setError('')
    } catch (noteError) {
      setError(noteError.message || 'Unable to add note.')
    }
  }

  const detailTitle = useMemo(() => selectedCompany?.name || '', [selectedCompany])

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Companies</h2>
              <p>See account ownership and live pipeline in one place.</p>
            </div>
            <Button
              onClick={() => {
                setMode('create')
                setForm(emptyForm)
                setDetail(null)
                setSelectedCompanyId(null)
              }}
            >
              Add company
            </Button>
          </div>
          <Field label="Search companies">
            <input className="text-input" value={search} onChange={handleSearchChange} />
          </Field>
          {error ? <p className="form-error">{error}</p> : null}
          <div className="record-list" role="list" aria-label="Companies list">
            {companies.map((company) => (
              <article className="record-row" key={company.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenCompany(company)}>
                    {company.name}
                  </button>
                  <p>{company.industry || 'No industry'}</p>
                </div>
                <div>
                  <p>{company.domain}</p>
                  <p>{company.status}</p>
                </div>
              </article>
            ))}
          </div>
          <p className="field-hint">Showing {companies.length} of {meta.total} companies.</p>
        </div>
      </Card>

      {mode === 'create' ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>New company</h2>
              <p>Add an account and tie the right contacts to it immediately.</p>
            </div>
            <form className="auth-form" onSubmit={handleCreate}>
              <Field label="Company name">
                <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label="Domain">
                <input className="text-input" value={form.domain} onChange={(event) => setForm((current) => ({ ...current, domain: event.target.value }))} />
              </Field>
              <Field label="Industry">
                <input className="text-input" value={form.industry} onChange={(event) => setForm((current) => ({ ...current, industry: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Website">
                <input className="text-input" value={form.website} onChange={(event) => setForm((current) => ({ ...current, website: event.target.value }))} />
              </Field>
              <Field label="Linked contact IDs">
                <input className="text-input" value={form.linkedContactIDs} onChange={(event) => setForm((current) => ({ ...current, linkedContactIDs: event.target.value }))} />
              </Field>
              <Button type="submit">Save company</Button>
            </form>
          </div>
        </Card>
      ) : null}

      {mode === 'detail' && selectedCompany ? (
        <Card>
          <div className="card-stack">
            <div className="section-header">
              <div>
                <h2>{detailTitle}</h2>
                <p>{selectedCompany.domain}</p>
              </div>
              <Button className="button-danger" onClick={handleArchive}>
                Archive company
              </Button>
            </div>
            <form className="auth-form" onSubmit={handleUpdate}>
              <Field label="Company name">
                <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label="Domain">
                <input className="text-input" value={form.domain} onChange={(event) => setForm((current) => ({ ...current, domain: event.target.value }))} />
              </Field>
              <Field label="Industry">
                <input className="text-input" value={form.industry} onChange={(event) => setForm((current) => ({ ...current, industry: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Website">
                <input className="text-input" value={form.website} onChange={(event) => setForm((current) => ({ ...current, website: event.target.value }))} />
              </Field>
              <Field label="Status">
                <select className="text-input" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="prospect">Prospect</option>
                  <option value="customer">Customer</option>
                  <option value="lead">Lead</option>
                </select>
              </Field>
              <Field label="Linked contact IDs">
                <input className="text-input" value={form.linkedContactIDs} onChange={(event) => setForm((current) => ({ ...current, linkedContactIDs: event.target.value }))} />
              </Field>
              <Button type="submit">Update company</Button>
            </form>
            <Card>
              <div className="card-stack">
                <h3>Linked contacts</h3>
                <div className="record-list" role="list" aria-label="Linked contacts list">
                  {linkedContacts.map((contact) => (
                    <article className="record-row" key={contact.id} role="listitem">
                      <div>
                        <p>{contact.firstName} {contact.lastName}</p>
                        <p>{contact.relationshipTitle || 'Linked contact'}</p>
                      </div>
                      <div>
                        <p>{contact.email}</p>
                        <p>{contact.isPrimary ? 'Primary' : 'Linked'}</p>
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
                <div className="record-list" role="list" aria-label="Company notes list">
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
                <div className="record-list" role="list" aria-label="Company tasks list">
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
                  {selectedActivities.map((activity) => (
                    <article className="record-row" key={activity.id} role="listitem">
                      <div>
                        <p>{activity.summary}</p>
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
