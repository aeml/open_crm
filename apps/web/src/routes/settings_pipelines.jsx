import { useEffect, useMemo, useState } from 'react'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { createDealPipeline, createDealStage, listDealPipelines, reorderDealStages, updateDealPipeline, updateDealStageDefinition } from '../lib/deals'
import { usePageTitle } from '../lib/use_page_title'

function stageOutcome(stage) {
  if (stage.isWon) return 'won'
  if (stage.isClosed) return 'lost'
  return 'open'
}

function pipelineEditors(pipelines) {
  return Object.fromEntries(pipelines.map((pipeline) => [pipeline.id, { name: pipeline.name, makeDefault: false }]))
}

function probabilityForOutcome(outcome, current = 50) {
  if (outcome === 'won') return 100
  if (outcome === 'lost') return 0
  return current
}

function stageEditors(pipelines) {
  return Object.fromEntries(pipelines.flatMap((pipeline) => (pipeline.stages || []).map((stage) => [stage.id, { name: stage.name, outcome: stageOutcome(stage), probabilityPercent: stage.probabilityPercent ?? 50 }])))
}

export function SettingsPipelinesRoute() {
  const { session } = useAuth()
  usePageTitle('Pipelines')
  const canManage = useMemo(() => ['owner', 'admin'].includes(session?.membership?.role || ''), [session])
  const [pipelines, setPipelines] = useState([])
  const [pipelineEditing, setPipelineEditing] = useState({})
  const [stageEditing, setStageEditing] = useState({})
  const [stageForms, setStageForms] = useState({})
  const [newPipelineName, setNewPipelineName] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  function applyPipelines(nextPipelines) {
    setPipelines(nextPipelines)
    setPipelineEditing(pipelineEditors(nextPipelines))
    setStageEditing(stageEditors(nextPipelines))
  }

  async function load({ signal } = {}) {
    if (!canManage) return
    setIsLoading(true)
    try {
      applyPipelines(await listDealPipelines({ signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load pipelines.')
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [canManage])

  function replacePipeline(updated) {
    const exists = pipelines.some((pipeline) => pipeline.id === updated.id)
    const current = exists ? pipelines : [...pipelines, updated]
    const next = current.map((pipeline) => pipeline.id === updated.id ? updated : updated.isDefault ? { ...pipeline, isDefault: false } : pipeline)
    applyPipelines(next.sort((left, right) => left.position - right.position || left.id - right.id))
  }

  async function save(action, successMessage) {
    setIsSaving(true)
    setError('')
    try {
      const updated = await action()
      replacePipeline(updated)
      setNotice(successMessage)
      return true
    } catch (saveError) {
      setError(saveError.message || 'Unable to update pipeline configuration.')
      return false
    } finally {
      setIsSaving(false)
    }
  }

  async function handleCreatePipeline(event) {
    event.preventDefault()
    await save(async () => {
      const created = await createDealPipeline({ name: newPipelineName })
      setNewPipelineName('')
      return created
    }, 'Pipeline created with a complete stage template.')
  }

  function moveStage(pipeline, stageIndex, direction) {
    const targetIndex = stageIndex + direction
    if (targetIndex < 0 || targetIndex >= pipeline.stages.length) return
    const stageIds = pipeline.stages.map((stage) => stage.id)
    ;[stageIds[stageIndex], stageIds[targetIndex]] = [stageIds[targetIndex], stageIds[stageIndex]]
    save(() => reorderDealStages(pipeline.id, stageIds), 'Stage order updated. Existing deals kept their stage identities.')
  }

  if (!canManage) return <section className="dashboard-grid settings-grid"><Card><InlineError message="Admin access required" /></Card></section>

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <form className="card-stack" onSubmit={handleCreatePipeline}>
          <div><h2>Pipeline configuration</h2><p>Create up to 10 pipelines and manage up to 20 stable stages in each. Existing deals stay attached by stage ID.</p></div>
          <Field label="New pipeline name"><input className="text-input" maxLength="100" required value={newPipelineName} onChange={(event) => setNewPipelineName(event.target.value)} /></Field>
          <Button type="submit" disabled={isSaving || pipelines.length >= 10}>{isSaving ? 'Saving…' : 'Create pipeline'}</Button>
          {pipelines.length >= 10 ? <p className="field-hint">The 10-pipeline limit is reached.</p> : null}
          {error ? <InlineError message={error} onRetry={() => load()} /> : null}
          {notice ? <div className="inline-note" role="status">{notice}</div> : null}
        </form>
      </Card>
      {isLoading ? <Card><p role="status">Loading pipeline configuration...</p></Card> : null}
      {pipelines.map((pipeline) => {
        const pipelineValue = pipelineEditing[pipeline.id] || { name: pipeline.name, makeDefault: false }
        const stageForm = stageForms[pipeline.id] || { name: '', outcome: 'open', probabilityPercent: 50 }
        return (
          <Card key={pipeline.id}>
            <div className="card-stack">
              <div className="section-header"><div><h2>{pipeline.name}</h2><p>{pipeline.isDefault ? 'Default pipeline' : `Pipeline ${pipeline.position}`} · {pipeline.stages.length} stages</p></div></div>
              <Field label={`Pipeline name for ${pipeline.name}`}><input className="text-input" maxLength="100" value={pipelineValue.name} onChange={(event) => setPipelineEditing((current) => ({ ...current, [pipeline.id]: { ...pipelineValue, name: event.target.value } }))} /></Field>
              {!pipeline.isDefault ? <label><input type="checkbox" checked={pipelineValue.makeDefault} onChange={(event) => setPipelineEditing((current) => ({ ...current, [pipeline.id]: { ...pipelineValue, makeDefault: event.target.checked } }))} /> Make this the default pipeline</label> : <p className="field-hint">This is the default pipeline.</p>}
              <div><Button disabled={isSaving} onClick={() => save(() => updateDealPipeline(pipeline.id, pipelineValue), `${pipelineValue.name} updated.`)}>Save pipeline</Button></div>
              <div className="record-list" role="list" aria-label={`${pipeline.name} stages`}>
                {pipeline.stages.map((stage, index) => {
                  const value = stageEditing[stage.id] || { name: stage.name, outcome: stageOutcome(stage), probabilityPercent: stage.probabilityPercent ?? 50 }
                  return (
                    <article className="record-row" role="listitem" key={stage.id}>
                      <div className="card-stack">
                        <Field label={`Stage name for ${stage.name}`}><input className="text-input" maxLength="100" value={value.name} onChange={(event) => setStageEditing((current) => ({ ...current, [stage.id]: { ...value, name: event.target.value } }))} /></Field>
                        <Field label={`Outcome for ${stage.name}`}><select className="text-input" value={value.outcome} onChange={(event) => { const outcome = event.target.value; setStageEditing((current) => ({ ...current, [stage.id]: { ...value, outcome, probabilityPercent: probabilityForOutcome(outcome, value.probabilityPercent) } })) }}><option value="open">Open</option><option value="won">Won</option><option value="lost">Lost</option></select></Field>
                        <Field label={`Probability for ${stage.name}`} hint={value.outcome === 'open' ? 'Used to weight open deal value in the forecast.' : 'Won stages are fixed at 100%; lost stages are fixed at 0%.'}><input className="text-input" type="number" min="0" max="100" required disabled={value.outcome !== 'open'} value={value.probabilityPercent} onChange={(event) => setStageEditing((current) => ({ ...current, [stage.id]: { ...value, probabilityPercent: Number.parseInt(event.target.value, 10) } }))} /></Field>
                        <div className="button-row">
                          <Button className="button-secondary" disabled={isSaving || index === 0} onClick={() => moveStage(pipeline, index, -1)}>Move {stage.name} up</Button>
                          <Button className="button-secondary" disabled={isSaving || index === pipeline.stages.length - 1} onClick={() => moveStage(pipeline, index, 1)}>Move {stage.name} down</Button>
                          <Button disabled={isSaving} onClick={() => save(() => updateDealStageDefinition(pipeline.id, stage.id, value), `${value.name} updated without changing its stage ID.`)}>Save {stage.name}</Button>
                        </div>
                      </div>
                    </article>
                  )
                })}
              </div>
              <form className="auth-form" onSubmit={(event) => { event.preventDefault(); save(() => createDealStage(pipeline.id, stageForm), `${stageForm.name} added.`).then((saved) => { if (saved) setStageForms((current) => ({ ...current, [pipeline.id]: { name: '', outcome: 'open', probabilityPercent: 50 } })) }) }}>
                <Field label={`New stage name for ${pipeline.name}`}><input className="text-input" maxLength="100" required value={stageForm.name} onChange={(event) => setStageForms((current) => ({ ...current, [pipeline.id]: { ...stageForm, name: event.target.value } }))} /></Field>
                <Field label={`New stage outcome for ${pipeline.name}`}><select className="text-input" value={stageForm.outcome} onChange={(event) => { const outcome = event.target.value; setStageForms((current) => ({ ...current, [pipeline.id]: { ...stageForm, outcome, probabilityPercent: probabilityForOutcome(outcome, stageForm.probabilityPercent) } })) }}><option value="open">Open</option><option value="won">Won</option><option value="lost">Lost</option></select></Field>
                <Field label={`New stage probability for ${pipeline.name}`} hint="The percentage of open deal value counted in the weighted forecast."><input className="text-input" type="number" min="0" max="100" required disabled={stageForm.outcome !== 'open'} value={stageForm.probabilityPercent} onChange={(event) => setStageForms((current) => ({ ...current, [pipeline.id]: { ...stageForm, probabilityPercent: Number.parseInt(event.target.value, 10) } }))} /></Field>
                <Button type="submit" disabled={isSaving || pipeline.stages.length >= 20}>Add stage</Button>
                {pipeline.stages.length >= 20 ? <p className="field-hint">The 20-stage limit is reached.</p> : null}
              </form>
            </div>
          </Card>
        )
      })}
    </section>
  )
}
