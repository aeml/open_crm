package db

type DealStageSeed struct {
	Name     string
	Position int
	IsClosed bool
	IsWon    bool
}

func DefaultDealStages() []DealStageSeed {
	return []DealStageSeed{
		{Name: "Lead", Position: 1},
		{Name: "Qualified", Position: 2},
		{Name: "Proposal", Position: 3},
		{Name: "Negotiation", Position: 4},
		{Name: "Closed Won", Position: 5, IsClosed: true, IsWon: true},
		{Name: "Closed Lost", Position: 6, IsClosed: true, IsWon: false},
	}
}
