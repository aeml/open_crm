import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getPublicLeadLandingPage } from '../lib/landing_pages'
import { usePageTitle } from '../lib/use_page_title'
import { PublicLeadCaptureForm } from './public_lead_capture_form'

export function PublicLandingPageRoute() {
  const { slug = '' } = useParams()
  const [landingPage, setLandingPage] = useState(null)
  const [form, setForm] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  usePageTitle(landingPage?.title || 'Landing Page')

  async function loadLandingPage({ signal } = {}) {
    setIsLoading(true)
    try {
      const result = await getPublicLeadLandingPage(slug, { signal })
      setLandingPage(result?.page || null)
      setForm(result?.form || null)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load landing page.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadLandingPage({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [slug])

  const theme = landingPage?.theme || 'light'

  return (
    <main className={`landing-page landing-page-${theme}`}>
      <section className="landing-page-panel">
        {isLoading ? <p className="field-hint">Loading landing page...</p> : null}
        {error && !landingPage ? <InlineError message={error} onRetry={() => loadLandingPage()} retryLabel="Retry page" /> : null}
        {landingPage && form ? (
          <div className="landing-page-grid">
            <div className="landing-page-copy">
              <p className="eyebrow">{form.sourceLabel || 'Lead capture'}</p>
              <h1>{landingPage.title}</h1>
              {landingPage.subtitle ? <p className="landing-page-subtitle">{landingPage.subtitle}</p> : null}
              {landingPage.body ? <p className="message-body">{landingPage.body}</p> : null}
            </div>
            <PublicLeadCaptureForm form={form} submitLabel={landingPage.ctaLabel || 'Submit'} />
          </div>
        ) : null}
      </section>
    </main>
  )
}
