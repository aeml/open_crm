package db

import "strings"

func DefaultDealStagesForBusinessType(businessType string) []DealStageSeed {
	switch strings.TrimSpace(strings.ToLower(businessType)) {
	case "services":
		return []DealStageSeed{
			{Name: "Lead", Position: 1},
			{Name: "Discovery", Position: 2},
			{Name: "Scoping", Position: 3},
			{Name: "Proposal", Position: 4},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false},
		}
	case "product-sales":
		return []DealStageSeed{
			{Name: "Prospect", Position: 1},
			{Name: "Qualified", Position: 2},
			{Name: "Demo", Position: 3},
			{Name: "Proposal", Position: 4},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false},
		}
	case "construction-services":
		return []DealStageSeed{
			{Name: "Lead", Position: 1},
			{Name: "Site Visit", Position: 2},
			{Name: "Estimate", Position: 3},
			{Name: "Contract", Position: 4},
			{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true},
			{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false},
		}
	default:
		return DefaultDealStages()
	}
}
