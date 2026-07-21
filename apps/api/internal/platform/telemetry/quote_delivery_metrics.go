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
