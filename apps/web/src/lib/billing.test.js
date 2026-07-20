import { describe, expect, it } from 'vitest'
import { formatInvoiceAmount } from './billing'

describe('billing amount formatting', () => {
  it('formats ordinary, zero-decimal, and Stripe compatibility currencies from provider minor units', () => {
    expect(formatInvoiceAmount(4900, 'usd', 'stripe')).toBe(new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD' }).format(49))
    expect(formatInvoiceAmount(500, 'jpy', 'stripe')).toBe(new Intl.NumberFormat(undefined, { style: 'currency', currency: 'JPY' }).format(500))
    expect(formatInvoiceAmount(500, 'isk', 'stripe')).toBe(new Intl.NumberFormat(undefined, { style: 'currency', currency: 'ISK' }).format(5))
    expect(formatInvoiceAmount(500, 'ugx', 'stripe')).toBe(new Intl.NumberFormat(undefined, { style: 'currency', currency: 'UGX' }).format(5))
  })

  it('keeps the raw minor-unit evidence visible for an invalid currency code', () => {
    expect(formatInvoiceAmount(4900, 'not-a-currency', 'stripe')).toBe('4,900 NOT-A-CURRENCY')
  })
})
