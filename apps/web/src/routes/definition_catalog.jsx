import { useEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { isAbortError } from '../lib/api'

export const definitionCatalogPageSize = 50

export function useDefinitionCatalog({ requestPage, itemsKey = 'items', loadErrorMessage, onLoaded, reloadKey }) {
  const [items, setItems] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: definitionCatalogPageSize, total: 0 })
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [pageNumber, setPageNumber] = useState(1)
  const operationPending = useRef(false)
  const latestLoad = useRef(0)

  async function load({ signal, requestedPage = pageNumber, search = appliedSearch, itemStatus = statusFilter } = {}) {
    const loadID = latestLoad.current + 1
    latestLoad.current = loadID
    setIsLoading(true)
    try {
      const page = await requestPage({ search, status: itemStatus, page: requestedPage, pageSize: definitionCatalogPageSize, signal })
      if (signal?.aborted || loadID !== latestLoad.current) return null
      setItems(page[itemsKey])
      setMeta(page.meta)
      onLoaded?.(page)
      setError('')
      return page
    } catch (loadError) {
      if (!isAbortError(loadError) && loadID === latestLoad.current) {
        setError(loadError.message || loadErrorMessage)
      }
      return null
    } finally {
      if (!signal?.aborted && loadID === latestLoad.current) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [appliedSearch, pageNumber, reloadKey, statusFilter])

  function handleSearch(event) {
    event.preventDefault()
    const nextSearch = searchInput.trim()
    if (nextSearch === appliedSearch && pageNumber === 1) load({ requestedPage: 1, search: nextSearch })
    else {
      setPageNumber(1)
      setAppliedSearch(nextSearch)
    }
  }

  return {
    appliedSearch,
    error,
    handleSearch,
    isLoading,
    items,
    load,
    meta,
    operationPending,
    pageNumber,
    searchInput,
    setAppliedSearch,
    setError,
    setPageNumber,
    setSearchInput,
    setStatusFilter,
    statusFilter
  }
}

export function DefinitionCatalogFilters({
  applyLabel = 'Apply search', disabled, handleSearch, isLoading, searchInput, searchLabel, searchPlaceholder,
  setPageNumber, setSearchInput, setStatusFilter, statusFilter, statusLabel, children
}) {
  return (
    <form className="filters-grid" onSubmit={handleSearch}>
      <Field label={searchLabel}>
        <input className="text-input" maxLength={100} value={searchInput} disabled={disabled} onChange={(event) => setSearchInput(event.target.value)} placeholder={searchPlaceholder} />
      </Field>
      {statusLabel ? <Field label={statusLabel}>
        <select className="text-input" value={statusFilter} disabled={disabled} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
          {children}
        </select>
      </Field> : null}
      <Button className="button-secondary" type="submit" disabled={isLoading || disabled}>{applyLabel}</Button>
    </form>
  )
}

export function DefinitionCatalogPagination({
  appliedSearch, className = 'button-row', disabled, isLoading, itemCount,
  limitHint, meta, nextLabel = 'Next page', noun, pageNumber,
  previousLabel = 'Previous page', setPageNumber
}) {
  return (
    <>
      <p className="field-hint" role="status">Showing {itemCount} of {meta.total} {noun}{appliedSearch ? ` matching “${appliedSearch}”` : ''}. {limitHint}</p>
      <div className={className}>
        <Button className="button-secondary" type="button" disabled={isLoading || pageNumber <= 1 || disabled} onClick={() => setPageNumber((current) => current - 1)}>{previousLabel}</Button>
        <Button className="button-secondary" type="button" disabled={isLoading || pageNumber * meta.pageSize >= meta.total || disabled} onClick={() => setPageNumber((current) => current + 1)}>{nextLabel}</Button>
      </div>
    </>
  )
}
