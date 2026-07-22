package app

import (
	"context"

	modulequotetemplates "github.com/aeml/open_crm/apps/api/internal/modules/quotetemplates"
)

type quoteTemplatesService interface {
	ListByOrganization(context.Context, int64, modulequotetemplates.ListQuery) (modulequotetemplates.ListPage, error)
	GetPolicy(context.Context, int64) (modulequotetemplates.Policy, error)
	Create(context.Context, int64, int64, modulequotetemplates.Input) (modulequotetemplates.Template, error)
	Update(context.Context, int64, int64, int64, modulequotetemplates.Input) (modulequotetemplates.Template, error)
	Archive(context.Context, int64, int64, int64, int) (modulequotetemplates.Template, error)
	UpdatePolicy(context.Context, int64, int64, bool) (modulequotetemplates.Policy, error)
}
