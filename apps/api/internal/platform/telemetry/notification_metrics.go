package telemetry

import (
	"fmt"
	"strings"
	"time"
)

var notificationEventTypes = [...]string{
	"deal.assigned",
	"meeting.reminder",
	"record.activity",
	"record.mentioned",
	"task.assigned",
	"task.due_soon",
	"task.overdue",
	"other",
}

type notificationRetentionSnapshot struct {
	Runs      map[string]uint64
	Rows      map[string]uint64
	LastRunAt time.Time
	LastRunOK bool
}

func (c *Collector) ObserveNotificationRetention(outcome string, readDeleted, unreadDeleted int64) {
	if c == nil {
		return
	}
	outcome = finiteOutcome(outcome)
	c.mu.Lock()
	c.retentionRuns[outcome]++
	c.retentionLastAt = time.Now().UTC()
	c.retentionLastOK = outcome == "success"
	if readDeleted > 0 {
		c.retentionRows["read"] += uint64(readDeleted)
	}
	if unreadDeleted > 0 {
		c.retentionRows["unread"] += uint64(unreadDeleted)
	}
	c.mu.Unlock()
}

func writeNotificationMetrics(output *strings.Builder, snapshot RuntimeSnapshot, retention notificationRetentionSnapshot) {
	writeHelpType(output, "open_crm_notifications_available", "Whether aggregate notification health was collected successfully.", "gauge")
	writeBool(output, "open_crm_notifications_available", snapshot.NotificationsAvailable)
	writeHelpType(output, "open_crm_notifications_unread", "Unread notifications across all tenants; no tenant or recipient labels are exposed.", "gauge")
	fmt.Fprintf(output, "open_crm_notifications_unread %d\n", nonNegative64(snapshot.NotificationsUnread))
	writeHelpType(output, "open_crm_notifications_created_24h", "Notifications created in the trailing 24 hours across all tenants.", "gauge")
	fmt.Fprintf(output, "open_crm_notifications_created_24h %d\n", nonNegative64(snapshot.NotificationsCreated24h))
	writeHelpType(output, "open_crm_notification_recipients_24h", "Distinct organization-recipient pairs receiving a notification in the trailing 24 hours.", "gauge")
	fmt.Fprintf(output, "open_crm_notification_recipients_24h %d\n", nonNegative64(snapshot.NotificationRecipients24h))
	writeHelpType(output, "open_crm_notification_max_per_recipient_24h", "Largest trailing-24-hour notification count for one organization-recipient pair; recipient identity is not exposed.", "gauge")
	fmt.Fprintf(output, "open_crm_notification_max_per_recipient_24h %d\n", nonNegative64(snapshot.NotificationMaxPerRecipient24h))
	writeHelpType(output, "open_crm_notification_oldest_unread_age_seconds", "Age of the oldest unread notification across all tenants.", "gauge")
	fmt.Fprintf(output, "open_crm_notification_oldest_unread_age_seconds %s\n", durationValue(snapshot.OldestUnreadAge))
	writeHelpType(output, "open_crm_notification_events_24h", "Trailing-24-hour notification volume by reviewed finite event type.", "gauge")
	for _, eventType := range notificationEventTypes {
		fmt.Fprintf(output, "open_crm_notification_events_24h{event_type=%s} %d\n", quote(eventType), nonNegative64(notificationEventCount(snapshot.NotificationEvents24h, eventType)))
	}

	writeHelpType(output, "open_crm_notification_retention_runs_total", "Notification retention passes by finite outcome.", "counter")
	for _, outcome := range []string{"success", "error"} {
		fmt.Fprintf(output, "open_crm_notification_retention_runs_total{outcome=%s} %d\n", quote(outcome), retention.Runs[outcome])
	}
	writeHelpType(output, "open_crm_notification_retention_deleted_total", "Notifications removed by acknowledged state.", "counter")
	for _, state := range []string{"read", "unread"} {
		fmt.Fprintf(output, "open_crm_notification_retention_deleted_total{state=%s} %d\n", quote(state), retention.Rows[state])
	}
	writeHelpType(output, "open_crm_notification_retention_last_run_timestamp_seconds", "Unix timestamp of the last notification retention pass in this API process.", "gauge")
	fmt.Fprintf(output, "open_crm_notification_retention_last_run_timestamp_seconds %d\n", unixOrZero(retention.LastRunAt))
	writeHelpType(output, "open_crm_notification_retention_last_run_success", "Whether the last notification retention pass in this API process succeeded.", "gauge")
	writeBool(output, "open_crm_notification_retention_last_run_success", retention.LastRunOK)
}

func notificationEventCount(events map[string]int64, eventType string) int64 {
	if events == nil {
		return 0
	}
	if eventType != "other" {
		return events[eventType]
	}
	count := events["other"]
	for key, value := range events {
		if key != "other" && !knownNotificationEvent(key) {
			count += value
		}
	}
	return count
}

func knownNotificationEvent(value string) bool {
	for _, eventType := range notificationEventTypes[:len(notificationEventTypes)-1] {
		if value == eventType {
			return true
		}
	}
	return false
}

func nonNegative64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
