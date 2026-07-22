import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listCompanyLinkedContacts } from '../lib/companies'

const pageSize = 50

function mergeByID(current, additions) {
  const seen = new Set()
  return [...(current || []), ...(additions || [])].filter((contact) => {
    if (!contact?.id || seen.has(contact.id)) return false
    seen.add(contact.id)
    return true
  })
}

export function useCompanyLinkedContacts({ companyId, initialContacts, initialMeta }) {
  const seedContacts = initialContacts || []
  const [contacts, setContacts] = useState(seedContacts)
  const [meta, setMeta] = useState({ page: 1, pageSize, total: seedContacts.length, ...(initialMeta || {}) })
  const [unfilteredContacts, setUnfilteredContacts] = useState(seedContacts)
  const [unfilteredMeta, setUnfilteredMeta] = useState({ page: 1, pageSize, total: seedContacts.length, ...(initialMeta || {}) })
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const controllerRef = useRef(null)
  const requestRef = useRef(null)

  useEffect(() => {
    controllerRef.current?.abort()
    requestRef.current = null
    setContacts(initialContacts || [])
    setMeta({ page: 1, pageSize, total: (initialContacts || []).length, ...(initialMeta || {}) })
    setUnfilteredContacts(initialContacts || [])
    setUnfilteredMeta({ page: 1, pageSize, total: (initialContacts || []).length, ...(initialMeta || {}) })
    setQuery('')
    setAppliedQuery('')
    setIsLoading(false)
    setError('')
  }, [companyId, initialContacts, initialMeta])

  useEffect(() => () => controllerRef.current?.abort(), [])

  async function load(nextQuery, nextPage, append = false) {
    if (!companyId) return false
    controllerRef.current?.abort()
    const controller = new AbortController()
    const request = { companyId }
    controllerRef.current = controller
    requestRef.current = request
    setIsLoading(true)
    setError('')
    try {
      const data = await listCompanyLinkedContacts(companyId, { search: nextQuery.trim(), page: nextPage, pageSize }, { signal: controller.signal })
      if (requestRef.current !== request || controller.signal.aborted) return false
      const normalizedQuery = nextQuery.trim()
      const nextContacts = data.linkedContacts || []
      const nextMeta = data.meta || { page: nextPage, pageSize, total: nextContacts.length }
      setContacts((current) => append ? mergeByID(current, nextContacts) : nextContacts)
      setMeta(nextMeta)
      if (!normalizedQuery) {
        setUnfilteredContacts((current) => append ? mergeByID(current, nextContacts) : nextContacts)
        setUnfilteredMeta(nextMeta)
      }
      setAppliedQuery(normalizedQuery)
      return true
    } catch (loadError) {
      if (!isAbortError(loadError) && requestRef.current === request) setError(loadError.message || 'Unable to load linked people.')
      return false
    } finally {
      if (requestRef.current === request) setIsLoading(false)
      if (controllerRef.current === controller) controllerRef.current = null
    }
  }

  function include(contact) {
    if (!contact?.id || unfilteredContacts.some((entry) => entry.id === contact.id)) return
    setUnfilteredContacts((current) => mergeByID([contact], current))
    setUnfilteredMeta((current) => ({ ...current, total: current.total + 1 }))
    if (!appliedQuery) {
      setContacts((current) => mergeByID([contact], current))
      setMeta((current) => ({ ...current, total: current.total + 1 }))
    }
  }

  const knownContacts = mergeByID(unfilteredContacts, contacts)
  const primaryContact = unfilteredContacts.find((contact) => contact.isPrimary) || null

  return {
    appliedQuery,
    contacts,
    error,
    include,
    isLoading,
    knownContacts,
    loadMore: () => load(appliedQuery, meta.page + 1, true),
    meta,
    primaryContact,
    query,
    reset: () => {
      setQuery('')
      return load('', 1)
    },
    refresh: () => {
      setQuery('')
      return load('', 1)
    },
    search: () => load(query, 1),
    setQuery,
    unfilteredContacts,
    unfilteredMeta
  }
}
