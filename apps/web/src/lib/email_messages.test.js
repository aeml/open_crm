import { describe, expect, it } from 'vitest'
import { emailEngagementSummary, emailMessageTimestamp, emailRecordLabel, emailRecordPath, formatEmailTimestamp } from './email_messages'

describe('email message presentation helpers', () => {
  it('maps supported records and prefers received time', () => {
    expect(emailRecordPath({ entityType: 'contact', entityId: 4 })).toBe('/contacts/4')
    expect(emailRecordPath({ entityType: 'company', entityId: 5 })).toBe('/companies/5')
    expect(emailRecordPath({ entityType: 'deal', entityId: 6 })).toBe('/deals/6')
    expect(emailRecordPath({ entityType: 'task', entityId: 7 })).toBe('')
    expect(emailRecordLabel({ entityType: 'company', entityId: 5 })).toBe('company #5')
    expect(emailMessageTimestamp({ receivedAt: 'received', createdAt: 'created' })).toBe('received')
  })

  it('formats valid timestamps and rejects invalid values', () => {
    expect(formatEmailTimestamp('')).toBe('')
    expect(formatEmailTimestamp('not-a-date')).toBe('')
    expect(formatEmailTimestamp('2026-05-01T12:00:00Z')).not.toBe('')
  })

  it('explains active, disabled, and expired engagement evidence', () => {
    expect(emailEngagementSummary({ engagementTrackingState: 'not_enabled', openCount: 9 })).toBe('Tracking off')
    expect(emailEngagementSummary({ engagementTrackingState: 'expired', clickCount: 4 })).toMatch(/data removed/i)
    expect(emailEngagementSummary({ engagementTrackingState: 'active', openCount: 2, clickCount: 1 })).toBe('Opens 2 · clicks 1 · Active')
  })
})
