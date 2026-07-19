package db

import "strings"

func DefaultDealStagesForBusinessType(businessType string) []DealStageSeed {
	switch strings.TrimSpace(strings.ToLower(businessType)) {
	case "services":
		return []DealStageSeed{
			{Name: "Lead", Position: 1, ProbabilityPercent: 20},
			{Name: "Discovery", Position: 2, ProbabilityPercent: 40},
			{Name: "Scope", Position: 3, ProbabilityPercent: 60},
			{Name: "Quote", Position: 4, ProbabilityPercent: 80},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true, ProbabilityPercent: 100},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false, ProbabilityPercent: 0},
		}
	case "product-sales":
		return []DealStageSeed{
			{Name: "Prospect", Position: 1, ProbabilityPercent: 20},
			{Name: "Qualified", Position: 2, ProbabilityPercent: 40},
			{Name: "Demo", Position: 3, ProbabilityPercent: 60},
			{Name: "Proposal", Position: 4, ProbabilityPercent: 80},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true, ProbabilityPercent: 100},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false, ProbabilityPercent: 0},
		}
	case "construction-services":
		return []DealStageSeed{
			{Name: "Lead", Position: 1, ProbabilityPercent: 20},
			{Name: "Site Visit", Position: 2, ProbabilityPercent: 40},
			{Name: "Estimate", Position: 3, ProbabilityPercent: 60},
			{Name: "Contract", Position: 4, ProbabilityPercent: 80},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true, ProbabilityPercent: 100},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false, ProbabilityPercent: 0},
		}
	default:
		return DefaultDealStages()
	}
}
