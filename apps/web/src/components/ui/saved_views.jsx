import { useState } from 'react'
import { Button } from './button'
import { Field } from './field'
import { createSavedView, deleteSavedView, listSavedViews, updateSavedView } from '../../lib/saved_views'

export function SavedViews({ entityType, currentFilters, onApply, defaultName = 'My view', viewScope = '', allowDefault = true, noun = 'view', canManage = true }) {
  const [views, setViews] = useState([])
  const [selectedViewId, setSelectedViewId] = useState('')
  const [name, setName] = useState(defaultName)
  const [message, setMessage] = useState('')
  const [isDefault, setIsDefault] = useState(false)
  const pluralNoun = `${noun}s`
  const actionSuffix = noun === 'view' ? '' : ` ${noun}`

  async function refreshViews(nextSelectedId = selectedViewId) {
    const nextViews = await listSavedViews(entityType)
    setViews(nextViews.filter((view) => (view.filters?.savedViewScope || '') === viewScope))
    setSelectedViewId(nextSelectedId)
  }

  function filtersForSave() {
    return viewScope ? { ...currentFilters, savedViewScope: viewScope } : currentFilters
  }

  async function handleLoad() {
    try {
      await refreshViews(selectedViewId)
      setMessage(`Saved ${pluralNoun} loaded.`)
    } catch (error) {
      setMessage(error.message || `Unable to load saved ${pluralNoun}.`)
    }
  }

  function selectedView() {
    return views.find((view) => String(view.id) === selectedViewId) || null
  }

  async function handleSave() {
    const trimmedName = name.trim()
    if (!trimmedName) {
      setMessage(`Name the ${noun} before saving it.`)
      return
    }

    try {
      const view = await createSavedView({ entityType, name: trimmedName, filters: filtersForSave(), isDefault: allowDefault && isDefault })
      await refreshViews(String(view.id))
      setMessage(`Saved ${view.name}.`)
    } catch (error) {
      setMessage(error.message || 'Unable to save view.')
    }
  }

  async function handleUpdate() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to update.`)
      return
    }

    try {
      const updated = await updateSavedView(view.id, { entityType, name: view.name, filters: filtersForSave(), isDefault: allowDefault && view.isDefault })
      await refreshViews(String(updated.id))
      setMessage(`Updated ${updated.name}.`)
    } catch (error) {
      setMessage(error.message || 'Unable to update view.')
    }
  }

  async function handleMakeDefault() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to make default.`)
      return
    }

    try {
      const updated = await updateSavedView(view.id, { entityType, name: view.name, filters: view.filters || {}, isDefault: true })
      await refreshViews(String(updated.id))
      setMessage(`${updated.name} is now the default.`)
    } catch (error) {
      setMessage(error.message || 'Unable to update default view.')
    }
  }

  async function handleDelete() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to delete.`)
      return
    }

    try {
      await deleteSavedView(view.id)
      await refreshViews('')
      setMessage(`Deleted ${view.name}.`)
    } catch (error) {
      setMessage(error.message || 'Unable to delete view.')
    }
  }

  function handleApply() {
    const view = selectedView()
    if (!view) {
      setMessage(`Choose a saved ${noun} to apply.`)
      return
    }
    onApply(view.filters || {})
    setMessage(`Applied ${view.name}.`)
  }

  return (
    <div className="saved-views-panel">
      <Field label={`Saved ${pluralNoun}`}>
        <select className="text-input" value={selectedViewId} onChange={(event) => setSelectedViewId(event.target.value)}>
          <option value="">{`Choose a saved ${noun}`}</option>
          {views.map((view) => (
            <option key={view.id} value={view.id}>{view.name}{view.isDefault ? ' (default)' : ''}</option>
          ))}
        </select>
      </Field>
      <div className="button-row">
        <Button className="button-secondary" type="button" onClick={handleLoad}>{`Load ${pluralNoun}`}</Button>
        <Button className="button-secondary" type="button" onClick={handleApply}>{`Apply${actionSuffix}`}</Button>
        {canManage ? <Button className="button-secondary" type="button" onClick={handleUpdate}>{`Update${actionSuffix}`}</Button> : null}
        {canManage && allowDefault ? <Button className="button-secondary" type="button" onClick={handleMakeDefault}>Make default</Button> : null}
        {canManage ? <Button className="button-danger" type="button" onClick={handleDelete}>{`Delete${actionSuffix}`}</Button> : null}
      </div>
      {canManage ? <Field label={`Save current ${noun === 'view' ? 'filters' : noun} as`}>
        <input className="text-input" value={name} onChange={(event) => setName(event.target.value)} />
      </Field> : null}
      {canManage && allowDefault ? (
        <label className="checkbox-row">
          <input type="checkbox" checked={isDefault} onChange={(event) => setIsDefault(event.target.checked)} />
          <span>Make this my default view</span>
        </label>
      ) : null}
      {canManage ? <div>
        <Button type="button" onClick={handleSave}>{`Save ${noun}`}</Button>
      </div> : null}
      {message ? <p className="field-hint" role="status">{message}</p> : null}
    </div>
  )
}
