package app

import (
	"testing"
	"time"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
)

func TestEmailEngagementViewHidesExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	activeExpiry := now.Add(time.Hour)
	expiredAt := now.Add(-time.Second)
	observedAt := now.Add(-time.Minute)

	active := emailEngagementViewFor(moduleemailmessages.Message{
		EngagementTrackingEnabled: true, EngagementTrackingExpiresAt: &activeExpiry,
		OpenCount: 2, FirstOpenedAt: &observedAt, ClickCount: 3, FirstClickedAt: &observedAt,
	}, now)
	if active.state != "active" || active.openCount != 2 || active.clickCount != 3 {
		t.Fatalf("unexpected active engagement view: %#v", active)
	}
	expired := emailEngagementViewFor(moduleemailmessages.Message{
		EngagementTrackingEnabled: true, EngagementTrackingExpiresAt: &expiredAt,
		OpenCount: 2, FirstOpenedAt: &observedAt, ClickCount: 3, FirstClickedAt: &observedAt,
	}, now)
	if expired.state != "expired" || expired.openCount != 0 || expired.clickCount != 0 || expired.firstOpenedAt != "" || expired.firstClickedAt != "" {
		t.Fatalf("expired evidence must not be exposed: %#v", expired)
	}
	if off := emailEngagementViewFor(moduleemailmessages.Message{}, now); off.state != "not_enabled" {
		t.Fatalf("unexpected disabled engagement view: %#v", off)
	}
}
