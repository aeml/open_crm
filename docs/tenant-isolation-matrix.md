# Tenant Isolation Evidence Matrix

Last reconciled: 2026-07-21

Evidence row count: `25`

Evidence digest: `f6e61354ff37aeb92019a0a54315c45e61858eab024e0cb90d6520a862649a93`

This is the executable Phase 2 negative-path matrix for capabilities promoted
into the pilot workflow. It complements, rather than replaces,
`security-surface-inventory.md`: that inventory binds every HTTP selector to
authentication, role, tenant, entitlement, abuse, observability, and test
policy, while this matrix identifies the freshly migrated PostgreSQL
acceptance test that proves each promoted service family keeps tenant data and
effects separate.

`apps/api/internal/app/tenant_isolation_evidence_test.go` owns the exact sorted
evidence set. The test fails if a referenced test disappears, stops being a
real-PostgreSQL test, or diverges from the count, digest, or rows below. A green
matrix is evidence that the named tests exist and run in the database CI job;
the assertions inside those tests remain the proof of behavior.

## Promoted service evidence

| Surface | PostgreSQL source | Exact test | Negative boundary proved |
| --- | --- | --- | --- |
| `archive-recovery` | `apps/api/internal/modules/archiveoperations/service_postgres_test.go` | `TestArchiveRecoveryIsTenantSafeDependencyAwareAndAuditedAgainstPostgres` | Foreign record and operation IDs remain missing; dependency conflicts and recovery evidence stay in the owning workspace. |
| `audit-history` | `apps/api/internal/modules/audit/service_postgres_test.go` | `TestAuditRetentionExportAndTenantBoundaryAgainstPostgres` | Foreign events do not enter lists or CSV; rejected mutation and retention paths cannot rewrite another workspace's history. |
| `bulk-operations` | `apps/api/internal/modules/bulkoperations/service_postgres_test.go` | `TestBulkOperationsAreIdempotentTenantSafeAndChangeAwareAgainstPostgres` | Foreign records, assignees, operation history, and rollback IDs reject atomically. |
| `client-review-schedules` | `apps/api/internal/modules/clientreviews/service_postgres_test.go` | `TestClientReviewSchedulesOwnARecoverableTenantSafeTaskLifecycleAgainstPostgres` | Foreign clients and tasks cannot be scheduled, advanced, cleared, or recovered through another tenant. |
| `collaboration` | `apps/api/internal/modules/collaboration/service_postgres_test.go` | `TestFollowersMentionsNotificationsAndDigestAgainstPostgres` | Foreign records and actors cannot create follows, mentions, notifications, or digest entries. |
| `core-csv-export` | `apps/api/internal/performance/pilot_load_postgres_test.go` | `TestPilotReadLoadAndFailureBudgetsAgainstPostgres` | A foreign marker is absent from the maximum supported contact export, including under representative multi-tenant load. |
| `core-record-boundaries` | `apps/api/internal/app/core_tenant_isolation_postgres_test.go` | `TestCoreRecordTenantBoundariesAgainstPostgres` | Contacts, companies, deals, tasks, saved views, and notes reject foreign list/get/update/archive/delete, relationship, stage, entity, actor, and assignee paths without partial effects. |
| `custom-fields` | `apps/api/internal/modules/customfields/service_postgres_test.go` | `TestCustomFieldsEndToEndAgainstPostgres` | Definitions, values, filters, imports, exports, and archive operations remain tenant scoped; a foreign definition ID is hidden. |
| `custom-report-execution` | `apps/api/internal/modules/customreports/execution_postgres_test.go` | `TestSavedTableReportsExecuteTenantSafeTypedQueriesAgainstPostgres` | Definition mutations, contact/company/deal/task table and grouped-bar queries, and admin CSV exports bind the owning organization and active actor, exclude archived and foreign markers, reject cross-tenant definition IDs without disclosure, and leave no partial definition/download audit on forbidden or oversized paths. |
| `data-quality` | `apps/api/internal/modules/dataquality/service_postgres_test.go` | `TestDataQualityReportsAreExplainableBusinessAwareAndTenantSafeAgainstPostgres` | Fixed-quality queues and counts exclude every seeded foreign record. |
| `deal-assignments` | `apps/api/internal/modules/deals/assignment_notifications_postgres_test.go` | `TestDealAssignmentsAreTransactionalPreferenceAwareAndIdempotentAgainstPostgres` | A foreign owner is rejected before deal or notification effects; transactional notification failure also rolls back the assignment. |
| `deal-close-and-handoff` | `apps/api/internal/modules/deals/win_loss_postgres_test.go` | `TestDealCloseReviewsKeepOutcomeContextCoherentAndTenantScopedAgainstPostgres` | Foreign stages/accounts cannot alter close state or handoff evidence; replay and reopening preserve the owning tenant's history. |
| `deal-task-automation` | `apps/api/internal/modules/workflowautomations/deal_task_rules_postgres_test.go` | `TestDealTaskRulesExecuteTransactionallyIdempotentlyAndWithinTenant` | Foreign definitions do not execute for local deal events, and generated tasks/runs remain source-tenant bound. |
| `duplicate-management` | `apps/api/internal/modules/duplicateoperations/service_postgres_test.go` | `TestDuplicateReviewAndMergePreserveRelationshipsAgainstPostgres` | Foreign candidates remain invisible and a cross-tenant merge key/record pair rejects without relationship changes. |
| `forecast` | `apps/api/internal/modules/dashboard/forecast_postgres_test.go` | `TestForecastUsesConfiguredProbabilitiesDateRangeUnassignedDealsAndTenantScope` | Forecast totals, stages, owners, quotas, and task buckets exclude a seeded high-value foreign pipeline. |
| `imports-and-rollback` | `apps/api/internal/modules/imports/service_postgres_test.go` | `TestTrackedImportIdempotencyErrorsIsolationAndRollbackAgainstPostgres` | Imported rows, batch history, idempotency, and rollback are tenant scoped; foreign history and rollback IDs stay missing. |
| `invitations` | `apps/api/internal/modules/users/invitations_postgres_test.go` | `TestInvitationLifecycleRotatesExpiresRevokesAndCompletesAgainstPostgres` | Foreign delivery, resend, and revoke attempts return not found and cannot consume or rotate the owner's token lineage. |
| `pipeline-configuration` | `apps/api/internal/modules/deals/pipeline_configuration_postgres_test.go` | `TestPipelineConfigurationIsAuditedTenantSafeAndPreservesDealsAgainstPostgres` | Foreign pipelines and stages cannot be renamed, reordered, deleted, or assigned to local deals. |
| `sales-activity-reporting` | `apps/api/internal/modules/salesreports/service_postgres_test.go` | `TestSalesActivityReportingUsesDurableSnapshotsAndTenantSafeActorSemanticsAgainstPostgres` | Event snapshots and rollups exclude foreign deals; foreign owner filters reject rather than disclose. |
| `session-management` | `apps/api/internal/modules/auth/sessions_postgres_test.go` | `TestSessionManagementIsPrivateGlobalAndAuditedAgainstPostgres` | A user cannot list or revoke another user's session, including a session in a foreign workspace. |
| `task-reminders` | `apps/api/internal/modules/taskreminders/service_postgres_test.go` | `TestTaskRemindersAreDurablePreferenceAwareAndIdempotentAgainstPostgres` | A foreign tenant cannot consume another tenant's reminder job or receive its notification/activity effects. |
| `touchpoints` | `apps/api/internal/modules/touchpoints/service_postgres_test.go` | `TestTouchpointsAreTraceableTenantSafeAndViewerAwareAgainstPostgres` | Foreign contact/client IDs and activity remain absent from history, follow-up queues, client-period counts, source links, and health summaries; private sources remain viewer-aware. |
| `user-lifecycle` | `apps/api/internal/modules/users/lifecycle_postgres_test.go` | `TestUserLifecycleReassignsWorkInvalidatesAccessAndPreservesHistoryAgainstPostgres` | Foreign targets and reassignment users reject; membership, sessions, work, history, and audit stay in the owning workspace. |
| `workspace-bootstrap` | `apps/api/internal/modules/onboarding/service_postgres_test.go` | `TestVerifiedWorkspaceSignupIsIdempotentAndStartsTrialAfterVerificationAgainstPostgres` | Provisioning commits one isolated organization/owner/pipeline only after token verification; conflicting retry identity cannot create a second tenant. |
| `workspace-portability` | `apps/api/internal/modules/workspaceexports/service_postgres_test.go` | `TestWorkspaceExportLifecycleAgainstPostgres` | Dataset queries, job/artifact IDs, downloads, redaction, checksum evidence, and expiry remain tenant scoped. |

## Other enforced layers

- `apps/api/internal/app/cross_org_test.go` verifies that the core HTTP handlers
  translate service misses to non-disclosing `404` responses.
- `apps/api/internal/app/security_inventory_test.go` digest-gates all 247
  registered routes, so a new selector must receive an explicit session/token
  tenant policy and test reference.
- Role and viewer denial are handler concerns and remain covered by the route
  family permission tests named in `security-surface-inventory.md`; the
  PostgreSQL tests above additionally exercise foreign actors and assignees
  whenever their IDs cross the service boundary.
- `apps/web/e2e/critical_journey.spec.js` creates two real browser contexts and
  workspaces against PostgreSQL, then proves foreign contact, follower,
  touchpoint, quote, close, and portable-export requests return `404`; the
  other workspace's client-period collection remains `200` but contains none
  of the pilot workspace's clients, counts, or sources.
- Composite tenant foreign keys are used where stable relational tables permit
  them. Polymorphic record IDs are revalidated under the tenant predicate at
  the transactional service boundary.

## Promotion and review rule

1. A newly registered or renamed route must first pass the full security
   inventory review.
2. A capability may be called `production-capable` only when its normal
   navigation path has real-PostgreSQL negative evidence here, or the capability
   matrix explicitly keeps it hidden/incomplete.
3. Evidence must exercise every applicable class: tenant-scoped list/read,
   mutation/delete/recovery, caller-supplied related IDs, actor/assignee IDs,
   and no-partial-effect rollback. A handler fake alone is insufficient.
4. Update the exact test and this row in the same slice. Recompute the digest
   only after reviewing the behavior; never change it solely to make CI green.
5. Extend the browser isolation journey when a promoted public token flow or
   critical lead-to-client transition introduces a new externally reachable
   boundary.
