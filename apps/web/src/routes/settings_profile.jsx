import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { updateProfile, getPreferences, updatePreferences } from '../lib/profile'
import { usePageTitle } from '../lib/use_page_title'

const landingViewOptions = [
  { value: '', label: 'Default (Dashboard)' },
  { value: '/dashboard', label: 'Dashboard' },
  { value: '/companies', label: 'Companies / Clients' },
  { value: '/deals', label: 'Deals / Jobs' },
  { value: '/tasks', label: 'Tasks' }
]

export function SettingsProfileRoute() {
  const { session, refreshSession } = useAuth()
  usePageTitle('My Profile')

  const [firstName, setFirstName] = useState(session?.user?.firstName || '')
  const [lastName, setLastName] = useState(session?.user?.lastName || '')
  const [profileError, setProfileError] = useState('')
  const [profileSaved, setProfileSaved] = useState(false)
  const [isProfileSaving, setIsProfileSaving] = useState(false)

  const [defaultLandingView, setDefaultLandingView] = useState('')
  const [notifyOnTaskAssigned, setNotifyOnTaskAssigned] = useState(true)
  const [notifyOnDealAssigned, setNotifyOnDealAssigned] = useState(true)
  const [prefsError, setPrefsError] = useState('')
  const [prefsSaved, setPrefsSaved] = useState(false)
  const [isPrefsLoading, setIsPrefsLoading] = useState(true)
  const [isPrefsSaving, setIsPrefsSaving] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setIsPrefsLoading(true)

    getPreferences({ signal: controller.signal })
      .then((prefs) => {
        setDefaultLandingView(prefs?.defaultLandingView || '')
        setNotifyOnTaskAssigned(prefs?.notifyOnTaskAssigned !== false)
        setNotifyOnDealAssigned(prefs?.notifyOnDealAssigned !== false)
        setIsPrefsLoading(false)
      })
      .catch((err) => {
        if (!isAbortError(err)) {
          setPrefsError('Unable to load preferences.')
          setIsPrefsLoading(false)
        }
      })

    return () => controller.abort()
  }, [])

  async function handleProfileSubmit(event) {
    event.preventDefault()
    setIsProfileSaving(true)
    setProfileError('')
    setProfileSaved(false)

    try {
      await updateProfile({ firstName, lastName })
      await refreshSession()
      setProfileSaved(true)
    } catch (err) {
      setProfileError(err.message || 'Unable to update profile.')
    } finally {
      setIsProfileSaving(false)
    }
  }

  async function handlePrefsSubmit(event) {
    event.preventDefault()
    setIsPrefsSaving(true)
    setPrefsError('')
    setPrefsSaved(false)

    try {
      await updatePreferences({ defaultLandingView, notifyOnTaskAssigned, notifyOnDealAssigned })
      setPrefsSaved(true)
    } catch (err) {
      setPrefsError(err.message || 'Unable to save preferences.')
    } finally {
      setIsPrefsSaving(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Personal profile</h2>
              <p>Update your display name across Open CRM.</p>
            </div>
          </div>
          {profileError ? (
            <InlineError message={profileError} />
          ) : null}
          {profileSaved ? (
            <p className="field-hint" role="status">Profile updated.</p>
          ) : null}
          <form className="auth-form" onSubmit={handleProfileSubmit}>
            <Field label="First name">
              <input
                className="text-input"
                value={firstName}
                onChange={(event) => { setFirstName(event.target.value); setProfileSaved(false) }}
                required
              />
            </Field>
            <Field label="Last name">
              <input
                className="text-input"
                value={lastName}
                onChange={(event) => { setLastName(event.target.value); setProfileSaved(false) }}
                required
              />
            </Field>
            <Field label="Email">
              <input
                className="text-input"
                type="email"
                value={session?.user?.email || ''}
                readOnly
                aria-describedby="email-hint"
              />
              <p className="field-hint" id="email-hint">Email cannot be changed here.</p>
            </Field>
            <Button type="submit" disabled={isProfileSaving}>
              {isProfileSaving ? 'Saving…' : 'Save profile'}
            </Button>
          </form>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Preferences</h2>
              <p>Choose where you land after signing in.</p>
            </div>
          </div>
          {isPrefsLoading ? (
            <p className="field-hint">Loading preferences…</p>
          ) : null}
          {prefsError ? (
            <InlineError message={prefsError} />
          ) : null}
          {prefsSaved ? (
            <p className="field-hint" role="status">Preferences saved.</p>
          ) : null}
          {!isPrefsLoading ? (
            <form className="auth-form" onSubmit={handlePrefsSubmit}>
              <Field label="Default landing view">
                <select
                  className="text-input"
                  value={defaultLandingView}
                  onChange={(event) => { setDefaultLandingView(event.target.value); setPrefsSaved(false) }}
                >
                  {landingViewOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="In-app notifications">
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={notifyOnTaskAssigned}
                    onChange={(event) => { setNotifyOnTaskAssigned(event.target.checked); setPrefsSaved(false) }}
                  />
                  Notify me when a task is assigned to me
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={notifyOnDealAssigned}
                    onChange={(event) => { setNotifyOnDealAssigned(event.target.checked); setPrefsSaved(false) }}
                  />
                  Notify me when a deal is assigned to me
                </label>
              </Field>
              <Button type="submit" disabled={isPrefsSaving}>
                {isPrefsSaving ? 'Saving…' : 'Save preferences'}
              </Button>
            </form>
          ) : null}
        </div>
      </Card>
    </section>
  )
}
