import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getSharedReportDashboard, updateSharedReportDashboard } from '../lib/reports'

function editableDashboardDefinition(definition) {
  return definition?.isActive === true && definition.visualizationType === 'bar' && definition.visualizationContract === 'grouped_bar_v1'
}

function draftFromDashboard(dashboard) {
  return dashboard.widgets.map((widget) => ({ reportDefinitionId: widget.reportDefinitionId, width: widget.width }))
}

function draftMatchesDashboard(draft, dashboard) {
  return draft.length === dashboard.widgets.length && draft.every((widget, index) => (
    widget.reportDefinitionId === dashboard.widgets[index].reportDefinitionId && widget.width === dashboard.widgets[index].width
  ))
}

export function ReportDashboardSettings({ definitions = [], totalDefinitions = 0, canManage = false, isLoadingMore = false, onLoadMore }) {
  const [dashboard, setDashboard] = useState(null)
  const [draft, setDraft] = useState([])
  const [addDefinitionId, setAddDefinitionId] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const requestRef = useRef(null)
  const requestVersion = useRef(0)
  const mutationPending = useRef(false)

  async function loadDashboard() {
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    const version = requestVersion.current + 1
    requestVersion.current = version
    setIsLoading(true)
    setError('')
    try {
      const loaded = await getSharedReportDashboard({ signal: controller.signal })
      if (controller.signal.aborted || requestVersion.current !== version) return
      setDashboard(loaded)
      setDraft(draftFromDashboard(loaded))
      setStatus('')
    } catch (loadError) {
      if (!isAbortError(loadError) && requestVersion.current === version) {
        setError(loadError.message || 'Unable to load the shared report dashboard.')
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null
      if (!controller.signal.aborted && requestVersion.current === version) setIsLoading(false)
    }
  }

  useEffect(() => {
    loadDashboard()
    return () => requestRef.current?.abort()
  }, [])

  const definitionMap = useMemo(() => {
    const mapped = new Map()
    dashboard?.widgets.forEach((widget) => mapped.set(widget.definition.id, widget.definition))
    definitions.forEach((definition) => mapped.set(definition.id, definition))
    return mapped
  }, [dashboard, definitions])
  const selectedIds = new Set(draft.map((widget) => widget.reportDefinitionId))
  const availableDefinitions = definitions.filter((definition) => editableDashboardDefinition(definition) && !selectedIds.has(definition.id))
  const hasMoreDefinitions = definitions.length < totalDefinitions
  const hasChanges = dashboard ? !draftMatchesDashboard(draft, dashboard) : false

  function addReport() {
    const definitionId = Number(addDefinitionId)
    if (!canManage || draft.length >= 6 || !availableDefinitions.some((definition) => definition.id === definitionId)) return
    setDraft((current) => [...current, { reportDefinitionId: definitionId, width: 'half' }])
    setAddDefinitionId('')
    setError('')
    setStatus('')
  }

  function removeReport(definitionId) {
    setDraft((current) => current.filter((widget) => widget.reportDefinitionId !== definitionId))
    setStatus('')
  }

  function updateWidth(definitionId, width) {
    setDraft((current) => current.map((widget) => (widget.reportDefinitionId === definitionId ? { ...widget, width } : widget)))
    setStatus('')
  }

  function moveReport(index, offset) {
    const nextIndex = index + offset
    if (nextIndex < 0 || nextIndex >= draft.length) return
    setDraft((current) => {
      const next = [...current]
      const moving = next[index]
      next[index] = next[nextIndex]
      next[nextIndex] = moving
      return next
    })
    setStatus('')
  }

  async function saveDashboard() {
    if (!canManage || !dashboard || !hasChanges || mutationPending.current) return
    mutationPending.current = true
    setIsSaving(true)
    setError('')
    setStatus('')
    try {
      const saved = await updateSharedReportDashboard({ revision: dashboard.revision, widgets: draft })
      if (saved.revision !== dashboard.revision + 1) {
        throw new Error('The shared dashboard revision did not advance. Refresh before retrying.')
      }
      setDashboard(saved)
      setDraft(draftFromDashboard(saved))
      setStatus('Shared dashboard updated. Everyone in this workspace will see the same snapshot on Dashboard.')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update the shared report dashboard.')
    } finally {
      mutationPending.current = false
      setIsSaving(false)
    }
  }

  return (
    <Card className="report-dashboard-settings">
      <div className="card-stack">
        <div>
          <p className="eyebrow">Shared analytics</p>
          <h2>Dashboard reports</h2>
          <p>Choose up to six active grouped-bar reports. They run together in one tenant-scoped snapshot and appear for every workspace member on Dashboard.</p>
        </div>
        {isLoading ? <p className="field-hint" role="status">Loading shared dashboard configuration…</p> : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {error ? <InlineError message={error} onRetry={loadDashboard} retryLabel="Reload shared dashboard" /> : null}
        {dashboard ? (
          <>
            <div className="record-list" role="list" aria-label="Selected dashboard reports">
              {draft.length === 0 ? (
                <article className="record-row" role="listitem">
                  <div>
                    <p>No dashboard reports selected.</p>
                    <p className="field-hint">Add an active grouped-bar report to publish the first shared chart.</p>
                  </div>
                </article>
              ) : draft.map((widget, index) => {
                const definition = definitionMap.get(widget.reportDefinitionId)
                const isEditable = editableDashboardDefinition(definition)
                return (
                  <article className={isEditable ? 'record-row' : 'record-row record-row-alert'} key={widget.reportDefinitionId} role="listitem">
                    <div>
                      <p>{definition?.name || `Report #${widget.reportDefinitionId}`}</p>
                      <p className="field-hint">Position {index + 1} · {widget.width === 'full' ? 'Full row' : 'Half row'}{isEditable ? '' : ' · Remove or reactivate this report before saving'}</p>
                    </div>
                    {canManage ? (
                      <div className="report-dashboard-widget-controls">
                        <label className="field-hint">
                          Width for {definition?.name || `report ${widget.reportDefinitionId}`}
                          <select className="text-input" value={widget.width} onChange={(event) => updateWidth(widget.reportDefinitionId, event.target.value)}>
                            <option value="half">Half row</option>
                            <option value="full">Full row</option>
                          </select>
                        </label>
                        <div className="button-row">
                          <Button className="button-secondary" type="button" disabled={index === 0 || isSaving} onClick={() => moveReport(index, -1)}>Move up</Button>
                          <Button className="button-secondary" type="button" disabled={index === draft.length - 1 || isSaving} onClick={() => moveReport(index, 1)}>Move down</Button>
                          <Button className="button-secondary" type="button" disabled={isSaving} onClick={() => removeReport(widget.reportDefinitionId)}>Remove</Button>
                        </div>
                      </div>
                    ) : null}
                  </article>
                )
              })}
            </div>
            {canManage ? (
              <div className="card-stack">
                <label className="field-hint">
                  Add grouped-bar report
                  <select className="text-input" value={addDefinitionId} disabled={draft.length >= 6 || availableDefinitions.length === 0 || isSaving} onChange={(event) => setAddDefinitionId(event.target.value)}>
                    <option value="">Choose a report</option>
                    {availableDefinitions.map((definition) => <option value={definition.id} key={definition.id}>{definition.name}</option>)}
                  </select>
                </label>
                <div className="button-row">
                  <Button className="button-secondary" type="button" disabled={!addDefinitionId || draft.length >= 6 || isSaving} onClick={addReport}>Add to dashboard</Button>
                  <Button type="button" disabled={!hasChanges || isSaving} onClick={saveDashboard}>{isSaving ? 'Saving dashboard…' : 'Save dashboard'}</Button>
                </div>
                {draft.length >= 6 ? <p className="field-hint" role="status">The dashboard has reached its six-report limit.</p> : null}
              </div>
            ) : <p className="field-hint">Workspace writers manage this shared configuration. You can run and review every published chart on Dashboard.</p>}
            <p className="field-hint">Showing {definitions.length} of {totalDefinitions} stored definitions while choosing reports. Only active grouped-bar reports are eligible.</p>
            {hasMoreDefinitions && onLoadMore ? <Button className="button-secondary" type="button" disabled={isLoadingMore || isSaving} onClick={onLoadMore}>{isLoadingMore ? 'Loading more definitions…' : 'Load more definitions for dashboard'}</Button> : null}
          </>
        ) : null}
      </div>
    </Card>
  )
}
