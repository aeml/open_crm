package emailsequences

import (
	"strings"
	"testing"
)

func TestSelectDueSendsSQLScopesAutomaticSending(t *testing.T) {
	lowerSQL := strings.ToLower(selectDueSendsSQL)
	for _, expected := range []string{"seq.status = 'active'", "seq.approved_revision = seq.revision", "seq.approved_at is not null", "e.status = 'active'", "e.next_send_at <= now()", "e.enrolled_by_user_id is not null", "contact.archived_at is null", "coalesce(contact.email, '') <> ''"} {
		if !strings.Contains(lowerSQL, expected) {
			t.Fatalf("expected due send SQL to include %q, got %s", expected, selectDueSendsSQL)
		}
	}
}
