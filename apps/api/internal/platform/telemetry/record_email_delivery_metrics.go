package telemetry

import (
	"fmt"
	"strings"
	"time"
)

type recordEmailDeliveryRecoverySnapshot struct {
	Runs      map[string]uint64
	Recovered uint64
	LastRunAt time.Time
	LastRunOK bool
}

func (c *Collector) ObserveRecordEmailDeliveryRecovery(outcome string, markedUncertain int64) {
	if c == nil {
		return
	}
	outcome = finiteOutcome(outcome)
	c.mu.Lock()
	c.recordEmailDeliveryRecoveryRuns[outcome]++
	c.recordEmailDeliveryRecoveryLastAt = time.Now().UTC()
	c.recordEmailDeliveryRecoveryLastOK = outcome == "success"
	if markedUncertain > 0 {
		c.recordEmailDeliveryRecoveryRecovered += uint64(markedUncertain)
	}
	c.mu.Unlock()
}

func writeRecordEmailDeliveryMetrics(output *strings.Builder, snapshot RuntimeSnapshot, recovery recordEmailDeliveryRecoverySnapshot) {
	writeHelpType(output, "open_crm_record_email_deliveries_available", "Whether durable one-to-one record-email delivery health was collected successfully.", "gauge")
	writeBool(output, "open_crm_record_email_deliveries_available", snapshot.RecordEmailDeliveriesAvailable)
	writeHelpType(output, "open_crm_record_email_delivery_sending", "Current one-to-one record emails with an active provider send claim.", "gauge")
	fmt.Fprintf(output, "open_crm_record_email_delivery_sending %d\n", nonNegative64(snapshot.RecordEmailDeliveriesSending))
	writeHelpType(output, "open_crm_record_email_delivery_stale_sending", "One-to-one record-email provider claims older than the safe recovery threshold.", "gauge")
	fmt.Fprintf(output, "open_crm_record_email_delivery_stale_sending %d\n", nonNegative64(snapshot.RecordEmailDeliveriesStaleSending))
	writeHelpType(output, "open_crm_record_email_delivery_uncertain", "One-to-one record emails awaiting explicit operator resolution.", "gauge")
	fmt.Fprintf(output, "open_crm_record_email_delivery_uncertain %d\n", nonNegative64(snapshot.RecordEmailDeliveriesUncertain))
	writeHelpType(output, "open_crm_record_email_delivery_recovery_runs_total", "Stale record-email recovery passes by finite outcome.", "counter")
	for _, outcome := range []string{"success", "error"} {
		fmt.Fprintf(output, "open_crm_record_email_delivery_recovery_runs_total{outcome=%s} %d\n", quote(outcome), recovery.Runs[outcome])
	}
	writeHelpType(output, "open_crm_record_email_delivery_recovered_total", "Interrupted record-email sends moved to explicit uncertain state.", "counter")
	fmt.Fprintf(output, "open_crm_record_email_delivery_recovered_total %d\n", recovery.Recovered)
	writeHelpType(output, "open_crm_record_email_delivery_recovery_last_run_timestamp_seconds", "Unix timestamp of the last record-email recovery pass in this API process.", "gauge")
	fmt.Fprintf(output, "open_crm_record_email_delivery_recovery_last_run_timestamp_seconds %d\n", unixOrZero(recovery.LastRunAt))
	writeHelpType(output, "open_crm_record_email_delivery_recovery_last_run_success", "Whether the last record-email recovery pass in this API process succeeded.", "gauge")
	writeBool(output, "open_crm_record_email_delivery_recovery_last_run_success", recovery.LastRunOK)
}
