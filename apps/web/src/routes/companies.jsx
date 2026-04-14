import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { archiveCompany, createCompany, getCompany, listCompanies, updateCompany } from '../lib/companies'
import { createContact, listContacts } from '../lib/contacts'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'

const emptyForm = {
  name: '',
  clientType: 'organization',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  industry: '',
  email: '',
  phone: '',
  website: '',
  status: 'prospect',
  linkedContactIDs: ''
}

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}

function parseLinkedContactIDs(value) {
  return String(value || '')
    .split(',')
    .map((entry) => Number.parseInt(entry.trim(), 10))
    .filter((entry) => Number.isInteger(entry) && entry > 0)
}

function splitFullName(value) {
  const parts = String(value || '').trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return { firstName: '', lastName: '' }
  }
  if (parts.length === 1) {
    return { firstName: parts[0], lastName: parts[0] }
  }
  return {
    firstName: parts[0],
    lastName: parts.slice(1).join(' ')
  }
}

function formatAddress(value = {}) {
  const street = [value.addressLine1, value.addressLine2].filter(Boolean).join(', ')
  const locality = [value.city, value.state, value.postalCode].filter(Boolean).join(', ')
  return [street, locality, value.country].filter(Boolean).join(' | ')
}

function individualClientFromContact(contact) {
  return {
    id: `contact-${contact.id}`,
    entityId: contact.id,
    entityType: 'contact',
    clientType: 'individual',
    name: `${contact.firstName || ''} ${contact.lastName || ''}`.trim(),
    addressLine1: contact.addressLine1 || '',
    addressLine2: contact.addressLine2 || '',
    city: contact.city || '',
    state: contact.state || '',
    postalCode: contact.postalCode || '',
    country: contact.country || '',
    industry: contact.jobTitle || '',
    phone: contact.phone || '',
    website: '',
    status: contact.status || 'lead',
    email: contact.email || ''
  }
}

function organizationClientFromCompany(company) {
  return {
    ...company,
    entityId: company.id,
    entityType: 'company'
  }
}

function buildClientRecords(companies, contacts) {
  return [
    ...(companies || []).map(organizationClientFromCompany),
    ...(contacts || []).filter((contact) => contact.isClient).map(individualClientFromContact)
  ].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId)
}

function normalizeClientType(value) {
  return value === 'individual' ? 'individual' : 'organization'
}

function isIndividualClient(clientType) {
  return normalizeClientType(clientType) === 'individual'
}

function limitLinkedContacts(clientType, value) {
  const ids = parseLinkedContactIDs(value)
  if (isIndividualClient(clientType)) {
    return ids.slice(0, 1).join(',')
  }
  return ids.join(',')
}

function buildCompanyPayload(form) {
  const individual = isIndividualClient(form.clientType)
  return {
    name: form.name,
    clientType: normalizeClientType(form.clientType),
    addressLine1: form.addressLine1,
    addressLine2: form.addressLine2,
    city: form.city,
    state: form.state,
    postalCode: form.postalCode,
    country: form.country,
    industry: individual ? '' : form.industry,
    email: individual ? form.email : '',
    phone: form.phone,
    website: individual ? '' : form.website,
    status: form.status,
    linkedContactIDs: parseLinkedContactIDs(form.linkedContactIDs)
  }
}

function clientTypeLabel(clientType) {
  return isIndividualClient(clientType) ? 'Individual' : 'Organization'
}

function linkedContactFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Person record' : 'Linked contact'
}

function linkedContactFieldHint(clientType) {
  return isIndividualClient(clientType)
    ? 'Individual clients need one linked person record.'
    : 'Link the main person for this organization client.'
}

function createDescription(clientType) {
  return isIndividualClient(clientType)
    ? 'Add an individual client and link the matching person record.'
    : 'Add an organization client and tie the right contacts to it immediately.'
}

function nameFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Full name' : 'Client name'
}

function phoneFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Phone number' : 'Phone'
}

function detailSubtitle(company, linkedContacts = []) {
  if (!company) {
    return ''
  }
  if (isIndividualClient(company.clientType)) {
    return company.email || (linkedContacts?.[0]?.email) || company.phone || company.status || 'Individual client'
  }
  return company.website || formatAddress(company) || company.status || ''
}

function applyLinkedContactSelection(currentForm, contactOptions, value) {
  const nextLinkedContactIDs = limitLinkedContacts(currentForm.clientType, value)
  if (!isIndividualClient(currentForm.clientType)) {
    return { ...currentForm, linkedContactIDs: nextLinkedContactIDs }
  }

  const selectedID = parseLinkedContactIDs(nextLinkedContactIDs)[0] || 0
  const selectedContact = contactOptions.find((contact) => contact.id === selectedID)

  return {
    ...currentForm,
    linkedContactIDs: nextLinkedContactIDs,
    name: currentForm.name || `${selectedContact?.firstName || ''} ${selectedContact?.lastName || ''}`.trim(),
    email: currentForm.email || selectedContact?.email || '',
    phone: currentForm.phone || selectedContact?.phone || '',
    addressLine1: currentForm.addressLine1 || selectedContact?.addressLine1 || '',
    addressLine2: currentForm.addressLine2 || selectedContact?.addressLine2 || '',
    city: currentForm.city || selectedContact?.city || '',
    state: currentForm.state || selectedContact?.state || '',
    postalCode: currentForm.postalCode || selectedContact?.postalCode || '',
    country: currentForm.country || selectedContact?.country || ''
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

export function CompaniesRoute() {
  const navigate = useNavigate()
  const { companyId } = useParams()
  const routeCompanyId = Number.parseInt(companyId || '', 10)
  const [mode, setMode] = useState('list')
  const [companies, setCompanies] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState('')
  const [selectedCompanyId, setSelectedCompanyId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [error, setError] = useState('')

  const selectedCompany = detail?.company || null
  const linkedContacts = detail?.linkedContacts || []
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedActivities = detail?.activities || []

  async function loadCompanies(nextSearch = '') {
    const [companyData, contactData] = await Promise.all([listCompanies(nextSearch), listContacts(nextSearch)])

    if (Array.isArray(companyData?.companies)) {
      const nextCompanies = companyData.companies
      const nextContacts = (contactData?.contacts || []).filter((contact) => contact.isClient)
      const clients = buildClientRecords(nextCompanies, nextContacts)
      setCompanies(clients)
        setMeta({ page: 1, pageSize: 20, total: clients.length })
        return
      }

    if (companyData?.company) {
      const entry = organizationClientFromCompany(companyData.company)
      setCompanies([entry])
       setMeta({ page: 1, pageSize: 20, total: 1 })
       setDetailCache((current) => ({ ...current, [entry.id]: companyData }))
       return
     }

     setCompanies([])
     setMeta({ page: 1, pageSize: 20, total: 0 })
  }

  async function loadContactOptions() {
    const data = await listContacts()
    setContactOptions(data.contacts || [])
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

  function fillFormFromDetail(data) {
    setForm({
      name: data.company.name || '',
      clientType: normalizeClientType(data.company.clientType),
      addressLine1: data.company.addressLine1 || '',
      addressLine2: data.company.addressLine2 || '',
      city: data.company.city || '',
      state: data.company.state || '',
      postalCode: data.company.postalCode || '',
      country: data.company.country || '',
      industry: data.company.industry || '',
      email: data.company.email || '',
      phone: data.company.phone || '',
      website: data.company.website || '',
      status: data.company.status || 'prospect',
      linkedContactIDs: limitLinkedContacts(data.company.clientType, (data.linkedContacts || []).map((contact) => contact.id).join(','))
    })
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      try {
        await Promise.all([loadCompanies(''), loadContactOptions(), loadUserOptions()])
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
    if (company.entityType === 'contact') {
      navigate(`/contacts/${company.entityId}`)
      return
    }

    const companyID = company.id
    const cached = detailCache[companyID]
    if (cached) {
      setSelectedCompanyId(companyID)
      setDetail(cached)
      fillFormFromDetail(cached)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${companyID}`)
      return
    }

    try {
      const [data, notes, taskData] = await Promise.all([
        getCompany(companyID),
        listNotes('company', companyID),
        listTasks({ status: 'open', entityType: 'company', entityId: companyID })
      ])
      const detailData = { ...data, notes, tasks: taskData.tasks || [] }
      setDetailCache((current) => ({ ...current, [companyID]: detailData }))
      setSelectedCompanyId(companyID)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${companyID}`)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load company.')
    }
  }

  useEffect(() => {
    let cancelled = false

    async function openRouteCompany() {
      if (!Number.isInteger(routeCompanyId) || routeCompanyId <= 0) {
        if (selectedCompanyId || mode === 'detail') {
          setSelectedCompanyId(null)
          setDetail(null)
          setForm(emptyForm)
          setNoteBody('')
          setTaskForm(emptyTaskForm)
          setMode('list')
        }
        return
      }

      if (selectedCompanyId === routeCompanyId && detail?.company?.id === routeCompanyId) {
        return
      }

      const cached = detailCache[routeCompanyId]
      if (cached) {
        setSelectedCompanyId(routeCompanyId)
        setDetail(cached)
        fillFormFromDetail(cached)
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
        return
      }

      try {
        const [data, notes, taskData] = await Promise.all([
          getCompany(routeCompanyId),
          listNotes('company', routeCompanyId),
          listTasks({ status: 'open', entityType: 'company', entityId: routeCompanyId })
        ])
        if (cancelled) {
          return
        }
        const detailData = { ...data, notes, tasks: taskData.tasks || [] }
        setDetailCache((current) => ({ ...current, [routeCompanyId]: detailData }))
        setSelectedCompanyId(routeCompanyId)
        setDetail(detailData)
        fillFormFromDetail(detailData)
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load company.')
        }
      }
    }

    openRouteCompany()
    return () => {
      cancelled = true
    }
  }, [detail, detailCache, mode, routeCompanyId, selectedCompanyId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      if (isIndividualClient(form.clientType)) {
        const fullName = splitFullName(form.name)
        const data = await createContact({
          firstName: fullName.firstName,
          lastName: fullName.lastName,
          email: form.email,
          phone: form.phone,
          addressLine1: form.addressLine1,
          addressLine2: form.addressLine2,
          city: form.city,
          state: form.state,
          postalCode: form.postalCode,
          country: form.country,
          jobTitle: '',
          status: form.status,
          isClient: true
        })
        const nextClient = individualClientFromContact(data.contact)
        setCompanies((current) => [...current, nextClient].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
        setMeta((current) => ({ ...current, total: current.total + 1 }))
        setForm(emptyForm)
        setMode('list')
        navigate(`/contacts/${data.contact.id}`)
        setError('')
        return
      }

      const data = await createCompany({
        ...buildCompanyPayload(form)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to create company.')
      }
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [] }
      setDetailCache((current) => ({ ...current, [data.company.id]: detailData }))
      setCompanies((current) => [...current, organizationClientFromCompany(data.company)].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedCompanyId(data.company.id)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${data.company.id}`)
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
        ...buildCompanyPayload(form)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to update company.')
      }
      const detailData = { ...data, notes: detail?.notes || [], tasks: detail?.tasks || [] }
      setDetailCache((current) => ({ ...current, [selectedCompanyId]: detailData }))
      setCompanies((current) => current.map((entry) => (entry.id === selectedCompanyId ? organizationClientFromCompany(data.company) : entry)))
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
      setTaskForm(emptyTaskForm)
      setMode('list')
      navigate('/companies')
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

  async function handleCreateTask(event) {
    event.preventDefault()
    if (!selectedCompanyId || !taskForm.title.trim()) {
      return
    }

    try {
      const data = await createTask({
        entityType: 'company',
        entityId: selectedCompanyId,
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
        setDetailCache((cache) => ({ ...cache, [selectedCompanyId]: next }))
        return next
      })
      setTaskForm(emptyTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  const detailTitle = useMemo(() => selectedCompany?.name || '', [selectedCompany])

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
                <h2>Clients</h2>
              <p>See client ownership, linked people, and live pipeline in one place.</p>
            </div>
            <Button
              onClick={() => {
                navigate('/companies')
                setMode('create')
                setForm(emptyForm)
                setDetail(null)
                setSelectedCompanyId(null)
              }}
            >
              Add client
            </Button>
          </div>
          <Field label="Search clients">
            <input className="text-input" value={search} onChange={handleSearchChange} />
          </Field>
          {error ? <p className="form-error">{error}</p> : null}
          <div className="record-list" role="list" aria-label="Clients list">
            {companies.map((company) => (
              <article className="record-row" key={company.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenCompany(company)}>
                    {company.name}
                  </button>
                  <p>{company.industry || `${clientTypeLabel(company.clientType)} client`}</p>
                </div>
                <div>
                  <p>{company.email || company.website || formatAddress(company) || clientTypeLabel(company.clientType)}</p>
                  <p>{company.status}</p>
                </div>
              </article>
            ))}
          </div>
          <p className="field-hint">Showing {companies.length} of {meta.total} clients.</p>
        </div>
      </Card>

      {mode === 'create' ? (
        <Card>
            <div className="card-stack">
              <div>
                <h2>New client</h2>
                <p>{createDescription(form.clientType)}</p>
              </div>
            <form className="auth-form" onSubmit={handleCreate}>
              <Field label="Client type">
                <select
                  className="text-input"
                  value={form.clientType}
                  onChange={(event) => setForm((current) => ({
                    ...current,
                    clientType: event.target.value,
                    linkedContactIDs: limitLinkedContacts(event.target.value, current.linkedContactIDs)
                  }))}
                >
                  <option value="organization">Organization</option>
                  <option value="individual">Individual</option>
                </select>
              </Field>
              <Field label={nameFieldLabel(form.clientType)}>
                <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label={phoneFieldLabel(form.clientType)}>
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              {isIndividualClient(form.clientType) ? (
                <Field label="Email">
                  <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
                </Field>
              ) : null}
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
              <Field label={linkedContactFieldLabel(form.clientType)} hint={linkedContactFieldHint(form.clientType)}>
                <select className="text-input" value={form.linkedContactIDs} onChange={(event) => setForm((current) => applyLinkedContactSelection(current, contactOptions, event.target.value))}>
                  <option value="">{normalizeClientType(form.clientType) === 'individual' ? 'Select person record' : 'No linked contact'}</option>
                  {contactOptions.map((contact) => (
                    <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
                  ))}
                </select>
              </Field>
              {!isIndividualClient(form.clientType) ? (
                <>
                  <Field label="Industry">
                    <input className="text-input" value={form.industry} onChange={(event) => setForm((current) => ({ ...current, industry: event.target.value }))} />
                  </Field>
                  <Field label="Website" hint="Company site, like https://acme.com.">
                    <input className="text-input" value={form.website} onChange={(event) => setForm((current) => ({ ...current, website: event.target.value }))} />
                  </Field>
                </>
              ) : null}
              <Button type="submit">Save client</Button>
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
                <p>{detailSubtitle(selectedCompany, linkedContacts)}</p>
              </div>
              <Button className="button-danger" onClick={handleArchive}>
                Archive client
              </Button>
            </div>
            <form className="auth-form" onSubmit={handleUpdate}>
              <Field label="Client type">
                <select
                  className="text-input"
                  value={form.clientType}
                  onChange={(event) => setForm((current) => ({
                    ...current,
                    clientType: event.target.value,
                    linkedContactIDs: limitLinkedContacts(event.target.value, current.linkedContactIDs)
                  }))}
                >
                  <option value="organization">Organization</option>
                  <option value="individual">Individual</option>
                </select>
              </Field>
              <Field label={nameFieldLabel(form.clientType)}>
                <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label={phoneFieldLabel(form.clientType)}>
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              {isIndividualClient(form.clientType) ? (
                <Field label="Email">
                  <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
                </Field>
              ) : null}
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
              <Field label="Status">
                <select className="text-input" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="prospect">Prospect</option>
                  <option value="customer">Customer</option>
                  <option value="lead">Lead</option>
                </select>
              </Field>
              <Field label={linkedContactFieldLabel(form.clientType)} hint={linkedContactFieldHint(form.clientType)}>
                <select className="text-input" value={form.linkedContactIDs} onChange={(event) => setForm((current) => applyLinkedContactSelection(current, contactOptions, event.target.value))}>
                  <option value="">{normalizeClientType(form.clientType) === 'individual' ? 'Select person record' : 'No linked contact'}</option>
                  {contactOptions.map((contact) => (
                    <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
                  ))}
                </select>
              </Field>
              {!isIndividualClient(form.clientType) ? (
                <>
                  <Field label="Industry">
                    <input className="text-input" value={form.industry} onChange={(event) => setForm((current) => ({ ...current, industry: event.target.value }))} />
                  </Field>
                  <Field label="Website" hint="Company site, like https://acme.com.">
                    <input className="text-input" value={form.website} onChange={(event) => setForm((current) => ({ ...current, website: event.target.value }))} />
                  </Field>
                </>
              ) : null}
              <Button type="submit">Update client</Button>
            </form>
            <Card>
              <div className="card-stack">
                <h3>People</h3>
                <div className="record-list" role="list" aria-label="Linked contacts list">
                  {linkedContacts.length === 0 ? (
                    <article className="record-row" role="listitem">
                      <div>
                        <p>{normalizeClientType(selectedCompany.clientType) === 'individual' ? 'No linked person yet.' : 'No linked people yet.'}</p>
                      </div>
                    </article>
                  ) : linkedContacts.map((contact) => (
                    <article className="record-row" key={contact.id} role="listitem">
                      <div>
                        <button className="button button-ghost contact-link" type="button" onClick={() => navigate(`/contacts/${contact.id}`)}>
                          {contact.firstName} {contact.lastName}
                        </button>
                        <p>{contact.relationshipTitle || (normalizeClientType(selectedCompany.clientType) === 'individual' ? 'Client record' : 'Linked contact')}</p>
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
                <div className="record-list" role="list" aria-label="Client notes list">
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
                <div className="record-list" role="list" aria-label="Client tasks list">
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
