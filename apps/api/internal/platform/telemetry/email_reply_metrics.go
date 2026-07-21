package telemetry

import (
	"fmt"
	"strings"
	"time"
)

type emailReplyRecoverySnapshot struct {
	Runs      map[string]uint64
	Recovered uint64
	LastRunAt time.Time
	LastRunOK bool
}

func (c *Collector) ObserveEmailReplyRecovery(outcome string, markedUncertain int64) {
	if c == nil {
		return
	}
	outcome = finiteOutcome(outcome)
	c.mu.Lock()
	c.emailReplyRecoveryRuns[outcome]++
	c.emailReplyRecoveryLastAt = time.Now().UTC()
	c.emailReplyRecoveryLastOK = outcome == "success"
	if markedUncertain > 0 {
		c.emailReplyRecoveryRecovered += uint64(markedUncertain)
	}
	c.mu.Unlock()
}

func writeEmailReplyMetrics(output *strings.Builder, snapshot RuntimeSnapshot, recovery emailReplyRecoverySnapshot) {
	writeHelpType(output, "open_crm_email_replies_available", "Whether durable email-reply health was collected successfully.", "gauge")
	writeBool(output, "open_crm_email_replies_available", snapshot.EmailRepliesAvailable)
	writeHelpType(output, "open_crm_email_reply_sending", "Current connected-mailbox replies with an active provider send claim.", "gauge")
	fmt.Fprintf(output, "open_crm_email_reply_sending %d\n", nonNegative64(snapshot.EmailRepliesSending))
	writeHelpType(output, "open_crm_email_reply_stale_sending", "Provider send claims older than the safe recovery threshold.", "gauge")
	fmt.Fprintf(output, "open_crm_email_reply_stale_sending %d\n", nonNegative64(snapshot.EmailRepliesStaleSending))
	writeHelpType(output, "open_crm_email_reply_uncertain", "Connected-mailbox replies awaiting explicit operator resolution.", "gauge")
	fmt.Fprintf(output, "open_crm_email_reply_uncertain %d\n", nonNegative64(snapshot.EmailRepliesUncertain))
	writeHelpType(output, "open_crm_email_reply_recovery_runs_total", "Stale email-reply recovery passes by finite outcome.", "counter")
	for _, outcome := range []string{"success", "error"} {
		fmt.Fprintf(output, "open_crm_email_reply_recovery_runs_total{outcome=%s} %d\n", quote(outcome), recovery.Runs[outcome])
	}
	writeHelpType(output, "open_crm_email_reply_recovered_total", "Interrupted email-reply sends moved to explicit uncertain state.", "counter")
	fmt.Fprintf(output, "open_crm_email_reply_recovered_total %d\n", recovery.Recovered)
	writeHelpType(output, "open_crm_email_reply_recovery_last_run_timestamp_seconds", "Unix timestamp of the last reply-recovery pass in this API process.", "gauge")
	fmt.Fprintf(output, "open_crm_email_reply_recovery_last_run_timestamp_seconds %d\n", unixOrZero(recovery.LastRunAt))
	writeHelpType(output, "open_crm_email_reply_recovery_last_run_success", "Whether the last reply-recovery pass in this API process succeeded.", "gauge")
	writeBool(output, "open_crm_email_reply_recovery_last_run_success", recovery.LastRunOK)
}
