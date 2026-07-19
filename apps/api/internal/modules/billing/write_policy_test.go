package billing

import (
	"context"
	"errors"
	"testing"

	modulejobs "github.com/aeml/open_crm/apps/api/internal/modules/jobs"
)

type fakeWriteEnforcer struct {
	organizationID int64
	err            error
}

func (f *fakeWriteEnforcer) EnforceWritable(_ context.Context, organizationID int64) error {
	f.organizationID = organizationID
	return f.err
}

func TestGuardJobHandlerScopesAndBlocksTenantEffects(t *testing.T) {
	enforcer := &fakeWriteEnforcer{err: ErrSubscriptionInactive}
	handled := false
	handler := GuardJobHandler(enforcer, func(context.Context, modulejobs.Job) (map[string]any, error) {
		handled = true
		return map[string]any{"sent": true}, nil
	})

	_, err := handler(context.Background(), modulejobs.Job{OrganizationID: 42})
	if !errors.Is(err, ErrSubscriptionInactive) || handled || enforcer.organizationID != 42 {
		t.Fatalf("expected suspended org 42 to be deferred before its effect: org=%d handled=%v err=%v", enforcer.organizationID, handled, err)
	}
}

func TestGuardJobHandlerAllowsWritableTenant(t *testing.T) {
	enforcer := &fakeWriteEnforcer{}
	handled := false
	handler := GuardJobHandler(enforcer, func(_ context.Context, job modulejobs.Job) (map[string]any, error) {
		handled = true
		return map[string]any{"organizationId": job.OrganizationID}, nil
	})

	result, err := handler(context.Background(), modulejobs.Job{OrganizationID: 7})
	if err != nil || !handled || result["organizationId"] != int64(7) {
		t.Fatalf("expected writable tenant effect: handled=%v result=%#v err=%v", handled, result, err)
	}
}
