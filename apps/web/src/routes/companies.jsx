import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveCompany, companiesExportURL, createCompany, getCompany, listCompanies, updateCompany } from '../lib/companies'
import { createContact, listContacts } from '../lib/contacts'
import { listDeals } from '../lib/deals'
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

const emptyLinkedPersonForm = {
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  jobTitle: '',
  status: 'lead'
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

function companyFormValues(company, linkedContacts = []) {
  return {
    name: company.name || '',
    clientType: normalizeClientType(company.clientType),
    addressLine1: company.addressLine1 || '',
    addressLine2: company.addressLine2 || '',
    city: company.city || '',
    state: company.state || '',
    postalCode: company.postalCode || '',
    country: company.country || '',
    industry: company.industry || '',
    email: company.email || '',
    phone: company.phone || '',
    website: company.website || '',
    status: company.status || 'prospect',
    linkedContactIDs: limitLinkedContacts(company.clientType, (linkedContacts || []).map((contact) => contact.id).join(','))
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

function linkedPersonFormValues(company) {
  const status = ['prospect', 'customer', 'lead'].includes(company?.status) ? company.status : 'lead'
  return {
    ...emptyLinkedPersonForm,
    status
  }
}

function sortContactOptions(contacts) {
  return [...contacts].sort((left, right) => {
    const leftName = `${left.firstName || ''} ${left.lastName || ''}`.trim()
    const rightName = `${right.firstName || ''} ${right.lastName || ''}`.trim()
    return leftName.localeCompare(rightName) || left.id - right.id
  })
}

function mergeLinkedContactIDs(linkedContacts, nextContactID) {
  const result = []
  const seen = new Set()

  for (const contact of linkedContacts || []) {
    const contactID = Number.parseInt(String(contact?.id || ''), 10)
    if (!Number.isInteger(contactID) || contactID <= 0 || seen.has(contactID)) {
      continue
    }
    seen.add(contactID)
    result.push(contactID)
  }

  if (Number.isInteger(nextContactID) && nextContactID > 0 && !seen.has(nextContactID)) {
    result.push(nextContactID)
  }

  return result
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

function primaryLinkedContactID(linkedContacts = []) {
  const primaryContact = linkedContacts.find((contact) => contact.isPrimary) || linkedContacts[0]
  return primaryContact?.id || 0
}

function duplicateSearchTerm(message, fallback = '') {
  const text = String(message || '')
  const marker = text.toLowerCase().lastIndexOf('duplicate company:')
  if (marker >= 0) {
    const candidate = text.slice(marker + 'duplicate company:'.length).split('(')[0].trim()
    if (candidate) {
      return candidate
    }
  }
  return String(fallback || '').trim()
}

export function CompaniesRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { companyId } = useParams()
  const { session, businessProfile } = useAuth()
  const routeCompanyId = Number.parseInt(companyId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const pipelineLabels = relatedPipelineLabels(businessType)
  const initialSearch = searchParams.get('q') || ''
  const [mode, setMode] = useState('list')
  const [companies, setCompanies] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [selectedCompanyId, setSelectedCompanyId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [linkedPersonForm, setLinkedPersonForm] = useState(emptyLinkedPersonForm)
  const [showLinkedPersonForm, setShowLinkedPersonForm] = useState(false)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const searchControllerRef = useRef(null)

  const selectedCompany = detail?.company || null
  const linkedContacts = detail?.linkedContacts || []
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const hasSearch = search.trim() !== ''
  const selectedActivities = detail?.activities || []

  function buildCompaniesPath(nextSearch = search) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    const suffix = params.toString() ? `?${params.toString()}` : ''
    return `/companies${suffix}`
  }

  async function loadCompanies(nextSearch = '', { signal } = {}) {
    const [companyData, contactData] = await Promise.all([listCompanies(nextSearch, { signal }), listContacts(nextSearch, { signal })])

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

  async function loadContactOptions({ signal } = {}) {
    const data = await listContacts('', { signal })
    setContactOptions(sortContactOptions(data.contacts || []))
  }

  async function loadUserOptions({ signal } = {}) {
    const nextUsers = await listOrganizationUsers({ signal })
    setUserOptions(nextUsers)
    setTaskForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  function fillFormFromDetail(data) {
    setForm(companyFormValues(data.company, data.linkedContacts || []))
    setLinkedPersonForm(linkedPersonFormValues(data.company))
    setShowLinkedPersonForm(false)
  }

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await Promise.all([loadCompanies(initialSearch, { signal: controller.signal }), loadContactOptions({ signal: controller.signal }), loadUserOptions({ signal: controller.signal })])
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load companies.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsListLoading(false)
        }
      }
    }

    run()
    return () => {
      controller.abort()
    }
  }, [])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    navigate(buildCompaniesPath(value), { replace: true })
    await reloadCompanies(value)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    setSearch(nextSearch)
    setMode('list')
    setDetail(null)
    setSelectedCompanyId(null)
    navigate(buildCompaniesPath(nextSearch), { replace: true })
    await reloadCompanies(nextSearch)
  }

  async function reloadCompanies(nextSearch = search) {
    searchControllerRef.current?.abort()
    const controller = new AbortController()
    searchControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadCompanies(nextSearch, { signal: controller.signal })
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load companies.')
      }
    } finally {
      if (searchControllerRef.current === controller) {
        searchControllerRef.current = null
      }
      if (!controller.signal.aborted) {
        setIsListLoading(false)
      }
    }
  }

  async function handleDuplicateSearch() {
    if (!duplicateSearch) {
      return
    }

    setSearch(duplicateSearch)
    setMode('list')
    setDetail(null)
    setSelectedCompanyId(null)
    navigate('/companies')
    try {
      await loadCompanies(duplicateSearch)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load companies.')
    }
  }

  function handleOpenDuplicate() {
    if (!duplicateCandidate?.id || !duplicateCandidate?.entityType) {
      return
    }
    if (duplicateCandidate.entityType === 'contact') {
      navigate(`/contacts/${duplicateCandidate.id}`)
      return
    }
    navigate(`/companies/${duplicateCandidate.id}`)
  }

  function handleOpenDeal(dealID) {
    navigate(`/deals/${dealID}`)
  }

  function handleCreateRelatedDeal() {
    if (!selectedCompanyId) {
      return
    }
    const params = new URLSearchParams({ companyId: String(selectedCompanyId) })
    const contactID = primaryLinkedContactID(linkedContacts)
    if (contactID > 0) {
      params.set('primaryContactId', String(contactID))
    }
    navigate(`/deals?${params.toString()}`)
  }

  function handleOpenCompanyTasks() {
    if (!selectedCompanyId) {
      return
    }
    navigate(`/tasks?entityType=company&entityId=${selectedCompanyId}`)
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
        const [data, notes, taskData, dealData] = await Promise.all([
          getCompany(companyID),
          listNotes('company', companyID),
          listTasks({ status: 'open', entityType: 'company', entityId: companyID }),
          listDeals({ companyId: companyID })
        ])
        const detailData = { ...data, notes, tasks: taskData.tasks || [], deals: dealData.deals || [] }
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
    const controller = new AbortController()

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
        setIsDetailLoading(true)
        const [data, notes, taskData, dealData] = await Promise.all([
          getCompany(routeCompanyId, { signal: controller.signal }),
          listNotes('company', routeCompanyId, { signal: controller.signal }),
          listTasks({ status: 'open', entityType: 'company', entityId: routeCompanyId }, { signal: controller.signal }),
          listDeals({ companyId: routeCompanyId }, { signal: controller.signal })
        ])
        if (controller.signal.aborted) {
          return
        }
        const detailData = { ...data, notes, tasks: taskData.tasks || [], deals: dealData.deals || [] }
        setDetailCache((current) => ({ ...current, [routeCompanyId]: detailData }))
        setSelectedCompanyId(routeCompanyId)
        setDetail(detailData)
        fillFormFromDetail(detailData)
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load company.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsDetailLoading(false)
        }
      }
    }

    openRouteCompany()
    return () => {
      controller.abort()
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
        setDuplicateSearch('')
        setDuplicateCandidate(null)
        return
      }

      const data = await createCompany({
        ...buildCompanyPayload(form)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to create company.')
      }
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [], deals: [] }
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
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to create company.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
      setDuplicateCandidate(saveError.duplicate || null)
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
      const detailData = { ...data, notes: detail?.notes || [], tasks: detail?.tasks || [], deals: detail?.deals || [] }
      setDetailCache((current) => ({ ...current, [selectedCompanyId]: detailData }))
      setCompanies((current) => current.map((entry) => (entry.id === selectedCompanyId ? organizationClientFromCompany(data.company) : entry)))
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to update company.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleCreateLinkedPerson(event) {
    event.preventDefault()
    if (!selectedCompanyId || !selectedCompany || isIndividualClient(selectedCompany.clientType)) {
      return
    }

    try {
      const contactData = await createContact({
        firstName: linkedPersonForm.firstName,
        lastName: linkedPersonForm.lastName,
        email: linkedPersonForm.email,
        phone: linkedPersonForm.phone,
        addressLine1: '',
        addressLine2: '',
        city: '',
        state: '',
        postalCode: '',
        country: '',
        jobTitle: linkedPersonForm.jobTitle,
        status: linkedPersonForm.status
      })
      if (!contactData?.contact?.id) {
        throw new Error('Unable to create contact.')
      }

      setContactOptions((current) => sortContactOptions([...current.filter((entry) => entry.id !== contactData.contact.id), contactData.contact]))

      const linkedContactIDs = mergeLinkedContactIDs(linkedContacts, contactData.contact.id)
      const companyData = await updateCompany(selectedCompanyId, buildCompanyPayload({
        ...form,
        linkedContactIDs: linkedContactIDs.join(',')
      }))
      if (!companyData?.company?.id) {
        throw new Error('Unable to update company.')
      }

      const detailData = { ...companyData, notes: detail?.notes || [], tasks: detail?.tasks || [], deals: detail?.deals || [] }
      setDetailCache((current) => ({ ...current, [selectedCompanyId]: detailData }))
      setCompanies((current) => current.map((entry) => (entry.id === selectedCompanyId ? organizationClientFromCompany(companyData.company) : entry)))
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to add linked person.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, linkedPersonForm.email || `${linkedPersonForm.firstName} ${linkedPersonForm.lastName}`))
      setDuplicateCandidate(saveError.duplicate || null)
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
      setDuplicateSearch('')
      setDuplicateCandidate(null)
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
            <div className="button-row">
              <Button className="button-secondary" type="button" onClick={() => { window.location.href = companiesExportURL(search) }}>
                Export CSV
              </Button>
              <Button
                onClick={() => {
                  navigate('/companies')
                  setMode('create')
                  setForm(emptyForm)
                  setLinkedPersonForm(emptyLinkedPersonForm)
                  setShowLinkedPersonForm(false)
                  setDetail(null)
                  setSelectedCompanyId(null)
                }}
              >
                Add client
              </Button>
            </div>
          </div>
          <Field label="Search clients">
            <input className="text-input" value={search} onChange={handleSearchChange} />
          </Field>
          <SavedViews entityType="companies" currentFilters={{ q: search }} onApply={handleApplySavedView} defaultName="Client view" />
          {isListLoading ? <p className="field-hint">Loading clients...</p> : null}
          {error ? (
            <div className="card-stack">
              <p className="form-error">{error}</p>
              <div>
                <Button className="button-secondary" type="button" onClick={() => reloadCompanies(search)}>
                  Retry clients
                </Button>
              </div>
              {duplicateCandidate ? (
                <div>
                  <Button className="button-secondary" onClick={handleOpenDuplicate}>
                    Open matching {duplicateCandidate.entityType === 'contact' ? 'contact' : 'client'}
                  </Button>
                </div>
              ) : null}
              {duplicateSearch ? (
                <div>
                  <Button className="button-secondary" onClick={handleDuplicateSearch}>
                    Search existing clients for {duplicateSearch}
                  </Button>
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="record-list" role="list" aria-label="Clients list">
            {!isListLoading && companies.length === 0 ? (
              <EmptyState
                title={hasSearch ? 'No clients match the current search.' : 'No clients yet.'}
                description={hasSearch ? 'Try a different client, website, or contact name.' : 'Create an organization or individual client so your contacts, deals, jobs, notes, and tasks have a home.'}
                actionLabel={hasSearch ? 'Clear search' : 'Create first client'}
                onAction={() => {
                  if (hasSearch) {
                    handleSearchChange({ target: { value: '' } })
                    return
                  }
                  navigate('/companies')
                  setMode('create')
                  setForm(emptyForm)
                  setLinkedPersonForm(emptyLinkedPersonForm)
                  setShowLinkedPersonForm(false)
                  setDetail(null)
                  setSelectedCompanyId(null)
                }}
              />
            ) : companies.map((company) => (
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
            {isDetailLoading ? <p className="field-hint">Loading client detail...</p> : null}
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
                <div className="section-header">
                  <div>
                    <h3>People</h3>
                    <p>{normalizeClientType(selectedCompany.clientType) === 'individual' ? 'Manage the linked person for this client.' : 'Add and manage the people tied to this client.'}</p>
                  </div>
                  {!isIndividualClient(selectedCompany.clientType) ? (
                    <Button className="button-secondary" onClick={() => {
                      setLinkedPersonForm(linkedPersonFormValues(selectedCompany))
                      setShowLinkedPersonForm((current) => !current)
                    }}>
                      {showLinkedPersonForm ? 'Cancel' : 'Add person'}
                    </Button>
                  ) : null}
                </div>
                {showLinkedPersonForm && !isIndividualClient(selectedCompany.clientType) ? (
                  <form className="auth-form" onSubmit={handleCreateLinkedPerson}>
                    <Field label="First name">
                      <input className="text-input" value={linkedPersonForm.firstName} onChange={(event) => setLinkedPersonForm((current) => ({ ...current, firstName: event.target.value }))} required />
                    </Field>
                    <Field label="Last name">
                      <input className="text-input" value={linkedPersonForm.lastName} onChange={(event) => setLinkedPersonForm((current) => ({ ...current, lastName: event.target.value }))} required />
                    </Field>
                    <Field label="Email">
                      <input className="text-input" type="email" value={linkedPersonForm.email} onChange={(event) => setLinkedPersonForm((current) => ({ ...current, email: event.target.value }))} />
                    </Field>
                    <Field label="Phone">
                      <input className="text-input" value={linkedPersonForm.phone} onChange={(event) => setLinkedPersonForm((current) => ({ ...current, phone: event.target.value }))} />
                    </Field>
                    <Field label="Job title">
                      <input className="text-input" value={linkedPersonForm.jobTitle} onChange={(event) => setLinkedPersonForm((current) => ({ ...current, jobTitle: event.target.value }))} />
                    </Field>
                    <Button type="submit">Save person</Button>
                  </form>
                ) : null}
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
                <div className="section-header">
                  <div>
                    <h3>{`Related ${pipelineLabels.plural.toLowerCase()}`}</h3>
                    <p>{`See active ${pipelineLabels.plural.toLowerCase()} tied to this client.`}</p>
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
                        <p>{deal.primaryContactName || (deal.expectedCloseDate ? `Target ${deal.expectedCloseDate}` : 'No primary contact')}</p>
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
                <div className="section-header">
                  <h3>Tasks</h3>
                  <Button className="button-secondary" type="button" onClick={handleOpenCompanyTasks}>
                    Open in tasks
                  </Button>
                </div>
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
                <ActivityTimeline activities={selectedActivities} ariaLabel="Activity list" />
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
