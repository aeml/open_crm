import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listContacts } from '../lib/contacts'
import { sortContactOptions } from './company_view'

const pageSize = 20

function mergeContacts(current, additions) {
  const byID = new Map((current || []).map((contact) => [contact.id, contact]))
  for (const contact of additions || []) {
    if (contact?.id) byID.set(contact.id, contact)
  }
  return sortContactOptions([...byID.values()])
}

export function useContactLookup() {
  const [contacts, setContacts] = useState([])
  const [query, setQuery] = useState('')
  const [appliedQuery, setAppliedQuery] = useState('')
  const [meta, setMeta] = useState({ page: 1, pageSize, total: 0 })
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const controllerRef = useRef(null)
  const requestRef = useRef(null)

  async function load(nextQuery, nextPage, append = false) {
    controllerRef.current?.abort()
    const controller = new AbortController()
    const request = {}
    controllerRef.current = controller
    requestRef.current = request
    setIsLoading(true)
    setError('')
    try {
      const data = await listContacts({ search: nextQuery.trim(), page: nextPage, pageSize }, { signal: controller.signal })
      if (requestRef.current !== request || controller.signal.aborted) return false
      setContacts((current) => append ? mergeContacts(current, data.contacts || []) : sortContactOptions(data.contacts || []))
      setMeta(data.meta || { page: nextPage, pageSize, total: (data.contacts || []).length })
      setAppliedQuery(nextQuery.trim())
      return true
    } catch (loadError) {
      if (!isAbortError(loadError) && requestRef.current === request) setError(loadError.message || 'Unable to find contacts.')
      return false
    } finally {
      if (requestRef.current === request) setIsLoading(false)
      if (controllerRef.current === controller) controllerRef.current = null
    }
  }

  useEffect(() => {
    load('', 1)
    return () => {
      controllerRef.current?.abort()
      requestRef.current = null
    }
  }, [])

  function includeContacts(additions) {
    if (appliedQuery) return
    const newContacts = (additions || []).filter((contact) => contact?.id && !contacts.some((entry) => entry.id === contact.id))
    if (newContacts.length === 0) return
    setContacts((current) => mergeContacts(current, newContacts))
    setMeta((current) => ({ ...current, total: current.total + newContacts.length }))
  }

  return {
    appliedQuery,
    contacts,
    error,
    includeContacts,
    isLoading,
    loadMore: () => load(appliedQuery, meta.page + 1, true),
    meta,
    query,
    reset: () => {
      setQuery('')
      return load('', 1)
    },
    search: () => load(query, 1),
    setQuery
  }
}
