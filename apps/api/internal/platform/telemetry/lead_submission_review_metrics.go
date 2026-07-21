package telemetry

import (
	"fmt"
	"strings"
)

func writeLeadSubmissionReviewMetrics(output *strings.Builder, snapshot RuntimeSnapshot) {
	writeHelpType(output, "open_crm_lead_reviews_available", "Whether aggregate lead-submission review health was collected successfully.", "gauge")
	writeBool(output, "open_crm_lead_reviews_available", snapshot.LeadReviewsAvailable)
	writeHelpType(output, "open_crm_lead_reviews", "Current lead submissions by bounded review state across all tenants; no tenant or contact labels are exposed.", "gauge")
	for _, entry := range []struct {
		state string
		value int64
	}{{"unreviewed", snapshot.LeadReviewsUnreviewed}, {"legitimate", snapshot.LeadReviewsLegitimate}, {"spam", snapshot.LeadReviewsSpam}} {
		fmt.Fprintf(output, "open_crm_lead_reviews{state=%s} %d\n", quote(entry.state), nonNegative64(entry.value))
	}
	writeHelpType(output, "open_crm_lead_review_oldest_unreviewed_age_seconds", "Age of the oldest lead submission still awaiting operator review across all tenants.", "gauge")
	fmt.Fprintf(output, "open_crm_lead_review_oldest_unreviewed_age_seconds %s\n", durationValue(snapshot.LeadReviewOldestUnreviewedAge))
}
