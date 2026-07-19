package duplicateoperations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	moduleclientreviews "github.com/aeml/open_crm/apps/api/internal/modules/clientreviews"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
)

var selectableFields = map[string]map[string]struct{}{
	"contact": fieldSet("firstName", "lastName", "email", "phone", "jobTitle", "status", "ownerUserId", "addressLine1", "addressLine2", "city", "state", "postalCode", "country", "leadSource", "firstSourceUrl", "utmSource", "utmMedium", "utmCampaign", "utmTerm", "utmContent", "leadScore"),
	"company": fieldSet("name", "industry", "phone", "website", "status", "ownerUserId", "addressLine1", "addressLine2", "city", "state", "postalCode", "country"),
}

var customSourceFieldPattern = regexp.MustCompile(`^custom:[a-z][a-z0-9_]{1,39}$`)

func fieldSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func normalizeEntityType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "contact" && value != "company" {
		return ""
	}
	return value
}

func normalizeMergeInput(input MergeInput) (MergeInput, error) {
	input.EntityType = normalizeEntityType(input.EntityType)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.OrganizationID <= 0 || input.ActorUserID <= 0 || input.EntityType == "" || input.SourceEntityID <= 0 || input.TargetEntityID <= 0 || input.SourceEntityID == input.TargetEntityID {
		return MergeInput{}, fmt.Errorf("%w: organization, actor, entity type, and two distinct records are required", ErrInvalidInput)
	}
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || input.SourceUpdatedAt.IsZero() || input.TargetUpdatedAt.IsZero() {
		return MergeInput{}, fmt.Errorf("%w: a valid idempotency key and record versions are required", ErrInvalidInput)
	}
	allowed := selectableFields[input.EntityType]
	seen := map[string]struct{}{}
	fields := make([]string, 0, len(input.SourceFields))
	for _, field := range input.SourceFields {
		field = strings.TrimSpace(field)
		_, coreField := allowed[field]
		if !coreField && !customSourceFieldPattern.MatchString(field) {
			return MergeInput{}, fmt.Errorf("%w: unsupported source field %q", ErrInvalidInput, field)
		}
		if _, ok := seen[field]; !ok {
			seen[field] = struct{}{}
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	input.SourceFields = fields
	input.SourceUpdatedAt = input.SourceUpdatedAt.UTC()
	input.TargetUpdatedAt = input.TargetUpdatedAt.UTC()
	return input, nil
}

func mergeRequestDigest(input MergeInput) (string, error) {
	payload := struct {
		EntityType      string   `json:"entityType"`
		SourceEntityID  int64    `json:"sourceEntityId"`
		TargetEntityID  int64    `json:"targetEntityId"`
		SourceFields    []string `json:"sourceFields"`
		SourceUpdatedAt string   `json:"sourceUpdatedAt"`
		TargetUpdatedAt string   `json:"targetUpdatedAt"`
	}{input.EntityType, input.SourceEntityID, input.TargetEntityID, input.SourceFields, input.SourceUpdatedAt.Format(time.RFC3339Nano), input.TargetUpdatedAt.Format(time.RFC3339Nano)}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode merge request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) Merge(ctx context.Context, rawInput MergeInput) (MergeOperation, error) {
	if s == nil || s.pool == nil {
		return MergeOperation{}, fmt.Errorf("duplicate operations service not configured")
	}
	input, err := normalizeMergeInput(rawInput)
	if err != nil {
		return MergeOperation{}, err
	}
	digest, err := mergeRequestDigest(input)
	if err != nil {
		return MergeOperation{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return MergeOperation{}, fmt.Errorf("begin duplicate merge: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("duplicate-merge:%d:%s", input.OrganizationID, input.IdempotencyKey)); err != nil {
		return MergeOperation{}, fmt.Errorf("lock duplicate merge request: %w", err)
	}
	if operation, found, err := loadMergeByKey(ctx, tx, input.OrganizationID, input.IdempotencyKey); err != nil {
		return MergeOperation{}, err
	} else if found {
		if operation.requestSHA256 != digest {
			return MergeOperation{}, ErrIdempotencyConflict
		}
		operation.MergeOperation.Replayed = true
		return operation.MergeOperation, tx.Commit(ctx)
	}

	var actorActive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organization_memberships WHERE organization_id=$1 AND user_id=$2 AND membership_status='active')`, input.OrganizationID, input.ActorUserID).Scan(&actorActive); err != nil {
		return MergeOperation{}, fmt.Errorf("validate duplicate merge actor: %w", err)
	}
	if !actorActive {
		return MergeOperation{}, ErrInactiveActor
	}
	records, err := lockMergeRecords(ctx, tx, input)
	if err != nil {
		return MergeOperation{}, err
	}
	if err := moduleclientreviews.RejectScheduledEntities(ctx, tx, input.OrganizationID, input.EntityType, []int64{input.SourceEntityID, input.TargetEntityID}); err != nil {
		if errors.Is(err, moduleclientreviews.ErrActiveSchedule) {
			return MergeOperation{}, fmt.Errorf("%w: clear client review schedules before merging either record", ErrConflict)
		}
		return MergeOperation{}, err
	}
	source := records[input.SourceEntityID]
	target := records[input.TargetEntityID]
	if err := validateCustomSourceFields(ctx, tx, input); err != nil {
		return MergeOperation{}, err
	}
	if !source.UpdatedAt.Equal(input.SourceUpdatedAt) || !target.UpdatedAt.Equal(input.TargetUpdatedAt) {
		return MergeOperation{}, fmt.Errorf("%w: one of the records changed after review; reload candidates", ErrConflict)
	}

	var appliedAt time.Time
	if input.EntityType == "contact" {
		err = updateContactTarget(ctx, tx, input, &appliedAt)
	} else {
		err = updateCompanyTarget(ctx, tx, input, &appliedAt)
	}
	if err != nil {
		return MergeOperation{}, err
	}
	counts, err := moveRelationships(ctx, tx, input)
	if err != nil {
		return MergeOperation{}, err
	}
	if input.EntityType == "company" {
		if _, err := tx.Exec(ctx, `UPDATE companies SET client_type='organization', updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND (SELECT count(*) FROM contact_company_links WHERE organization_id=$1 AND company_id=$2) > 1`, input.OrganizationID, input.TargetEntityID); err != nil {
			return MergeOperation{}, fmt.Errorf("normalize merged client type: %w", err)
		}
		if err := tx.QueryRow(ctx, `SELECT updated_at FROM companies WHERE organization_id=$1 AND id=$2`, input.OrganizationID, input.TargetEntityID).Scan(&appliedAt); err != nil {
			return MergeOperation{}, fmt.Errorf("load merged client version: %w", err)
		}
	}
	entityTable := "contacts"
	if input.EntityType == "company" {
		entityTable = "companies"
	}
	command := fmt.Sprintf(`UPDATE %s SET archived_at=NOW(), updated_at=NOW() WHERE organization_id=$1 AND id=$2 AND archived_at IS NULL`, entityTable)
	if result, err := tx.Exec(ctx, command, input.OrganizationID, input.SourceEntityID); err != nil || result.RowsAffected() != 1 {
		if err == nil {
			err = ErrNotFound
		}
		return MergeOperation{}, fmt.Errorf("archive duplicate source: %w", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO activities (organization_id, entity_type, entity_id, actor_user_id, action, summary) VALUES ($1,$2,$3,$4,'duplicate.merged',$5)`, input.OrganizationID, input.EntityType, input.TargetEntityID, input.ActorUserID, fmt.Sprintf("Merged duplicate %s #%d into this record", input.EntityType, input.SourceEntityID)); err != nil {
		return MergeOperation{}, fmt.Errorf("record duplicate merge activity: %w", err)
	}
	fieldsJSON, _ := json.Marshal(input.SourceFields)
	countsJSON, _ := json.Marshal(counts)
	var operation MergeOperation
	operation.EntityType = input.EntityType
	operation.SourceEntityID = input.SourceEntityID
	operation.SourceLabel = source.Label
	operation.TargetEntityID = input.TargetEntityID
	operation.TargetLabel = target.Label
	operation.SourceFields = input.SourceFields
	operation.RelationshipCounts = counts
	operation.TargetAppliedUpdatedAt = appliedAt
	if err := tx.QueryRow(ctx, `
		INSERT INTO duplicate_merge_operations (organization_id,created_by_user_id,entity_type,source_entity_id,target_entity_id,source_fields,relationship_counts,idempotency_key,request_sha256,source_updated_at,target_updated_at,target_applied_updated_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,$9,$10,$11,$12)
		RETURNING id,created_at
	`, input.OrganizationID, input.ActorUserID, input.EntityType, input.SourceEntityID, input.TargetEntityID, fieldsJSON, countsJSON, input.IdempotencyKey, digest, input.SourceUpdatedAt, input.TargetUpdatedAt, appliedAt).Scan(&operation.ID, &operation.CreatedAt); err != nil {
		return MergeOperation{}, fmt.Errorf("record duplicate merge: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'duplicate.merged',$3,$4,$5,jsonb_build_object('sourceEntityId',$6::bigint,'sourceFields',$7::jsonb,'relationshipCounts',$8::jsonb))
	`, input.OrganizationID, input.ActorUserID, input.EntityType, input.TargetEntityID, fmt.Sprintf("Merged duplicate %s record", input.EntityType), input.SourceEntityID, fieldsJSON, countsJSON); err != nil {
		return MergeOperation{}, fmt.Errorf("audit duplicate merge: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MergeOperation{}, fmt.Errorf("commit duplicate merge: %w", err)
	}
	return operation, nil
}

type lockedRecord struct {
	Label     string
	UpdatedAt time.Time
}

func lockMergeRecords(ctx context.Context, tx pgx.Tx, input MergeInput) (map[int64]lockedRecord, error) {
	label := `trim(first_name || ' ' || last_name)`
	table := "contacts"
	if input.EntityType == "company" {
		label = "name"
		table = "companies"
	}
	query := fmt.Sprintf(`SELECT id,%s,updated_at FROM %s WHERE organization_id=$1 AND id=ANY($2::bigint[]) AND archived_at IS NULL ORDER BY id FOR UPDATE`, label, table)
	rows, err := tx.Query(ctx, query, input.OrganizationID, []int64{input.SourceEntityID, input.TargetEntityID})
	if err != nil {
		return nil, fmt.Errorf("lock duplicate records: %w", err)
	}
	defer rows.Close()
	records := map[int64]lockedRecord{}
	for rows.Next() {
		var id int64
		var record lockedRecord
		if err := rows.Scan(&id, &record.Label, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan locked duplicate record: %w", err)
		}
		records[id] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read locked duplicate records: %w", err)
	}
	if len(records) != 2 {
		return nil, ErrNotFound
	}
	return records, nil
}

type storedMerge struct {
	MergeOperation
	requestSHA256 string
}

func loadMergeByKey(ctx context.Context, tx pgx.Tx, organizationID int64, key string) (storedMerge, bool, error) {
	var stored storedMerge
	var fieldsJSON, countsJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT operation.id,operation.entity_type,operation.source_entity_id,
		       CASE operation.entity_type WHEN 'contact' THEN COALESCE((SELECT trim(first_name || ' ' || last_name) FROM contacts WHERE organization_id=operation.organization_id AND id=operation.source_entity_id),'Contact #'||operation.source_entity_id) ELSE COALESCE((SELECT name FROM companies WHERE organization_id=operation.organization_id AND id=operation.source_entity_id),'Client #'||operation.source_entity_id) END,
		       operation.target_entity_id,
		       CASE operation.entity_type WHEN 'contact' THEN COALESCE((SELECT trim(first_name || ' ' || last_name) FROM contacts WHERE organization_id=operation.organization_id AND id=operation.target_entity_id),'Contact #'||operation.target_entity_id) ELSE COALESCE((SELECT name FROM companies WHERE organization_id=operation.organization_id AND id=operation.target_entity_id),'Client #'||operation.target_entity_id) END,
		       operation.source_fields,operation.relationship_counts,operation.target_applied_updated_at,operation.created_at,operation.request_sha256
		FROM duplicate_merge_operations operation WHERE operation.organization_id=$1 AND operation.idempotency_key=$2
	`, organizationID, key).Scan(&stored.ID, &stored.EntityType, &stored.SourceEntityID, &stored.SourceLabel, &stored.TargetEntityID, &stored.TargetLabel, &fieldsJSON, &countsJSON, &stored.TargetAppliedUpdatedAt, &stored.CreatedAt, &stored.requestSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored, false, nil
	}
	if err != nil {
		return stored, false, fmt.Errorf("load duplicate merge replay: %w", err)
	}
	if err := decodeOperationJSON(fieldsJSON, countsJSON, &stored.MergeOperation); err != nil {
		return stored, false, fmt.Errorf("decode duplicate merge replay: %w", err)
	}
	return stored, true, nil
}

func updateContactTarget(ctx context.Context, tx pgx.Tx, input MergeInput, appliedAt *time.Time) error {
	err := tx.QueryRow(ctx, `
		UPDATE contacts target SET
		  first_name=CASE WHEN 'firstName'=ANY($4::text[]) THEN source.first_name ELSE target.first_name END,
		  last_name=CASE WHEN 'lastName'=ANY($4::text[]) THEN source.last_name ELSE target.last_name END,
		  email=CASE WHEN 'email'=ANY($4::text[]) THEN source.email ELSE target.email END,
		  phone=CASE WHEN 'phone'=ANY($4::text[]) THEN source.phone ELSE target.phone END,
		  job_title=CASE WHEN 'jobTitle'=ANY($4::text[]) THEN source.job_title ELSE target.job_title END,
		  status=CASE WHEN 'status'=ANY($4::text[]) THEN source.status ELSE target.status END,
		  owner_user_id=CASE WHEN 'ownerUserId'=ANY($4::text[]) THEN source.owner_user_id ELSE target.owner_user_id END,
		  address_line1=CASE WHEN 'addressLine1'=ANY($4::text[]) THEN source.address_line1 ELSE target.address_line1 END,
		  address_line2=CASE WHEN 'addressLine2'=ANY($4::text[]) THEN source.address_line2 ELSE target.address_line2 END,
		  city=CASE WHEN 'city'=ANY($4::text[]) THEN source.city ELSE target.city END,
		  state=CASE WHEN 'state'=ANY($4::text[]) THEN source.state ELSE target.state END,
		  postal_code=CASE WHEN 'postalCode'=ANY($4::text[]) THEN source.postal_code ELSE target.postal_code END,
		  country=CASE WHEN 'country'=ANY($4::text[]) THEN source.country ELSE target.country END,
		  lead_source=CASE WHEN 'leadSource'=ANY($4::text[]) THEN source.lead_source ELSE target.lead_source END,
		  first_source_url=CASE WHEN 'firstSourceUrl'=ANY($4::text[]) THEN source.first_source_url ELSE target.first_source_url END,
		  utm_source=CASE WHEN 'utmSource'=ANY($4::text[]) THEN source.utm_source ELSE target.utm_source END,
		  utm_medium=CASE WHEN 'utmMedium'=ANY($4::text[]) THEN source.utm_medium ELSE target.utm_medium END,
		  utm_campaign=CASE WHEN 'utmCampaign'=ANY($4::text[]) THEN source.utm_campaign ELSE target.utm_campaign END,
		  utm_term=CASE WHEN 'utmTerm'=ANY($4::text[]) THEN source.utm_term ELSE target.utm_term END,
		  utm_content=CASE WHEN 'utmContent'=ANY($4::text[]) THEN source.utm_content ELSE target.utm_content END,
		  lead_score=CASE WHEN 'leadScore'=ANY($4::text[]) THEN source.lead_score ELSE target.lead_score END,
		  lead_grade=CASE WHEN 'leadScore'=ANY($4::text[]) THEN source.lead_grade ELSE target.lead_grade END,
		  lead_scored_at=CASE WHEN 'leadScore'=ANY($4::text[]) THEN source.lead_scored_at ELSE target.lead_scored_at END,
		  lead_score_breakdown=CASE WHEN 'leadScore'=ANY($4::text[]) THEN source.lead_score_breakdown ELSE target.lead_score_breakdown END,
		  is_client=target.is_client OR source.is_client,
		  custom_fields=mergeCustomFields(COALESCE(source.custom_fields, '{}'::jsonb), COALESCE(target.custom_fields, '{}'::jsonb), $4::text[]),
		  updated_at=NOW()
		FROM contacts source
		WHERE target.organization_id=$1 AND target.id=$2 AND source.organization_id=$1 AND source.id=$3
		RETURNING target.updated_at
	`, input.OrganizationID, input.TargetEntityID, input.SourceEntityID, input.SourceFields).Scan(appliedAt)
	if err != nil {
		return fmt.Errorf("update surviving contact: %w", err)
	}
	return nil
}

func updateCompanyTarget(ctx context.Context, tx pgx.Tx, input MergeInput, appliedAt *time.Time) error {
	err := tx.QueryRow(ctx, `
		UPDATE companies target SET
		  name=CASE WHEN 'name'=ANY($4::text[]) THEN source.name ELSE target.name END,
		  industry=CASE WHEN 'industry'=ANY($4::text[]) THEN source.industry ELSE target.industry END,
		  phone=CASE WHEN 'phone'=ANY($4::text[]) THEN source.phone ELSE target.phone END,
		  website=CASE WHEN 'website'=ANY($4::text[]) THEN source.website ELSE target.website END,
		  status=CASE WHEN 'status'=ANY($4::text[]) THEN source.status ELSE target.status END,
		  owner_user_id=CASE WHEN 'ownerUserId'=ANY($4::text[]) THEN source.owner_user_id ELSE target.owner_user_id END,
		  address_line1=CASE WHEN 'addressLine1'=ANY($4::text[]) THEN source.address_line1 ELSE target.address_line1 END,
		  address_line2=CASE WHEN 'addressLine2'=ANY($4::text[]) THEN source.address_line2 ELSE target.address_line2 END,
		  city=CASE WHEN 'city'=ANY($4::text[]) THEN source.city ELSE target.city END,
		  state=CASE WHEN 'state'=ANY($4::text[]) THEN source.state ELSE target.state END,
		  postal_code=CASE WHEN 'postalCode'=ANY($4::text[]) THEN source.postal_code ELSE target.postal_code END,
		  country=CASE WHEN 'country'=ANY($4::text[]) THEN source.country ELSE target.country END,
		  client_type=CASE WHEN target.client_type='individual' AND source.client_type='individual' THEN 'individual' ELSE 'organization' END,
		  custom_fields=mergeCustomFields(COALESCE(source.custom_fields, '{}'::jsonb), COALESCE(target.custom_fields, '{}'::jsonb), $4::text[]),
		  updated_at=NOW()
		FROM companies source
		WHERE target.organization_id=$1 AND target.id=$2 AND source.organization_id=$1 AND source.id=$3
		RETURNING target.updated_at
	`, input.OrganizationID, input.TargetEntityID, input.SourceEntityID, input.SourceFields).Scan(appliedAt)
	if err != nil {
		return fmt.Errorf("update surviving company: %w", err)
	}
	return nil
}

func validateCustomSourceFields(ctx context.Context, tx pgx.Tx, input MergeInput) error {
	selected := map[string]struct{}{}
	for _, field := range input.SourceFields {
		if strings.HasPrefix(field, "custom:") {
			selected[strings.TrimPrefix(field, "custom:")] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	definitions, err := modulecustomfields.LoadDefinitions(ctx, tx, input.OrganizationID, input.EntityType, false)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		delete(selected, definition.FieldKey)
	}
	if len(selected) > 0 {
		return fmt.Errorf("%w: a selected custom field is unknown or archived", ErrInvalidInput)
	}
	return nil
}
