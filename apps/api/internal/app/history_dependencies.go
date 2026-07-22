package app

import (
	"context"

	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	modulenotes "github.com/aeml/open_crm/apps/api/internal/modules/notes"
	platformtimeline "github.com/aeml/open_crm/apps/api/internal/platform/timelinepagination"
)

type notesService interface {
	ListByEntity(context.Context, int64, string, int64, platformtimeline.Query) (modulenotes.Page, error)
	Create(context.Context, int64, int64, modulenotes.CreateInput) (modulenotes.CreateResult, error)
}

type activityFeedService interface {
	ListByEntity(context.Context, int64, string, int64, platformtimeline.Query) (moduleactivityfeed.Page, error)
}
