package db

type DealStageSeed struct {
	Name               string
	Position           int
	IsClosed           bool
	IsWon              bool
	ProbabilityPercent int
}

func DefaultDealStages() []DealStageSeed {
	return []DealStageSeed{
		{Name: "Lead", Position: 1, ProbabilityPercent: 20},
		{Name: "Qualified", Position: 2, ProbabilityPercent: 40},
		{Name: "Proposal", Position: 3, ProbabilityPercent: 60},
		{Name: "Negotiation", Position: 4, ProbabilityPercent: 80},
		{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true, ProbabilityPercent: 100},
		{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false, ProbabilityPercent: 0},
	}
}
