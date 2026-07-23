package app

import (
	"errors"
	"net/http"

	moduledeals "github.com/aeml/open_crm/apps/api/internal/modules/deals"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

func writeDealWorkflowActionError(w http.ResponseWriter, requestID string, err error) bool {
	if !errors.Is(err, moduledeals.ErrWorkflowActionBlocked) {
		return false
	}
	platformweb.WriteError(
		w,
		http.StatusConflict,
		requestID,
		"WORKFLOW_ACTION_BLOCKED",
		"The deal change was not saved because an active workflow action is not ready. Add a primary contact with an email, assign an active owner, and verify the target sequence.",
	)
	return true
}
