import { describe, expect, it } from 'vitest'
import { leadFormEmbedSnippet } from './lead_form_embed'

describe('lead form embed contract', () => {
  it('submits inline without credentials and hydrates exact browser attribution', () => {
    const snippet = leadFormEmbedSnippet({
      publicId: 'lf_public',
      revision: 7,
      sourceLabel: 'Website request',
      consentText: 'I agree to receive one reply.',
      successMessage: 'Thanks for the request.',
      fields: [{ key: 'email', label: 'Email', fieldType: 'email', required: true }]
    })

    expect(snippet).toContain('form.addEventListener(\'submit\'')
    expect(snippet).toContain("credentials: 'omit'")
    expect(snippet).toContain('new URLSearchParams()')
    expect(snippet).toContain('new FormData(form).entries()')
    expect(snippet).toContain('form.elements.sourceUrl.value = currentURL.href')
    expect(snippet).toContain('["utm_source","utm_medium","utm_campaign"')
    expect(snippet).toContain("setStatus(payload?.data?.successMessage || fallbackSuccess, 'success')")
    expect(snippet).toContain('await prepare({ preserveStatus: true })')
    expect(snippet).toContain('Number(challenge.formRevision) !== expectedRevision')
  })

  it('escapes HTML attributes and script-owned values', () => {
    const snippet = leadFormEmbedSnippet({
      publicId: 'lf_public',
      revision: 1,
      sourceLabel: 'Source" autofocus onfocus="alert(1)',
      consentText: '<img src=x onerror=alert(1)>',
      successMessage: '</script><script>alert(1)</script>',
      fields: [{ key: 'email', label: '<Email>', fieldType: 'email', required: true }]
    })

    expect(snippet).toContain('value="Source&quot; autofocus onfocus=&quot;alert(1)"')
    expect(snippet).toContain('&lt;img src=x onerror=alert(1)&gt;')
    expect(snippet).toContain('&lt;Email&gt;')
    expect(snippet).not.toContain('</script><script>alert(1)</script>')
    expect(snippet).toContain('\\u003c/script\\u003e\\u003cscript\\u003ealert(1)')
  })
})
