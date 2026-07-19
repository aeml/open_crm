package billing

import (
	"context"
	"time"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

const suspendedJobRetryDelay = 15 * time.Minute

// WriteEnforcer is the narrow billing decision required by asynchronous tenant
// work. Keeping the worker boundary on the same decision as HTTP mutations
// prevents suspended workspaces from continuing provider effects in the
// background.
type WriteEnforcer interface {
	EnforceWritable(context.Context, int64) error
}

// GuardJobHandler applies hosted write policy before tenant-scoped background
// work. Subscription blocks are deferred without consuming retry attempts so
// payment recovery can resume the original durable work safely. Billing
// reconciliation itself must not be wrapped because it is part of recovery.
func GuardJobHandler(enforcer WriteEnforcer, handler modulejobs.Handler) modulejobs.Handler {
	if enforcer == nil || handler == nil {
		return handler
	}
	return func(ctx context.Context, job modulejobs.Job) (map[string]any, error) {
		if err := enforcer.EnforceWritable(ctx, job.OrganizationID); err != nil {
			return nil, modulejobs.Deferred(err, time.Now().UTC().Add(suspendedJobRetryDelay))
		}
		return handler(ctx, job)
	}
}
