package leadforms

import (
	"strings"

	platformpagination "github.com/aeml/open_crm/apps/api/internal/platform/pagination"
)

const DefaultLeadSurfaceListPageSize = 50

type LeadSurfaceListQuery struct {
	Status   string
	Page     int
	PageSize int
}

type LandingPageListPage struct {
	Pages    []LandingPage `json:"pages"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
	Total    int           `json:"total"`
}

type ChatWidgetListPage struct {
	Widgets  []ChatWidget `json:"widgets"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	Total    int          `json:"total"`
}

func normalizeLeadSurfaceListQuery(query LeadSurfaceListQuery) (LeadSurfaceListQuery, platformpagination.Page, error) {
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	if query.Status == "" {
		query.Status = "all"
	}
	if query.Status != "all" && query.Status != "active" && query.Status != "inactive" {
		return LeadSurfaceListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	page, err := platformpagination.Normalize(query.Page, query.PageSize, DefaultLeadSurfaceListPageSize)
	if err != nil {
		return LeadSurfaceListQuery{}, platformpagination.Page{}, ErrInvalidInput
	}
	query.Page = page.Number
	query.PageSize = page.Size
	return query, page, nil
}

func leadSurfaceStatusFilter(status, alias string) string {
	switch status {
	case "active":
		return " AND " + alias + ".is_active=TRUE"
	case "inactive":
		return " AND " + alias + ".is_active=FALSE"
	default:
		return ""
	}
}
