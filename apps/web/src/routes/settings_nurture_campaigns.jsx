import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listEmailSequences } from '../lib/email_sequences'
import { listLeadAudiences } from '../lib/lead_audiences'
import { createNurtureCampaign, listNurtureCampaigns, updateNurtureCampaign } from '../lib/nurture_campaigns'
import { usePageTitle } from '../lib/use_page_title'

function emptyForm() {
  return {
    name: '',
    description: '',
    audienceId: '',
    sequenceId: '',
    status: 'draft'
  }
}

function formFromCampaign(campaign) {
  return {
    name: campaign.name || '',
    description: campaign.description || '',
    audienceId: campaign.audienceId ? String(campaign.audienceId) : '',
    sequenceId: campaign.sequenceId ? String(campaign.sequenceId) : '',
    status: campaign.status || 'draft'
  }
}

function payloadFromForm(form) {
  return {
    name: form.name,
    description: form.description,
    audienceId: Number.parseInt(String(form.audienceId || 0), 10) || 0,
    sequenceId: Number.parseInt(String(form.sequenceId || 0), 10) || 0,
    status: form.status
  }
}

function sequenceLabel(sequence) {
  return `${sequence.name} (${sequence.status || 'draft'})`
}

export function SettingsNurtureCampaignsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Nurture Campaigns')
  const [campaigns, setCampaigns] = useState([])
  const [audiences, setAudiences] = useState([])
  const [sequences, setSequences] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadData({ signal } = {}) {
    setIsLoading(true)
    try {
      const [nextCampaigns, nextAudienceCatalog, nextSequences] = await Promise.all([
        listNurtureCampaigns({ signal }),
        listLeadAudiences({ signal }),
        listEmailSequences({ signal })
      ])
      setCampaigns(nextCampaigns)
      setAudiences(nextAudienceCatalog.audiences.filter((audience) => audience.isActive !== false))
      setSequences(nextSequences)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load nurture campaigns.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadData({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(campaign) {
    setEditingId(campaign.id)
    setForm(formFromCampaign(campaign))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return

    setIsSaving(true)
    setStatus('')
    try {
      const payload = payloadFromForm(form)
      if (editingId) {
        const updated = await updateNurtureCampaign(editingId, payload)
        setCampaigns((current) => current.map((campaign) => (campaign.id === editingId ? updated : campaign)))
        setStatus('Nurture campaign updated.')
      } else {
        const created = await createNurtureCampaign(payload)
        setCampaigns((current) => [created, ...current])
        setStatus('Nurture campaign created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save nurture campaign.')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Nurture campaigns</h2>
              <p>Connect saved audiences to email sequences before adding automatic enrollment rules.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading nurture campaigns...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadData()} retryLabel="Retry nurture campaigns" /> : null}
          <div className="record-list" role="list" aria-label="Nurture campaigns">
            {!isLoading && campaigns.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No nurture campaigns yet.</p>
                  <p className="field-hint">Create a plan from an active audience and an email sequence.</p>
                </div>
              </article>
            ) : campaigns.map((campaign) => (
              <article className={campaign.status === 'archived' ? 'record-row record-row-alert' : 'record-row'} key={campaign.id} role="listitem">
                <div>
                  <h3>{campaign.name}</h3>
                  <p className="field-hint">{campaign.status || 'draft'} | {campaign.audienceName || 'Audience'} | {campaign.sequenceName || 'Sequence'} ({campaign.sequenceStatus || 'draft'})</p>
                  {campaign.description ? <p>{campaign.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{campaign.eligibleCount || 0} eligible</span>
                  <span className="chip">{campaign.enrolledCount || 0} enrolled</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(campaign)}>Edit</Button> : null}
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit nurture campaign' : 'New nurture campaign'}</h2>
              <p className="field-hint">Active nurture campaigns require an active sequence. Automatic audience enrollment ships later.</p>
            </div>
            <Field label="Campaign name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Demo request nurture" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Contacts captured from demo campaigns." />
            </Field>
            <Field label="Audience" hint={audiences.length === 0 ? 'Create an active audience before saving a nurture campaign.' : ''}>
              <select className="text-input" value={form.audienceId} onChange={(event) => setForm({ ...form, audienceId: event.target.value })} required>
                <option value="">Choose an audience</option>
                {audiences.map((audience) => <option key={audience.id} value={audience.id}>{audience.name} ({audience.memberCount || 0})</option>)}
              </select>
            </Field>
            <Field label="Email sequence" hint={sequences.length === 0 ? 'Create an email sequence before saving a nurture campaign.' : ''}>
              <select className="text-input" value={form.sequenceId} onChange={(event) => setForm({ ...form, sequenceId: event.target.value })} required>
                <option value="">Choose a sequence</option>
                {sequences.map((sequence) => <option key={sequence.id} value={sequence.id}>{sequenceLabel(sequence)}</option>)}
              </select>
            </Field>
            <Field label="Status">
              <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
                <option value="draft">Draft</option>
                <option value="active">Active</option>
                <option value="paused">Paused</option>
                <option value="archived">Archived</option>
              </select>
            </Field>
            <div className="button-row">
              <Button type="submit" disabled={isSaving || audiences.length === 0 || sequences.length === 0}>{isSaving ? 'Saving...' : editingId ? 'Save nurture campaign' : 'Create nurture campaign'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
