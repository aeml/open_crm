import { describe, expect, it } from 'vitest'
import { companiesExportURL } from './companies'
import { contactsExportURL } from './contacts'
import { dealsExportURL, quotePDFURL, quoteVersionPDFURL } from './deals'
import { tasksExportURL } from './tasks'
import { crmExportDownloadURL, crmExportOwnership, crmExportSetupURL } from './crm_exports'

describe('export URL helpers', () => {
  it('builds filtered contacts and clients export URLs', () => {
    expect(contactsExportURL('Morgan Lee')).toBe('https://crmserver.mendola.tech/api/export/contacts?q=Morgan+Lee')
    expect(companiesExportURL('Northstar')).toBe('https://crmserver.mendola.tech/api/export/companies?q=Northstar')
    expect(contactsExportURL({ search: 'Morgan', customField: { fieldKey: 'region', operator: 'eq', value: 'West' } })).toBe('https://crmserver.mendola.tech/api/export/contacts?q=Morgan&customField=region&customOperator=eq&customValue=West')
  })

  it('builds filtered deals export URLs', () => {
    expect(dealsExportURL({ search: 'bluebird', pipelineId: 8, stageId: 2, ownerUserId: 1, closeFrom: '2026-04-01', closeTo: '2026-06-30' })).toBe('https://crmserver.mendola.tech/api/export/deals?q=bluebird&pipelineId=8&stageId=2&ownerUserId=1&closeFrom=2026-04-01&closeTo=2026-06-30')
  })

  it('builds deal quote PDF URLs', () => {
    expect(quotePDFURL(12)).toBe('https://crmserver.mendola.tech/api/deals/12/quote.pdf')
    expect(quoteVersionPDFURL(12, 71)).toBe('https://crmserver.mendola.tech/api/deals/12/quotes/71/pdf')
  })

  it('builds filtered task export URLs for the visible view', () => {
    expect(tasksExportURL({ search: 'call', status: 'open', due: 'overdue', assignee: 'unassigned', entityType: 'contact', entityId: 7 })).toBe('https://crmserver.mendola.tech/api/export/tasks?status=open&due=overdue&assignee=unassigned&entityType=contact&entityId=7&q=call')
  })

  it('carries exact filters into durable CRM export setup and download URLs', () => {
    const request = { resource: 'deals', search: 'bluebird', pipelineId: 8, closeFrom: '2026-04-01', ...crmExportOwnership('unassigned') }
    const setup = new URL(crmExportSetupURL(request), 'https://crm.mendola.tech')
    expect(JSON.parse(setup.searchParams.get('crmExport'))).toEqual({ ...request, ownerUserId: 0, unassigned: true })
    expect(crmExportDownloadURL(17)).toBe('https://crmserver.mendola.tech/api/crm-exports/17/download')
  })

  it('preserves assigned and unassigned ownership in direct CRM exports', () => {
    expect(contactsExportURL({ search: 'Morgan', ...crmExportOwnership('7') })).toContain('ownerUserId=7')
    expect(companiesExportURL({ ...crmExportOwnership('unassigned') })).toContain('unassigned=true')
    expect(dealsExportURL({ ...crmExportOwnership('unassigned') })).toContain('unassigned=true')
  })
})
