package deals

import (
	"errors"

	moduleworkflowautomations "github.com/aeml/open_crm/apps/api/internal/modules/workflowautomations"
)

// ErrWorkflowActionBlocked lets deal routes distinguish an operator-correctable
// active-rule prerequisite from an internal transaction failure. The source
// deal mutation still rolls back completely in both cases.
var ErrWorkflowActionBlocked = errors.New("deal workflow action blocked")

func mapDealWorkflowError(err error) error {
	if errors.Is(err, moduleworkflowautomations.ErrActionBlocked) {
		return errors.Join(ErrWorkflowActionBlocked, err)
	}
	return err
}
