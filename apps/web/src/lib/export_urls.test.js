import { describe, expect, it } from 'vitest'
import { companiesExportURL } from './companies'
import { contactsExportURL } from './contacts'
import { dealsExportURL, quotePDFURL, quoteVersionPDFURL } from './deals'
import { tasksExportURL } from './tasks'

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
})
