import { describe, expect, it } from 'vitest'
import { companiesExportURL } from './companies'
import { contactsExportURL } from './contacts'
import { dealsExportURL, quotePDFURL } from './deals'
import { tasksExportURL } from './tasks'

describe('export URL helpers', () => {
  it('builds filtered contacts and clients export URLs', () => {
    expect(contactsExportURL('Morgan Lee')).toBe('https://crmserver.mendola.tech/api/export/contacts?q=Morgan+Lee')
    expect(companiesExportURL('Northstar')).toBe('https://crmserver.mendola.tech/api/export/companies?q=Northstar')
  })

  it('builds filtered deals export URLs', () => {
    expect(dealsExportURL({ search: 'bluebird', pipelineId: 8, stageId: 2, ownerUserId: 1 })).toBe('https://crmserver.mendola.tech/api/export/deals?q=bluebird&pipelineId=8&stageId=2&ownerUserId=1')
  })

  it('builds deal quote PDF URLs', () => {
    expect(quotePDFURL(12)).toBe('https://crmserver.mendola.tech/api/deals/12/quote.pdf')
  })

  it('builds filtered task export URLs for the visible view', () => {
    expect(tasksExportURL({ search: 'call', status: 'open', due: 'overdue', assignee: 'unassigned', entityType: 'contact', entityId: 7 })).toBe('https://crmserver.mendola.tech/api/export/tasks?status=open&due=overdue&assignee=unassigned&entityType=contact&entityId=7&q=call')
  })
})
