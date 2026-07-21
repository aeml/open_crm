package telemetry

import (
	"fmt"
	"strings"
	"time"
)

type quoteDeliveryRecoverySnapshot struct {
	Runs      map[string]uint64
	Recovered uint64
	LastRunAt time.Time
	LastRunOK bool
}

func (c *Collector) ObserveQuoteDeliveryRecovery(outcome string, markedUncertain int64) {
	if c == nil {
		return
	}
	outcome = finiteOutcome(outcome)
	c.mu.Lock()
	c.quoteDeliveryRecoveryRuns[outcome]++
	c.quoteDeliveryRecoveryLastAt = time.Now().UTC()
	c.quoteDeliveryRecoveryLastOK = outcome == "success"
	if markedUncertain > 0 {
		c.quoteDeliveryRecoveryRecovered += uint64(markedUncertain)
	}
	c.mu.Unlock()
}

func writeQuoteDeliveryMetrics(output *strings.Builder, snapshot RuntimeSnapshot, recovery quoteDeliveryRecoverySnapshot) {
	writeHelpType(output, "open_crm_quote_deliveries_available", "Whether durable quote-delivery health was collected successfully.", "gauge")
	writeBool(output, "open_crm_quote_deliveries_available", snapshot.QuoteDeliveriesAvailable)
	writeHelpType(output, "open_crm_quote_delivery_sending", "Current quote deliveries with an active connected-mailbox provider claim.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_delivery_sending %d\n", nonNegative64(snapshot.QuoteDeliveriesSending))
	writeHelpType(output, "open_crm_quote_delivery_stale_sending", "Quote provider claims older than the safe recovery threshold.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_delivery_stale_sending %d\n", nonNegative64(snapshot.QuoteDeliveriesStaleSending))
	writeHelpType(output, "open_crm_quote_delivery_uncertain", "Quote deliveries awaiting explicit operator resolution.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_delivery_uncertain %d\n", nonNegative64(snapshot.QuoteDeliveriesUncertain))
	writeHelpType(output, "open_crm_quote_approval_pending", "Immutable quote PDFs awaiting an independent owner or administrator decision.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_approval_pending %d\n", nonNegative64(snapshot.QuoteApprovalsPending))
	writeHelpType(output, "open_crm_quote_approval_approved", "Immutable quote PDFs with retained independent approval evidence.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_approval_approved %d\n", nonNegative64(snapshot.QuoteApprovalsApproved))
	writeHelpType(output, "open_crm_quote_approval_rejected", "Immutable quote PDFs with retained rejection evidence.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_approval_rejected %d\n", nonNegative64(snapshot.QuoteApprovalsRejected))
	writeHelpType(output, "open_crm_quote_approval_oldest_pending_age_seconds", "Age in seconds of the oldest immutable quote PDF awaiting independent approval.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_approval_oldest_pending_age_seconds %s\n", durationValue(snapshot.QuoteOldestApprovalPendingAge))
	writeHelpType(output, "open_crm_quote_signature_awaiting_response", "Sent native quote-signature requests whose recipient link and quote deadline remain valid.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_awaiting_response %d\n", nonNegative64(snapshot.QuoteSignaturesAwaiting))
	writeHelpType(output, "open_crm_quote_signature_expired", "Sent native quote-signature requests that can no longer be completed because the quote or recipient link expired.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_expired %d\n", nonNegative64(snapshot.QuoteSignaturesExpired))
	writeHelpType(output, "open_crm_quote_signature_signed", "Native quote-signature requests with retained signed evidence and certificates.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_signed %d\n", nonNegative64(snapshot.QuoteSignaturesSigned))
	writeHelpType(output, "open_crm_quote_signature_awaiting_conversion", "Native signed quote requests on open deals not yet bound to a deliberate won-deal outcome.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_awaiting_conversion %d\n", nonNegative64(snapshot.QuoteSignaturesPending))
	writeHelpType(output, "open_crm_quote_signature_oldest_awaiting_conversion_age_seconds", "Age in seconds of the oldest native signed quote on an open deal awaiting deliberate won-deal conversion.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_oldest_awaiting_conversion_age_seconds %s\n", durationValue(snapshot.QuoteOldestPendingAge))
	writeHelpType(output, "open_crm_quote_signature_converted", "Native signed quote requests transactionally bound to a won-deal outcome.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_converted %d\n", nonNegative64(snapshot.QuoteSignaturesConverted))
	writeHelpType(output, "open_crm_quote_signature_declined", "Native quote-signature requests declined by recipients.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_declined %d\n", nonNegative64(snapshot.QuoteSignaturesDeclined))
	writeHelpType(output, "open_crm_quote_signature_voided", "Native quote-signature requests voided before completion.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_signature_voided %d\n", nonNegative64(snapshot.QuoteSignaturesVoided))
	writeHelpType(output, "open_crm_quote_delivery_recovery_runs_total", "Stale quote-delivery recovery passes by finite outcome.", "counter")
	for _, outcome := range []string{"success", "error"} {
		fmt.Fprintf(output, "open_crm_quote_delivery_recovery_runs_total{outcome=%s} %d\n", quote(outcome), recovery.Runs[outcome])
	}
	writeHelpType(output, "open_crm_quote_delivery_recovered_total", "Interrupted quote sends moved to explicit uncertain state.", "counter")
	fmt.Fprintf(output, "open_crm_quote_delivery_recovered_total %d\n", recovery.Recovered)
	writeHelpType(output, "open_crm_quote_delivery_recovery_last_run_timestamp_seconds", "Unix timestamp of the last quote-delivery recovery pass in this API process.", "gauge")
	fmt.Fprintf(output, "open_crm_quote_delivery_recovery_last_run_timestamp_seconds %d\n", unixOrZero(recovery.LastRunAt))
	writeHelpType(output, "open_crm_quote_delivery_recovery_last_run_success", "Whether the last quote-delivery recovery pass in this API process succeeded.", "gauge")
	writeBool(output, "open_crm_quote_delivery_recovery_last_run_success", recovery.LastRunOK)
}
