import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listLeadAudiences } from '../lib/lead_audiences'
import { createMarketingEmailCampaign, listMarketingEmailCampaigns, updateMarketingEmailCampaign } from '../lib/marketing_email_campaigns'
import { usePageTitle } from '../lib/use_page_title'

function emptyForm() {
  return {
    name: '',
    description: '',
    audienceId: '',
    subject: '',
    previewText: '',
    body: '',
    status: 'draft',
    scheduledAt: ''
  }
}

function toDatetimeLocal(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

function formFromCampaign(campaign) {
  return {
    name: campaign.name || '',
    description: campaign.description || '',
    audienceId: campaign.audienceId ? String(campaign.audienceId) : '',
    subject: campaign.subject || '',
    previewText: campaign.previewText || '',
    body: campaign.body || '',
    status: campaign.status || 'draft',
    scheduledAt: toDatetimeLocal(campaign.scheduledAt)
  }
}

function payloadFromForm(form) {
  return {
    name: form.name,
    description: form.description,
    audienceId: Number.parseInt(String(form.audienceId || 0), 10) || 0,
    subject: form.subject,
    previewText: form.previewText,
    body: form.body,
    status: form.status,
    scheduledAt: form.scheduledAt ? new Date(form.scheduledAt).toISOString() : null
  }
}

function formatDateTime(value) {
  if (!value) return 'Not scheduled'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Not scheduled'
  return date.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function analyticsValue(campaign, key) {
  return campaign.analytics?.[key] || 0
}

export function SettingsMarketingEmailCampaignsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Email Campaigns')
  const [campaigns, setCampaigns] = useState([])
  const [audiences, setAudiences] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadData({ signal } = {}) {
    setIsLoading(true)
    try {
      const [nextCampaigns, nextAudiences] = await Promise.all([
        listMarketingEmailCampaigns({ signal }),
        listLeadAudiences({ signal })
      ])
      setCampaigns(nextCampaigns)
      setAudiences(nextAudiences.filter((audience) => audience.isActive !== false))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email campaigns.')
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
        const updated = await updateMarketingEmailCampaign(editingId, payload)
        setCampaigns((current) => current.map((campaign) => (campaign.id === editingId ? updated : campaign)))
        setStatus('Email campaign updated.')
      } else {
        const created = await createMarketingEmailCampaign(payload)
        setCampaigns((current) => [created, ...current])
        setStatus('Email campaign created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email campaign.')
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
              <h2>Email campaigns</h2>
              <p>Plan one-time marketing sends against saved audiences, with schedule metadata and campaign analytics.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading email campaigns...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadData()} retryLabel="Retry campaigns" /> : null}
          <div className="record-list" role="list" aria-label="Email campaigns">
            {!isLoading && campaigns.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No campaigns yet.</p>
                  <p className="field-hint">Create a draft campaign from a saved audience before scheduling delivery.</p>
                </div>
              </article>
            ) : campaigns.map((campaign) => (
              <article className={campaign.status === 'cancelled' ? 'record-row record-row-alert' : 'record-row'} key={campaign.id} role="listitem">
                <div>
                  <h3>{campaign.name}</h3>
                  <p className="field-hint">{campaign.status || 'draft'} | {campaign.audienceName || 'Audience'} | {formatDateTime(campaign.scheduledAt)}</p>
                  <p>{campaign.subject}</p>
                  {campaign.description ? <p className="field-hint">{campaign.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{analyticsValue(campaign, 'recipientCount')} recipients</span>
                  <span className="chip">{analyticsValue(campaign, 'sentCount')} sent</span>
                  <span className="chip">{analyticsValue(campaign, 'openedCount')} opens</span>
                  <span className="chip">{analyticsValue(campaign, 'clickedCount')} clicks</span>
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
              <h2>{editingId ? 'Edit campaign' : 'New campaign'}</h2>
              <p className="field-hint">This foundation stores campaign plans and analytics counters. Bulk delivery and tracking workers ship later.</p>
            </div>
            <Field label="Campaign name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Spring demo blast" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="One-time send for demo campaign leads." />
            </Field>
            <Field label="Audience" hint={audiences.length === 0 ? 'Create an active audience before saving a campaign.' : ''}>
              <select className="text-input" value={form.audienceId} onChange={(event) => setForm({ ...form, audienceId: event.target.value })} required>
                <option value="">Choose an audience</option>
                {audiences.map((audience) => <option key={audience.id} value={audience.id}>{audience.name} ({audience.memberCount || 0})</option>)}
              </select>
            </Field>
            <Field label="Status">
              <select className="text-input" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
                <option value="draft">Draft</option>
                <option value="scheduled">Scheduled</option>
                <option value="paused">Paused</option>
                <option value="cancelled">Cancelled</option>
              </select>
            </Field>
            <Field label="Scheduled time">
              <input className="text-input" type="datetime-local" value={form.scheduledAt} onChange={(event) => setForm({ ...form, scheduledAt: event.target.value })} required={form.status === 'scheduled'} />
            </Field>
            <Field label="Subject">
              <input className="text-input" value={form.subject} onChange={(event) => setForm({ ...form, subject: event.target.value })} placeholder="See what is new this spring" required />
            </Field>
            <Field label="Preview text">
              <input className="text-input" value={form.previewText} onChange={(event) => setForm({ ...form, previewText: event.target.value })} placeholder="A short inbox preview." />
            </Field>
            <Field label="Body">
              <textarea className="text-input" rows={8} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} placeholder="Write the campaign email body." required />
            </Field>
            <div className="button-row">
              <Button type="submit" disabled={isSaving || audiences.length === 0}>{isSaving ? 'Saving...' : editingId ? 'Save campaign' : 'Create campaign'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}
