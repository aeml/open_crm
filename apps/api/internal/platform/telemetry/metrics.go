// Package telemetry exposes bounded, dependency-free Prometheus metrics for
// the Open CRM modular monolith. Labels are limited to static route patterns,
// internal provider/job names, and finite outcomes to avoid tenant or record
// cardinality and accidental customer-data exposure.
package telemetry

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var requestDurationBuckets = [...]float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}

type requestKey struct {
	method string
	route  string
	status int
}

type durationKey struct {
	method string
	route  string
}

type durationSeries struct {
	count   uint64
	sum     float64
	buckets [len(requestDurationBuckets)]uint64
}

type providerKey struct {
	provider  string
	operation string
	outcome   string
}

type jobKey struct {
	jobType string
	outcome string
}

type rateLimitKey struct {
	scope   string
	outcome string
}

// Collector stores process-local counters. Durable queue gauges and backup
// timestamps are supplied at scrape time through RuntimeSnapshot.
type Collector struct {
	mu              sync.RWMutex
	startedAt       time.Time
	requests        map[requestKey]uint64
	durations       map[durationKey]*durationSeries
	providerCalls   map[providerKey]uint64
	providerSeconds map[providerKey]float64
	jobOutcomes     map[jobKey]uint64
	rateLimits      map[rateLimitKey]uint64
	retentionRuns   map[string]uint64
	retentionRows   map[string]uint64
	retentionLastAt time.Time
	retentionLastOK bool
}

func NewCollector() *Collector {
	return &Collector{
		startedAt:       time.Now(),
		requests:        make(map[requestKey]uint64),
		durations:       make(map[durationKey]*durationSeries),
		providerCalls:   make(map[providerKey]uint64),
		providerSeconds: make(map[providerKey]float64),
		jobOutcomes:     make(map[jobKey]uint64),
		rateLimits:      make(map[rateLimitKey]uint64),
		retentionRuns:   make(map[string]uint64),
		retentionRows:   make(map[string]uint64),
	}
}

func (c *Collector) ObserveHTTPRequest(method, route string, status int, duration time.Duration) {
	if c == nil {
		return
	}
	method = finiteMethod(method)
	route = boundedLabel(normalizeRoute(method, route), "unmatched")
	if status < 100 || status > 599 {
		status = 0
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}

	c.mu.Lock()
	c.requests[requestKey{method: method, route: route, status: status}]++
	key := durationKey{method: method, route: route}
	series := c.durations[key]
	if series == nil {
		series = &durationSeries{}
		c.durations[key] = series
	}
	series.count++
	series.sum += seconds
	for index, upperBound := range requestDurationBuckets {
		if seconds <= upperBound {
			series.buckets[index]++
		}
	}
	c.mu.Unlock()
}

func (c *Collector) ObserveProvider(provider, operation, outcome string, duration time.Duration) {
	if c == nil {
		return
	}
	key := providerKey{
		provider:  boundedLabel(provider, "unknown"),
		operation: boundedLabel(operation, "unknown"),
		outcome:   finiteOutcome(outcome),
	}
	seconds := duration.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	c.mu.Lock()
	c.providerCalls[key]++
	c.providerSeconds[key] += seconds
	c.mu.Unlock()
}

func (c *Collector) ObserveJob(jobType, outcome string) {
	if c == nil {
		return
	}
	key := jobKey{jobType: boundedLabel(jobType, "unknown"), outcome: finiteJobOutcome(outcome)}
	c.mu.Lock()
	c.jobOutcomes[key]++
	c.mu.Unlock()
}

func (c *Collector) ObserveRateLimit(scope, outcome string) {
	if c == nil {
		return
	}
	key := rateLimitKey{
		scope:   boundedLabel(scope, "unknown"),
		outcome: finiteRateLimitOutcome(outcome),
	}
	c.mu.Lock()
	c.rateLimits[key]++
	c.mu.Unlock()
}

type RuntimeSnapshot struct {
	CollectionSuccess              bool
	DatabaseUp                     bool
	JobsAvailable                  bool
	JobsPending                    int
	JobsRunning                    int
	JobsRetryable                  int
	JobsDead                       int
	OldestReadyLag                 time.Duration
	NotificationsAvailable         bool
	NotificationsUnread            int64
	NotificationsCreated24h        int64
	NotificationRecipients24h      int64
	NotificationMaxPerRecipient24h int64
	OldestUnreadAge                time.Duration
	NotificationEvents24h          map[string]int64
	Backup                         BackupStatus
}

type SnapshotSource func(context.Context) RuntimeSnapshot

// Handler returns a bearer-token-protected Prometheus endpoint. An empty token
// deliberately hides the endpoint so metrics cannot be exposed accidentally.
func (c *Collector) Handler(token string, source SnapshotSource) http.Handler {
	token = strings.TrimSpace(token)
	if len(token) < 32 {
		token = ""
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.NotFound(w, r)
			return
		}
		provided := bearerToken(r.Header.Get("Authorization"))
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="open-crm-metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		snapshot := RuntimeSnapshot{}
		if source != nil {
			snapshot = source(r.Context())
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(c.render(snapshot)))
	})
}

func (c *Collector) render(snapshot RuntimeSnapshot) string {
	if c == nil {
		c = NewCollector()
	}
	c.mu.RLock()
	requests := copyMap(c.requests)
	durations := copyDurations(c.durations)
	providerCalls := copyMap(c.providerCalls)
	providerSeconds := copyMap(c.providerSeconds)
	jobOutcomes := copyMap(c.jobOutcomes)
	rateLimits := copyMap(c.rateLimits)
	retentionRuns := copyMap(c.retentionRuns)
	retentionRows := copyMap(c.retentionRows)
	retentionLastAt := c.retentionLastAt
	retentionLastOK := c.retentionLastOK
	startedAt := c.startedAt
	c.mu.RUnlock()

	var output strings.Builder
	writeHelpType(&output, "open_crm_process_start_time_seconds", "Unix timestamp when this API process started.", "gauge")
	fmt.Fprintf(&output, "open_crm_process_start_time_seconds %d\n", startedAt.Unix())
	writeHelpType(&output, "open_crm_metrics_collection_success", "Whether all scrape-time operational sources were collected successfully.", "gauge")
	writeBool(&output, "open_crm_metrics_collection_success", snapshot.CollectionSuccess)
	writeHelpType(&output, "open_crm_database_up", "Whether PostgreSQL responded to the scrape-time readiness query.", "gauge")
	writeBool(&output, "open_crm_database_up", snapshot.DatabaseUp)

	writeHelpType(&output, "open_crm_http_requests_total", "HTTP requests completed by method, bounded route pattern, and status.", "counter")
	requestKeys := sortedKeys(requests, func(key requestKey) string {
		return fmt.Sprintf("%s\x00%s\x00%03d", key.method, key.route, key.status)
	})
	for _, key := range requestKeys {
		fmt.Fprintf(&output, "open_crm_http_requests_total{method=%s,route=%s,status=%s} %d\n", quote(key.method), quote(key.route), quote(strconv.Itoa(key.status)), requests[key])
	}

	writeHelpType(&output, "open_crm_http_request_duration_seconds", "HTTP request duration by method and bounded route pattern.", "histogram")
	durationKeys := sortedKeys(durations, func(key durationKey) string { return key.method + "\x00" + key.route })
	for _, key := range durationKeys {
		series := durations[key]
		for index, upperBound := range requestDurationBuckets {
			fmt.Fprintf(&output, "open_crm_http_request_duration_seconds_bucket{method=%s,route=%s,le=%s} %d\n", quote(key.method), quote(key.route), quote(strconv.FormatFloat(upperBound, 'g', -1, 64)), series.buckets[index])
		}
		fmt.Fprintf(&output, "open_crm_http_request_duration_seconds_bucket{method=%s,route=%s,le=\"+Inf\"} %d\n", quote(key.method), quote(key.route), series.count)
		fmt.Fprintf(&output, "open_crm_http_request_duration_seconds_sum{method=%s,route=%s} %s\n", quote(key.method), quote(key.route), strconv.FormatFloat(series.sum, 'g', -1, 64))
		fmt.Fprintf(&output, "open_crm_http_request_duration_seconds_count{method=%s,route=%s} %d\n", quote(key.method), quote(key.route), series.count)
	}

	writeHelpType(&output, "open_crm_provider_operations_total", "External or fake provider operations by bounded provider, operation, and outcome.", "counter")
	writeHelpType(&output, "open_crm_provider_operation_duration_seconds_total", "Cumulative provider-operation duration by bounded provider, operation, and outcome.", "counter")
	providerKeys := sortedKeys(providerCalls, func(key providerKey) string { return key.provider + "\x00" + key.operation + "\x00" + key.outcome })
	for _, key := range providerKeys {
		labels := fmt.Sprintf("provider=%s,operation=%s,outcome=%s", quote(key.provider), quote(key.operation), quote(key.outcome))
		fmt.Fprintf(&output, "open_crm_provider_operations_total{%s} %d\n", labels, providerCalls[key])
		fmt.Fprintf(&output, "open_crm_provider_operation_duration_seconds_total{%s} %s\n", labels, strconv.FormatFloat(providerSeconds[key], 'g', -1, 64))
	}

	writeHelpType(&output, "open_crm_background_job_outcomes_total", "Background job terminal/retry outcomes observed by this process.", "counter")
	jobKeys := sortedKeys(jobOutcomes, func(key jobKey) string { return key.jobType + "\x00" + key.outcome })
	for _, key := range jobKeys {
		fmt.Fprintf(&output, "open_crm_background_job_outcomes_total{job_type=%s,outcome=%s} %d\n", quote(key.jobType), quote(key.outcome), jobOutcomes[key])
	}

	writeHelpType(&output, "open_crm_rate_limit_decisions_total", "Public abuse-control decisions by bounded static scope and outcome.", "counter")
	rateLimitKeys := sortedKeys(rateLimits, func(key rateLimitKey) string { return key.scope + "\x00" + key.outcome })
	for _, key := range rateLimitKeys {
		fmt.Fprintf(&output, "open_crm_rate_limit_decisions_total{scope=%s,outcome=%s} %d\n", quote(key.scope), quote(key.outcome), rateLimits[key])
	}

	writeHelpType(&output, "open_crm_background_jobs_available", "Whether durable queue gauges were collected successfully.", "gauge")
	writeBool(&output, "open_crm_background_jobs_available", snapshot.JobsAvailable)
	writeHelpType(&output, "open_crm_background_jobs", "Current durable jobs by status across all tenants; no tenant labels are exposed.", "gauge")
	for _, entry := range []struct {
		status string
		value  int
	}{{"pending", snapshot.JobsPending}, {"running", snapshot.JobsRunning}, {"retryable", snapshot.JobsRetryable}, {"dead", snapshot.JobsDead}} {
		fmt.Fprintf(&output, "open_crm_background_jobs{status=%s} %d\n", quote(entry.status), nonNegative(entry.value))
	}
	writeHelpType(&output, "open_crm_background_job_oldest_ready_lag_seconds", "Age of the oldest runnable pending or retryable job.", "gauge")
	fmt.Fprintf(&output, "open_crm_background_job_oldest_ready_lag_seconds %s\n", durationValue(snapshot.OldestReadyLag))

	writeNotificationMetrics(&output, snapshot, notificationRetentionSnapshot{
		Runs:      retentionRuns,
		Rows:      retentionRows,
		LastRunAt: retentionLastAt,
		LastRunOK: retentionLastOK,
	})
	writeBackupMetrics(&output, snapshot.Backup)
	return output.String()
}

func normalizeRoute(method, route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "unmatched"
	}
	if prefix := method + " "; strings.HasPrefix(route, prefix) {
		route = strings.TrimSpace(strings.TrimPrefix(route, prefix))
	}
	return route
}

func boundedLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 200 {
		return value[:200]
	}
	return value
}

func finiteOutcome(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "success") {
		return "success"
	}
	return "error"
}

func finiteMethod(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return "OTHER"
	}
}

func finiteJobOutcome(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "deferred", "retryable", "dead", "cycle_error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func finiteRateLimitOutcome(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allowed", "rejected", "error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "error"
	}
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func quote(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return `"` + value + `"`
}

func writeHelpType(output *strings.Builder, name, help, metricType string) {
	fmt.Fprintf(output, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, metricType)
}

func writeBool(output *strings.Builder, name string, value bool) {
	if value {
		fmt.Fprintf(output, "%s 1\n", name)
		return
	}
	fmt.Fprintf(output, "%s 0\n", name)
}

func writeBackupMetrics(output *strings.Builder, status BackupStatus) {
	writeHelpType(output, "open_crm_backup_status_available", "Whether backup status evidence was readable.", "gauge")
	writeBool(output, "open_crm_backup_status_available", status.Available)
	writeHelpType(output, "open_crm_backup_last_success_timestamp_seconds", "Completion timestamp of the last verified encrypted backup.", "gauge")
	fmt.Fprintf(output, "open_crm_backup_last_success_timestamp_seconds %d\n", unixOrZero(status.LastSuccessAt))
	writeHelpType(output, "open_crm_backup_last_attempt_success", "Whether the latest recorded backup attempt succeeded.", "gauge")
	writeBool(output, "open_crm_backup_last_attempt_success", status.LastAttemptSucceeded)
	writeHelpType(output, "open_crm_restore_drill_last_success_timestamp_seconds", "Completion timestamp of the last verified isolated restore drill.", "gauge")
	fmt.Fprintf(output, "open_crm_restore_drill_last_success_timestamp_seconds %d\n", unixOrZero(status.LastRestoreSuccessAt))
	writeHelpType(output, "open_crm_restore_drill_last_attempt_success", "Whether the latest recorded restore-drill attempt succeeded.", "gauge")
	writeBool(output, "open_crm_restore_drill_last_attempt_success", status.LastRestoreAttemptSucceeded)
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func durationValue(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	return strconv.FormatFloat(value.Seconds(), 'g', -1, 64)
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func copyMap[K comparable, V any](source map[K]V) map[K]V {
	result := make(map[K]V, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func copyDurations(source map[durationKey]*durationSeries) map[durationKey]durationSeries {
	result := make(map[durationKey]durationSeries, len(source))
	for key, value := range source {
		if value != nil {
			result[key] = *value
		}
	}
	return result
}

func sortedKeys[K comparable, V any](values map[K]V, key func(K) string) []K {
	result := make([]K, 0, len(values))
	for item := range values {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return key(result[i]) < key(result[j]) })
	return result
}
