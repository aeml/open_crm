package duplicateoperations

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	"github.com/jackc/pgx/v5"
)

func (s *Service) loadRecords(ctx context.Context, organizationID int64, entityType string, ids []int64) (map[int64]Record, error) {
	records := map[int64]Record{}
	if len(ids) == 0 {
		return records, nil
	}
	if entityType == "contact" {
		return s.loadContacts(ctx, organizationID, ids)
	}
	return s.loadCompanies(ctx, organizationID, ids)
}

func (s *Service) loadContacts(ctx context.Context, organizationID int64, ids []int64) (map[int64]Record, error) {
	definitions, err := modulecustomfields.LoadDefinitions(ctx, s.pool, organizationID, "contact", false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.first_name, c.last_name, COALESCE(c.email, ''), COALESCE(c.phone, ''),
		       COALESCE(c.address_line1, ''), COALESCE(c.address_line2, ''), COALESCE(c.city, ''),
		       COALESCE(c.state, ''), COALESCE(c.postal_code, ''), COALESCE(c.country, ''),
		       COALESCE(c.job_title, ''), COALESCE(c.status, ''), COALESCE(c.owner_user_id, 0),
		       COALESCE(NULLIF(trim(u.first_name || ' ' || u.last_name), ''), u.email, ''), c.is_client,
		       c.lead_source, c.first_source_url, c.utm_source, c.utm_medium, c.utm_campaign,
		       c.utm_term, c.utm_content, c.lead_score, COALESCE(c.custom_fields, '{}'::jsonb), c.updated_at
		FROM contacts c
		LEFT JOIN users u ON u.id = c.owner_user_id
		WHERE c.organization_id = $1 AND c.archived_at IS NULL AND c.id = ANY($2::bigint[])
	`, organizationID, ids)
	if err != nil {
		return nil, fmt.Errorf("load duplicate contact records: %w", err)
	}
	defer rows.Close()
	records := map[int64]Record{}
	for rows.Next() {
		var record Record
		var firstName, lastName, email, phone, address1, address2, city, state, postal, country string
		var jobTitle, status, ownerName, leadSource, firstSourceURL, utmSource, utmMedium, utmCampaign, utmTerm, utmContent string
		var ownerID int64
		var isClient bool
		var leadScore int
		var customFieldsJSON []byte
		if err := rows.Scan(&record.ID, &firstName, &lastName, &email, &phone, &address1, &address2, &city, &state, &postal, &country, &jobTitle, &status, &ownerID, &ownerName, &isClient, &leadSource, &firstSourceURL, &utmSource, &utmMedium, &utmCampaign, &utmTerm, &utmContent, &leadScore, &customFieldsJSON, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan duplicate contact record: %w", err)
		}
		record.Label = strings.TrimSpace(firstName + " " + lastName)
		record.Fields = []Field{
			makeField("firstName", "First name", firstName), makeField("lastName", "Last name", lastName),
			makeField("email", "Email", email), makeField("phone", "Phone", phone), makeField("jobTitle", "Job title", jobTitle),
			makeField("status", "Status", status), makeField("ownerUserId", "Owner", strconv.FormatInt(ownerID, 10)),
			makeField("addressLine1", "Address line 1", address1), makeField("addressLine2", "Address line 2", address2),
			makeField("city", "City", city), makeField("state", "State", state), makeField("postalCode", "Postal code", postal), makeField("country", "Country", country),
			makeField("leadSource", "Lead source", leadSource), makeField("firstSourceUrl", "First source URL", firstSourceURL),
			makeField("utmSource", "UTM source", utmSource), makeField("utmMedium", "UTM medium", utmMedium), makeField("utmCampaign", "UTM campaign", utmCampaign),
			makeField("utmTerm", "UTM term", utmTerm), makeField("utmContent", "UTM content", utmContent), makeField("leadScore", "Lead score", strconv.Itoa(leadScore)),
			makeReadOnlyField("isClient", "Client status (retained if either record is a client)", boolString(isClient)),
		}
		for index := range record.Fields {
			if record.Fields[index].Key == "ownerUserId" {
				record.Fields[index].DisplayValue = ownerName
				if ownerName == "" {
					record.Fields[index].DisplayValue = "Unassigned"
				}
			}
		}
		if err := appendCustomFields(&record, definitions, customFieldsJSON); err != nil {
			return nil, err
		}
		record.Related = map[string]int{}
		records[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read duplicate contact records: %w", err)
	}
	return records, nil
}

func (s *Service) loadCompanies(ctx context.Context, organizationID int64, ids []int64) (map[int64]Record, error) {
	definitions, err := modulecustomfields.LoadDefinitions(ctx, s.pool, organizationID, "company", false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, c.client_type, COALESCE(c.address_line1, ''), COALESCE(c.address_line2, ''),
		       COALESCE(c.city, ''), COALESCE(c.state, ''), COALESCE(c.postal_code, ''), COALESCE(c.country, ''),
		       COALESCE(c.industry, ''), COALESCE(c.phone, ''), COALESCE(c.website, ''), COALESCE(c.status, ''),
		       COALESCE(c.owner_user_id, 0), COALESCE(NULLIF(trim(u.first_name || ' ' || u.last_name), ''), u.email, ''), COALESCE(c.custom_fields, '{}'::jsonb), c.updated_at
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_user_id
		WHERE c.organization_id = $1 AND c.archived_at IS NULL AND c.id = ANY($2::bigint[])
	`, organizationID, ids)
	if err != nil {
		return nil, fmt.Errorf("load duplicate company records: %w", err)
	}
	defer rows.Close()
	records := map[int64]Record{}
	for rows.Next() {
		var record Record
		var name, clientType, address1, address2, city, state, postal, country, industry, phone, website, status, ownerName string
		var ownerID int64
		var customFieldsJSON []byte
		if err := rows.Scan(&record.ID, &name, &clientType, &address1, &address2, &city, &state, &postal, &country, &industry, &phone, &website, &status, &ownerID, &ownerName, &customFieldsJSON, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan duplicate company record: %w", err)
		}
		record.Label = name
		record.Fields = []Field{
			makeField("name", "Name", name), makeReadOnlyField("clientType", "Client type (safest type retained)", clientType), makeField("industry", "Industry", industry),
			makeField("phone", "Phone", phone), makeField("website", "Website", website), makeField("status", "Status", status),
			makeField("ownerUserId", "Owner", strconv.FormatInt(ownerID, 10)), makeField("addressLine1", "Address line 1", address1),
			makeField("addressLine2", "Address line 2", address2), makeField("city", "City", city), makeField("state", "State", state),
			makeField("postalCode", "Postal code", postal), makeField("country", "Country", country),
		}
		for index := range record.Fields {
			if record.Fields[index].Key == "ownerUserId" {
				record.Fields[index].DisplayValue = ownerName
				if ownerName == "" {
					record.Fields[index].DisplayValue = "Unassigned"
				}
			}
		}
		if err := appendCustomFields(&record, definitions, customFieldsJSON); err != nil {
			return nil, err
		}
		record.Related = map[string]int{}
		records[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read duplicate company records: %w", err)
	}
	return records, nil
}

func appendCustomFields(record *Record, definitions []modulecustomfields.Definition, encoded []byte) error {
	values, err := modulecustomfields.DecodeValues(encoded)
	if err != nil {
		return fmt.Errorf("decode duplicate custom fields: %w", err)
	}
	for _, definition := range definitions {
		value := modulecustomfields.FormatValue(definition, values[definition.FieldKey])
		field := makeField("custom:"+definition.FieldKey, definition.Label+" (custom)", value)
		if definition.DataType == "boolean" && len(values[definition.FieldKey]) > 0 {
			var boolean bool
			if json.Unmarshal(values[definition.FieldKey], &boolean) == nil {
				field.Value = strconv.FormatBool(boolean)
				field.DisplayValue = field.Value
			}
		}
		record.Fields = append(record.Fields, field)
	}
	return nil
}

func (s *Service) loadRelatedCounts(ctx context.Context, organizationID int64, entityType string, ids []int64, records map[int64]Record) error {
	if len(ids) == 0 {
		return nil
	}
	query := contactRelatedCountsQuery
	if entityType == "company" {
		query = companyRelatedCountsQuery
	}
	rows, err := s.pool.Query(ctx, query, organizationID, ids)
	if err != nil {
		return fmt.Errorf("load duplicate relationship counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entityID int64
		var kind string
		var count int
		if err := rows.Scan(&entityID, &kind, &count); err != nil {
			return fmt.Errorf("scan duplicate relationship count: %w", err)
		}
		record, ok := records[entityID]
		if ok {
			record.Related[kind] = count
			records[entityID] = record
		}
	}
	return rows.Err()
}

const contactRelatedCountsQuery = `
	SELECT entity_id, kind, count(*)::int FROM (
	  SELECT entity_id, 'notes'::text kind, id FROM notes WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'tasks', id FROM tasks WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'activities', id FROM activities WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT primary_contact_id, 'deals', id FROM deals WHERE organization_id=$1 AND primary_contact_id=ANY($2::bigint[])
	  UNION ALL SELECT contact_id, 'clients', id FROM contact_company_links WHERE organization_id=$1 AND contact_id=ANY($2::bigint[])
	  UNION ALL SELECT contact_id, 'sequences', id FROM email_sequence_enrollments WHERE organization_id=$1 AND contact_id=ANY($2::bigint[])
	  UNION ALL SELECT contact_id, 'lead submissions', id FROM lead_capture_submissions WHERE organization_id=$1 AND contact_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'communications', id FROM call_logs WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'communications', id FROM sms_messages WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'meetings', id FROM calendar_events WHERE organization_id=$1 AND entity_type='contact' AND entity_id=ANY($2::bigint[])
	) related GROUP BY entity_id, kind ORDER BY entity_id, kind`

const companyRelatedCountsQuery = `
	SELECT entity_id, kind, count(*)::int FROM (
	  SELECT entity_id, 'notes'::text kind, id FROM notes WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'tasks', id FROM tasks WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'activities', id FROM activities WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT company_id, 'deals', id FROM deals WHERE organization_id=$1 AND company_id=ANY($2::bigint[])
	  UNION ALL SELECT company_id, 'contacts', id FROM contact_company_links WHERE organization_id=$1 AND company_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'communications', id FROM call_logs WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'communications', id FROM sms_messages WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	  UNION ALL SELECT entity_id, 'meetings', id FROM calendar_events WHERE organization_id=$1 AND entity_type='company' AND entity_id=ANY($2::bigint[])
	) related GROUP BY entity_id, kind ORDER BY entity_id, kind`

func (s *Service) listMerges(ctx context.Context, organizationID int64, entityType string, limit int) ([]MergeOperation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT operation.id, operation.entity_type, operation.source_entity_id,
		       CASE operation.entity_type WHEN 'contact' THEN COALESCE((SELECT trim(first_name || ' ' || last_name) FROM contacts WHERE organization_id=operation.organization_id AND id=operation.source_entity_id), 'Contact #' || operation.source_entity_id)
		                                  ELSE COALESCE((SELECT name FROM companies WHERE organization_id=operation.organization_id AND id=operation.source_entity_id), 'Client #' || operation.source_entity_id) END,
		       operation.target_entity_id,
		       CASE operation.entity_type WHEN 'contact' THEN COALESCE((SELECT trim(first_name || ' ' || last_name) FROM contacts WHERE organization_id=operation.organization_id AND id=operation.target_entity_id), 'Contact #' || operation.target_entity_id)
		                                  ELSE COALESCE((SELECT name FROM companies WHERE organization_id=operation.organization_id AND id=operation.target_entity_id), 'Client #' || operation.target_entity_id) END,
		       operation.source_fields, operation.relationship_counts, operation.target_applied_updated_at, operation.created_at
		FROM duplicate_merge_operations operation
		WHERE operation.organization_id=$1 AND operation.entity_type=$2
		ORDER BY operation.created_at DESC, operation.id DESC LIMIT $3
	`, organizationID, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("list duplicate merge history: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanMergeOperation)
}

func scanMergeOperation(row pgx.CollectableRow) (MergeOperation, error) {
	var operation MergeOperation
	var fieldsJSON, countsJSON []byte
	err := row.Scan(&operation.ID, &operation.EntityType, &operation.SourceEntityID, &operation.SourceLabel, &operation.TargetEntityID, &operation.TargetLabel, &fieldsJSON, &countsJSON, &operation.TargetAppliedUpdatedAt, &operation.CreatedAt)
	if err == nil {
		err = decodeOperationJSON(fieldsJSON, countsJSON, &operation)
	}
	return operation, err
}
