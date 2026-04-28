import { useState } from 'react'
import { Button } from './button'
import { Field } from './field'
import { createSavedView, deleteSavedView, listSavedViews, updateSavedView } from '../../lib/saved_views'

export function SavedViews({ entityType, currentFilters, onApply, defaultName = 'My view' }) {
  const [views, setViews] = useState([])
  const [selectedViewId, setSelectedViewId] = useState('')
  const [name, setName] = useState(defaultName)
  const [message, setMessage] = useState('')
  const [isDefault, setIsDefault] = useState(false)

  async function refreshViews(nextSelectedId = selectedViewId) {
    const nextViews = await listSavedViews(entityType)
    setViews(nextViews)
    setSelectedViewId(nextSelectedId)
  }

  async function handleLoad() {
    try {
      await refreshViews(selectedViewId)
      setMessage('Saved views loaded.')
    } catch (error) {
      setMessage(error.message || 'Unable to load saved views.')
    }
  }

  function selectedView() {
    return views.find((view) => String(view.id) === selectedViewId) || null
  }

  async function handleSave() {
    const trimmedName = name.trim()
    if (!trimmedName) {
      setMessage('Name the view before saving it.')
      return
    }

    try {
      const view = await createSavedView({ entityType, name: trimmedName, filters: currentFilters, isDefault })
      await refreshViews(String(view.id))
      setMessage(`Saved ${view.name}.`)
    } catch (error) {
      setMessage(error.message || 'Unable to save view.')
    }
  }

  async function handleUpdate() {
    const view = selectedView()
    if (!view) {
      setMessage('Choose a saved view to update.')
      return
    }

    try {
      const updated = await updateSavedView(view.id, { entityType, name: view.name, filters: currentFilters, isDefault: view.isDefault })
      await refreshViews(String(updated.id))
      setMessage(`Updated ${updated.name}.`)
    } catch (error) {
      setMessage(error.message || 'Unable to update view.')
    }
  }

  async function handleMakeDefault() {
    const view = selectedView()
    if (!view) {
      setMessage('Choose a saved view to make default.')
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
      setMessage('Choose a saved view to delete.')
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
      setMessage('Choose a saved view to apply.')
      return
    }
    onApply(view.filters || {})
    setMessage(`Applied ${view.name}.`)
  }

  return (
    <div className="saved-views-panel">
      <Field label="Saved views">
        <select className="text-input" value={selectedViewId} onChange={(event) => setSelectedViewId(event.target.value)}>
          <option value="">Choose a saved view</option>
          {views.map((view) => (
            <option key={view.id} value={view.id}>{view.name}{view.isDefault ? ' (default)' : ''}</option>
          ))}
        </select>
      </Field>
      <div className="button-row">
        <Button className="button-secondary" type="button" onClick={handleLoad}>Load views</Button>
        <Button className="button-secondary" type="button" onClick={handleApply}>Apply</Button>
        <Button className="button-secondary" type="button" onClick={handleUpdate}>Update</Button>
        <Button className="button-secondary" type="button" onClick={handleMakeDefault}>Make default</Button>
        <Button className="button-danger" type="button" onClick={handleDelete}>Delete</Button>
      </div>
      <Field label="Save current filters as">
        <input className="text-input" value={name} onChange={(event) => setName(event.target.value)} />
      </Field>
      <label className="checkbox-row">
        <input type="checkbox" checked={isDefault} onChange={(event) => setIsDefault(event.target.checked)} />
        <span>Make this my default view</span>
      </label>
      <div>
        <Button type="button" onClick={handleSave}>Save view</Button>
      </div>
      {message ? <p className="field-hint" role="status">{message}</p> : null}
    </div>
  )
}
