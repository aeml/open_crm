import { describe, expect, it } from 'vitest'
import { formatMoney, pipelineLabels, quoteCurrencyDisclosure } from './deal_view'

describe('deal view presentation contracts', () => {
	it('keeps deal and service-job labels equivalent after shared construction', () => {
		expect(pipelineLabels('general')).toMatchObject({ collection: 'Deals', singular: 'Deal', companyLabel: 'Company', valueLabel: 'Value amount', moveAction: 'Move to stage' })
		expect(pipelineLabels('construction-services')).toMatchObject({ collection: 'Jobs', singular: 'Job', companyLabel: 'Client', valueLabel: 'Job value', moveAction: 'Move job to stage' })
	})

	it('uses retained server FX wording and identifies legacy versions', () => {
		expect(quoteCurrencyDisclosure({ fxDisclosure: { displayText: 'Retained FX evidence' } })).toBe('Retained FX evidence')
		expect(quoteCurrencyDisclosure({})).toMatch(/legacy version/i)
		expect(formatMoney('not-a-number', 'EUR')).toBe('$0.00')
	})
})
