import { useEffect, useRef, useState } from 'react'
import { Button } from './button'
import { Field } from './field'
import { createSavedView, deleteSavedView, listSavedViews, updateSavedView } from '../../lib/saved_views'

const MAX_STORED_VIEWS = 100

export function SavedViews({ entityType, currentFilters, onApply, defaultName = 'My view', viewScope = '', allowDefault = true, noun = 'view', canManage = true }) {
  const [views, setViews] = useState([])
  const [selectedViewId, setSelectedViewId] = useState('')
  const [name, setName] = useState(defaultName)
  const [message, setMessage] = useState('')
  const [isDefault, setIsDefault] = useState(false)
  const [catalogTotal, setCatalogTotal] = useState(0)
  const [catalogLoaded, setCatalogLoaded] = useState(false)
  const [pendingAction, setPendingAction] = useState('')
  const operationRef = useRef(null)
  const operationIDRef = useRef(0)
  const contextRef = useRef(0)
  const mountedRef = useRef(true)
  const pluralNoun = `${noun}s`
  const actionSuffix = noun === 'view' ? '' : ` ${noun}`
  const isBusy = pendingAction !== ''
  const isAtCapacity = catalogLoaded && catalogTotal >= MAX_STORED_VIEWS

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      if (operationRef.current?.kind === 'load') operationRef.current.controller.abort()
    }
  }, [])

  useEffect(() => {
    contextRef.current += 1
    if (operationRef.current?.kind === 'load') {
      operationRef.current.controller.abort()
      operationRef.current = null
      setPendingAction('')
    }
    setViews([])
    setSelectedViewId('')
    setName(defaultName)
    setMessage('')
    setIsDefault(false)
    setCatalogTotal(0)
    setCatalogLoaded(false)
  }, [defaultName, entityType, viewScope])

  function beginOperation(kind) {
    if (operationRef.current) {
      setMessage(`Wait for the current saved-${noun} operation to finish.`)
      return null
    }
    const operation = {
      id: ++operationIDRef.current,
      kind,
      context: contextRef.current,
      controller: new AbortController()
    }
    operationRef.current = operation
    setPendingAction(kind)
    return operation
  }

  function isCurrent(operation) {
    return mountedRef.current && operationRef.current === operation && operation.context === contextRef.current
  }

  function finishOperation(operation) {
    if (operationRef.current !== operation) return
    operationRef.current = null
    if (mountedRef.current) setPendingAction('')
  }

  async function refreshViews(operation, nextSelectedId = selectedViewId) {
    const allViews = await listSavedViews(entityType, { signal: operation.controller.signal })
    if (!isCurrent(operation)) return false
    const nextViews = allViews.filter((view) => (view.filters?.savedViewScope || '') === viewScope)
    setViews(nextViews)
    setCatalogTotal(allViews.length)
    setCatalogLoaded(true)
    setSelectedViewId(nextViews.some((view) => String(view.id) === String(nextSelectedId)) ? String(nextSelectedId) : '')
    return true
  }

  function filtersForSave() {
    return viewScope ? { ...currentFilters, savedViewScope: viewScope } : currentFilters
  }

  function selectedView() {
    return views.find((view) => String(view.id) === selectedViewId) || null
  }

  async function handleLoad() {
    const operation = beginOperation('load')
    if (!operation) return
    try {
      if (await refreshViews(operation, selectedViewId)) setMessage(`Saved ${pluralNoun} loaded.`)
    } catch (error) {
      if (isCurrent(operation)) setMessage(error.message || `Unable to load saved ${pluralNoun}.`)
    } finally {
      finishOperation(operation)
    }
  }

  async function runMutation(kind, mutate, successMessage, selectedID) {
    const operation = beginOperation(kind)
    if (!operation) return
    try {
      const result = await mutate()
      if (!isCurrent(operation)) return
      const success = successMessage(result)
      try {
        if (await refreshViews(operation, selectedID(result))) setMessage(success)
      } catch (reloadError) {
        if (isCurrent(operation)) setMessage(`${success} Reload failed: ${reloadError.message || `load saved ${pluralNoun}`}.`)
      }
    } catch (error) {
      if (isCurrent(operation)) setMessage(error.message || `Unable to ${kind} saved ${noun}.`)
    } finally {
      finishOperation(operation)
    }
  }

  async function handleSave() {
    const trimmedName = name.trim()
    if (!trimmedName) {
      setMessage(`Name the ${noun} before saving it.`)
      return
    }
    if (isAtCapacity) {
      setMessage(`Delete an unused saved ${noun} before creating another for this record type.`)
      return
    }
    await runMutation('save', async () => {
      const view = await createSavedView({ entityType, name: trimmedName, filters: filtersForSave(), isDefault: allowDefault && isDefault })
      return requireSavedView(view, entityType)
    }, (view) => `Saved ${view.name}.`, (view) => view.id)
  }

  async function handleUpdate() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to update.`)
      return
    }
    await runMutation('update', async () => {
      const updated = await updateSavedView(view.id, { entityType, name: view.name, filters: filtersForSave(), isDefault: allowDefault && view.isDefault, expectedRevision: view.revision })
      return requireSavedView(updated, entityType, view.id)
    }, (updated) => `Updated ${updated.name}.`, (updated) => updated.id)
  }

  async function handleMakeDefault() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to make default.`)
      return
    }
    await runMutation('make default', async () => {
      const updated = await updateSavedView(view.id, { entityType, name: view.name, filters: view.filters || {}, isDefault: true, expectedRevision: view.revision })
      return requireSavedView(updated, entityType, view.id)
    }, (updated) => `${updated.name} is now the default.`, (updated) => updated.id)
  }

  async function handleDelete() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to delete.`)
      return
    }
    await runMutation('delete', async () => {
      await deleteSavedView(view.id, view.revision)
      return view
    }, () => `Deleted ${view.name}.`, () => '')
  }

  function handleApply() {
    if (operationRef.current) {
      setMessage(`Wait for the current saved-${noun} operation to finish.`)
      return
    }
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to apply.`)
      return
    }
    onApply(view.filters || {})
    setMessage(`Applied ${view.name}.`)
  }

  return (
    <div className="saved-views-panel" role="region" aria-label={`Saved ${noun} management`} aria-busy={isBusy || undefined}>
      <Field label={`Saved ${pluralNoun}`}>
        <select className="text-input" value={selectedViewId} disabled={isBusy} onChange={(event) => setSelectedViewId(event.target.value)}>
          <option value="">{`Choose a saved ${noun}`}</option>
          {views.map((view) => (
            <option key={view.id} value={view.id}>{view.name}{view.isDefault ? ' (default)' : ''}</option>
          ))}
        </select>
      </Field>
      <div className="button-row">
        <Button className="button-secondary" type="button" disabled={isBusy} onClick={handleLoad}>{pendingAction === 'load' ? 'Loading…' : `Load ${pluralNoun}`}</Button>
        <Button className="button-secondary" type="button" disabled={isBusy} onClick={handleApply}>{`Apply${actionSuffix}`}</Button>
        {canManage ? <Button className="button-secondary" type="button" disabled={isBusy} onClick={handleUpdate}>{`Update${actionSuffix}`}</Button> : null}
        {canManage && allowDefault ? <Button className="button-secondary" type="button" disabled={isBusy} onClick={handleMakeDefault}>Make default</Button> : null}
        {canManage ? <Button className="button-danger" type="button" disabled={isBusy} onClick={handleDelete}>{`Delete${actionSuffix}`}</Button> : null}
      </div>
      {canManage ? <Field label={`Save current ${noun === 'view' ? 'filters' : noun} as`}>
        <input className="text-input" value={name} maxLength={100} disabled={isBusy} onChange={(event) => setName(event.target.value)} />
      </Field> : null}
      {canManage && allowDefault ? (
        <label className="checkbox-row">
          <input type="checkbox" checked={isDefault} disabled={isBusy} onChange={(event) => setIsDefault(event.target.checked)} />
          <span>Make this my default view</span>
        </label>
      ) : null}
      {canManage ? <div>
        <Button type="button" disabled={isBusy || isAtCapacity} onClick={handleSave}>{pendingAction === 'save' ? 'Saving…' : `Save ${noun}`}</Button>
      </div> : null}
      {catalogLoaded ? <p className="field-hint">{catalogTotal} of {MAX_STORED_VIEWS} saved views used for this record type.{isAtCapacity ? ` Delete one before creating another${catalogTotal > MAX_STORED_VIEWS ? '; legacy overflow remains available to manage' : ''}.` : ''}</p> : null}
      {message ? <p className="field-hint" role="status">{message}</p> : null}
    </div>
  )
}

function requireSavedView(view, entityType, expectedID) {
  const id = Number(view?.id)
  if (!Number.isSafeInteger(id) || id <= 0 || view?.entityType !== entityType || (expectedID !== undefined && String(view.id) !== String(expectedID))) {
    throw new Error('The server returned a different saved view. Reload before continuing.')
  }
  return view
}
