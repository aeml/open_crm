import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listCustomFields } from '../lib/custom_fields'
import { listOrganizationUsers } from '../lib/users'
import { buildClientRecords, organizationClientFromCompany, sortContactOptions } from './company_view'

export function buildCompaniesPath(search = '', owner = 'all', customFilter = {}) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (owner !== 'all') params.set('owner', owner)
  if (customFilter.fieldKey) {
    params.set('customField', customFilter.fieldKey)
    params.set('customOperator', customFilter.operator)
    params.set('customValue', customFilter.value)
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return `/companies${suffix}`
}

export function useCompanyDirectory({ initialCustomFilter, initialOwnerFilter, initialSearch }) {
  const [companies, setCompanies] = useState([])
  const [bulkEntityType, setBulkEntityType] = useState('company')
  const [selectedClientIds, setSelectedClientIds] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [customFilter, setCustomFilter] = useState(initialCustomFilter)
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [ownerOptions, setOwnerOptions] = useState([])
  const [companyCustomDefinitions, setCompanyCustomDefinitions] = useState([])
  const [contactCustomDefinitions, setContactCustomDefinitions] = useState([])
  const [customDefinitionsLoaded, setCustomDefinitionsLoaded] = useState(false)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const listControllerRef = useRef(null)
  const listRequestRef = useRef(null)

  function clearStatus() {
    setError('')
    setDuplicateSearch('')
    setDuplicateCandidate(null)
  }

  async function loadCompanies(nextSearch = '', nextOwner = 'all', { request = {}, signal, nextCustomFilter = customFilter } = {}) {
    listRequestRef.current = request
    const isUnassigned = nextOwner === 'unassigned'
    const ownerUserId = isUnassigned || nextOwner === 'all' ? 0 : Number.parseInt(nextOwner, 10) || 0

    try {
      const [companyData, contactData] = await Promise.all([
        listCompanies({ search: nextSearch, unassigned: isUnassigned, ownerUserId, customField: nextCustomFilter }, { signal }),
        listContacts({ search: nextSearch, unassigned: isUnassigned, ownerUserId }, { signal })
      ])
      if (listRequestRef.current !== request || signal?.aborted) return false

      if (Array.isArray(companyData?.companies)) {
        const nextContacts = nextCustomFilter.fieldKey ? [] : (contactData?.contacts || []).filter((contact) => contact.isClient)
        const clients = buildClientRecords(companyData.companies, nextContacts)
        setCompanies(clients)
        setSelectedClientIds([])
        setMeta({ page: 1, pageSize: 20, total: clients.length })
      } else if (companyData?.company) {
        setCompanies([organizationClientFromCompany(companyData.company)])
        setSelectedClientIds([])
        setMeta({ page: 1, pageSize: 20, total: 1 })
      } else {
        setCompanies([])
        setSelectedClientIds([])
        setMeta({ page: 1, pageSize: 20, total: 0 })
      }
      return true
    } catch (loadError) {
      if (listRequestRef.current !== request || signal?.aborted || isAbortError(loadError)) return false
      throw loadError
    }
  }

  async function reloadCompanies(nextSearch = search, nextOwner = ownerFilter, nextCustomFilter = customFilter) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    const request = {}
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      const applied = await loadCompanies(nextSearch, nextOwner, { request, signal: controller.signal, nextCustomFilter })
      if (applied) clearStatus()
    } catch (loadError) {
      if (!isAbortError(loadError) && listRequestRef.current === request) {
        setError(loadError.message || 'Unable to load companies.')
      }
    } finally {
      if (listRequestRef.current === request) setIsListLoading(false)
      if (listControllerRef.current === controller) listControllerRef.current = null
    }
  }

  useEffect(() => {
    const bootstrapController = new AbortController()
    const listController = new AbortController()
    const request = {}
    listControllerRef.current = listController

    async function run() {
      setIsListLoading(true)
      try {
        const [listApplied, contacts, owners, companyDefinitions, contactDefinitions] = await Promise.all([
          loadCompanies(initialSearch, initialOwnerFilter, { request, signal: listController.signal, nextCustomFilter: initialCustomFilter }),
          listContacts('', { signal: bootstrapController.signal }),
          listOrganizationUsers({ includeDisabled: true, signal: bootstrapController.signal }),
          listCustomFields('company', { signal: bootstrapController.signal }),
          listCustomFields('contact', { signal: bootstrapController.signal })
        ])
        if (bootstrapController.signal.aborted) return
        setContactOptions(sortContactOptions(contacts.contacts || []))
        setOwnerOptions(owners)
        setUserOptions(owners.filter((user) => (user.status || 'active') === 'active'))
        setCompanyCustomDefinitions(companyDefinitions)
        setContactCustomDefinitions(contactDefinitions)
        setCustomDefinitionsLoaded(true)
        if (listApplied) clearStatus()
      } catch (loadError) {
        if (!bootstrapController.signal.aborted && !isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load companies.')
        }
      } finally {
        if (listRequestRef.current === request) setIsListLoading(false)
        if (listControllerRef.current === listController) listControllerRef.current = null
      }
    }

    run()
    return () => {
      bootstrapController.abort()
      listController.abort()
      listControllerRef.current?.abort()
      listRequestRef.current = null
    }
  }, [])

  return {
    bulkEntityType,
    companies,
    companyCustomDefinitions,
    contactCustomDefinitions,
    contactOptions,
    customDefinitionsLoaded,
    customFilter,
    duplicateCandidate,
    duplicateSearch,
    error,
    isListLoading,
    meta,
    ownerFilter,
    ownerOptions,
    reloadCompanies,
    search,
    selectedClientIds,
    setBulkEntityType,
    setCompanies,
    setContactOptions,
    setCustomFilter,
    setDuplicateCandidate,
    setDuplicateSearch,
    setError,
    setMeta,
    setOwnerFilter,
    setSearch,
    setSelectedClientIds,
    userOptions
  }
}
