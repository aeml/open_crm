package telemetry

import (
	"fmt"
	"strings"
	"time"
)

type emailTrackingRetentionSnapshot struct {
	Runs      map[string]uint64
	Purged    uint64
	LastRunAt time.Time
	LastRunOK bool
}

func (c *Collector) ObserveEmailTrackingRetention(outcome string, messagesPurged int64) {
	if c == nil {
		return
	}
	outcome = finiteOutcome(outcome)
	c.mu.Lock()
	c.emailTrackingRetentionRuns[outcome]++
	c.emailTrackingRetentionLastAt = time.Now().UTC()
	c.emailTrackingRetentionLastOK = outcome == "success"
	if messagesPurged > 0 {
		c.emailTrackingRetentionPurged += uint64(messagesPurged)
	}
	c.mu.Unlock()
}

func writeEmailTrackingRetentionMetrics(output *strings.Builder, retention emailTrackingRetentionSnapshot) {
	writeHelpType(output, "open_crm_email_tracking_retention_runs_total", "Email engagement-tracking retention passes by finite outcome.", "counter")
	for _, outcome := range []string{"success", "error"} {
		fmt.Fprintf(output, "open_crm_email_tracking_retention_runs_total{outcome=%s} %d\n", quote(outcome), retention.Runs[outcome])
	}
	writeHelpType(output, "open_crm_email_tracking_retention_purged_total", "Expired email messages whose engagement observations were scrubbed.", "counter")
	fmt.Fprintf(output, "open_crm_email_tracking_retention_purged_total %d\n", retention.Purged)
	writeHelpType(output, "open_crm_email_tracking_retention_last_run_timestamp_seconds", "Unix timestamp of the last email engagement-tracking retention pass in this API process.", "gauge")
	fmt.Fprintf(output, "open_crm_email_tracking_retention_last_run_timestamp_seconds %d\n", unixOrZero(retention.LastRunAt))
	writeHelpType(output, "open_crm_email_tracking_retention_last_run_success", "Whether the last email engagement-tracking retention pass in this API process succeeded.", "gauge")
	writeBool(output, "open_crm_email_tracking_retention_last_run_success", retention.LastRunOK)
}
