import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { createDeal, listDeals, listDealStages, updateDealStage } from '../lib/deals'

const emptyForm = {
  name: '',
  stageId: '',
  companyId: '',
  primaryContactId: '',
  status: 'open',
  valueAmount: '',
  valueCurrency: 'USD',
  expectedCloseDate: ''
}

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

export function DealsRoute() {
  const [stages, setStages] = useState([])
  const [deals, setDeals] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' })
  const [form, setForm] = useState(emptyForm)
  const [selectedDealId, setSelectedDealId] = useState(null)
  const [selectedStageId, setSelectedStageId] = useState('')
  const [activities, setActivities] = useState([])
  const [error, setError] = useState('')

  async function loadPipeline() {
    const [loadedStages, loadedDeals] = await Promise.all([listDealStages(), listDeals()])
    setStages(loadedStages)
    setDeals(loadedDeals.deals || [])
    setMeta(loadedDeals.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' })
    if (loadedStages.length > 0 && !selectedStageId) {
      setSelectedStageId(String(loadedStages[0].id))
    }
    if (loadedStages.length > 0 && !form.stageId) {
      setForm((current) => ({ ...current, stageId: String(loadedStages[0].id) }))
    }
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      try {
        await loadPipeline()
        if (!cancelled) {
          setError('')
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load deals.')
        }
      }
    }

    run()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedDeal = useMemo(() => deals.find((entry) => entry.id === selectedDealId) || null, [deals, selectedDealId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createDeal({
        name: form.name,
        stageId: Number.parseInt(form.stageId, 10),
        companyId: Number.parseInt(form.companyId, 10) || 0,
        primaryContactId: Number.parseInt(form.primaryContactId, 10) || 0,
        status: form.status,
        valueAmount: form.valueAmount,
        valueCurrency: form.valueCurrency,
        expectedCloseDate: form.expectedCloseDate
      })
      setDeals((current) => [...current, data.deal])
      setActivities(data.activities || [])
      setSelectedDealId(data.deal.id)
      setSelectedStageId(String(data.deal.stageId))
      setMeta((current) => ({
        ...current,
        total: current.total + 1,
        openCount: current.openCount + 1,
        pipelineValue: String(Number.parseFloat(current.pipelineValue || '0') + Number.parseFloat(data.deal.valueAmount || '0'))
      }))
      setForm((current) => ({ ...emptyForm, stageId: current.stageId || form.stageId || (stages[0] ? String(stages[0].id) : '') }))
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to create deal.')
    }
  }

  async function handleMoveStage() {
    if (!selectedDealId || !selectedStageId) {
      return
    }

    try {
      const data = await updateDealStage(selectedDealId, Number.parseInt(selectedStageId, 10))
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setActivities(data.activities || [])
      setError('')
    } catch (moveError) {
      setError(moveError.message || 'Unable to move deal.')
    }
  }

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Deals</h2>
              <p>Real pipeline, real stages, no fake dashboard filler.</p>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Pipeline summary">
            <article className="record-row" role="listitem">
              <div>
                <p>Open deals</p>
              </div>
              <div>
                <p>{meta.openCount}</p>
              </div>
            </article>
            <article className="record-row" role="listitem">
              <div>
                <p>Won deals</p>
              </div>
              <div>
                <p>{meta.wonCount}</p>
              </div>
            </article>
            <article className="record-row" role="listitem">
              <div>
                <p>Pipeline value</p>
              </div>
              <div>
                <p>{formatMoney(meta.pipelineValue)}</p>
              </div>
            </article>
          </div>
          {error ? <p className="form-error">{error}</p> : null}
          <div className="record-list" role="list" aria-label="Deals list">
            {deals.map((deal) => (
              <article className="record-row" key={deal.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => { setSelectedDealId(deal.id); setSelectedStageId(String(deal.stageId)); setActivities([]) }}>
                    {deal.name}
                  </button>
                  <p>{deal.stageName}</p>
                </div>
                <div>
                  <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
                  <p>{deal.companyName || 'No company linked'}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>New deal</h2>
            <p>Create pipeline entries against the real org stage list.</p>
          </div>
          <form className="auth-form" onSubmit={handleCreate}>
            <Field label="Deal name">
              <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
            </Field>
            <Field label="Stage">
              <select className="text-input" value={form.stageId} onChange={(event) => setForm((current) => ({ ...current, stageId: event.target.value }))}>
                {stages.map((stage) => (
                  <option key={stage.id} value={stage.id}>{stage.name}</option>
                ))}
              </select>
            </Field>
            <Field label="Company ID">
              <input className="text-input" value={form.companyId} onChange={(event) => setForm((current) => ({ ...current, companyId: event.target.value }))} />
            </Field>
            <Field label="Primary contact ID">
              <input className="text-input" value={form.primaryContactId} onChange={(event) => setForm((current) => ({ ...current, primaryContactId: event.target.value }))} />
            </Field>
            <Field label="Value amount">
              <input className="text-input" value={form.valueAmount} onChange={(event) => setForm((current) => ({ ...current, valueAmount: event.target.value }))} />
            </Field>
            <Field label="Value currency">
              <input className="text-input" value={form.valueCurrency} onChange={(event) => setForm((current) => ({ ...current, valueCurrency: event.target.value }))} />
            </Field>
            <Field label="Expected close date">
              <input className="text-input" type="date" value={form.expectedCloseDate} onChange={(event) => setForm((current) => ({ ...current, expectedCloseDate: event.target.value }))} />
            </Field>
            <Button type="submit">Save deal</Button>
          </form>
        </div>
      </Card>

      {selectedDeal ? (
        <Card>
          <div className="card-stack">
            <div className="section-header">
              <div>
                <h2>{selectedDeal.name}</h2>
                <p>{selectedDeal.companyName || 'No company linked'}</p>
              </div>
            </div>
            <Field label="Move stage">
              <select className="text-input" value={selectedStageId} onChange={(event) => setSelectedStageId(event.target.value)}>
                {stages.map((stage) => (
                  <option key={stage.id} value={stage.id}>{stage.name}</option>
                ))}
              </select>
            </Field>
            <Button onClick={handleMoveStage}>Move to stage</Button>
            <Card>
              <div className="card-stack">
                <h3>Activity</h3>
                <div className="record-list" role="list" aria-label="Deal activity list">
                  {activities.map((activity) => (
                    <article className="record-row" key={activity.id} role="listitem">
                      <div>
                        <p>{activity.summary}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}
