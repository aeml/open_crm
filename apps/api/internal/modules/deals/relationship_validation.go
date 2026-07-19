package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func requireDealRelationships(ctx context.Context, tx pgx.Tx, organizationID, companyID, contactID int64) error {
	var companyAllowed, contactAllowed bool
	if err := tx.QueryRow(ctx, `
		SELECT
			$2::bigint=0 OR EXISTS(
				SELECT 1 FROM companies
				WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL
			),
			$3::bigint=0 OR EXISTS(
				SELECT 1 FROM contacts
				WHERE organization_id=$1 AND id=$3 AND archived_at IS NULL
			)
	`, organizationID, companyID, contactID).Scan(&companyAllowed, &contactAllowed); err != nil {
		return fmt.Errorf("validate deal relationships: %w", err)
	}
	if !companyAllowed || !contactAllowed {
		return ErrNotFound
	}
	return nil
}
