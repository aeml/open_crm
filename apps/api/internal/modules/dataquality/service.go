// Package dataquality provides focused, executable CRM cleanup reports.
package dataquality

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidInput = errors.New("invalid data quality query")

type Query struct{ StaleDays int }

type Record struct {
	EntityType string    `json:"entityType"`
	EntityID   int64     `json:"entityId"`
	Label      string    `json:"label"`
	Detail     string    `json:"detail"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Report struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Count       int      `json:"count"`
	Records     []Record `json:"records"`
}

type Summary struct {
	BusinessType string    `json:"businessType"`
	StaleDays    int       `json:"staleDays"`
	GeneratedAt  time.Time `json:"generatedAt"`
	Reports      []Report  `json:"reports"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Summary(ctx context.Context, organizationID int64, query Query) (Summary, error) {
	if s == nil || s.pool == nil {
		return Summary{}, fmt.Errorf("data quality service not configured")
	}
	if organizationID <= 0 {
		return Summary{}, ErrInvalidInput
	}
	if query.StaleDays == 0 {
		query.StaleDays = 30
	}
	if query.StaleDays < 7 || query.StaleDays > 365 {
		return Summary{}, fmt.Errorf("%w: staleDays must be between 7 and 365", ErrInvalidInput)
	}
	var businessType string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(NULLIF(business_type,''),'general') FROM organizations WHERE id=$1`, organizationID).Scan(&businessType); errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, ErrInvalidInput
	} else if err != nil {
		return Summary{}, fmt.Errorf("load data quality business profile: %w", err)
	}

	reports := make([]Report, 0, 6)
	definitions := []struct {
		key, title, description, sql string
		args                         []any
	}{
		{"missing_owners", "Records without an owner", "Active contacts, clients, open deals, and open tasks without a responsible teammate.", missingOwnersSQL, []any{organizationID}},
		{"missing_contact_details", "Contacts without contact details", "Active contacts with neither an email address nor a phone number.", missingContactDetailsSQL, []any{organizationID}},
		{"stale_deals", "Stale open deals", fmt.Sprintf("Open deals not updated in the last %d days.", query.StaleDays), staleDealsSQL, []any{organizationID, query.StaleDays}},
		{"incomplete_deals", "Incomplete open deals", "Open deals missing a client, primary contact, value, or expected close date.", incompleteDealsSQL, []any{organizationID}},
		{"unscheduled_tasks", "Open tasks without a due date", "Active open tasks that have no due date and can be missed during follow-up.", unscheduledTasksSQL, []any{organizationID}},
	}
	for _, definition := range definitions {
		report, err := s.queryReport(ctx, definition.key, definition.title, definition.description, definition.sql, definition.args...)
		if err != nil {
			return Summary{}, err
		}
		reports = append(reports, report)
	}
	if definition, ok := businessRule(businessType, organizationID); ok {
		report, err := s.queryReport(ctx, definition.key, definition.title, definition.description, definition.sql, definition.args...)
		if err != nil {
			return Summary{}, err
		}
		reports = append(reports, report)
	}
	return Summary{BusinessType: businessType, StaleDays: query.StaleDays, GeneratedAt: time.Now().UTC(), Reports: reports}, nil
}

func (s *Service) queryReport(ctx context.Context, key, title, description, sql string, args ...any) (Report, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return Report{}, fmt.Errorf("run %s data quality report: %w", key, err)
	}
	defer rows.Close()
	report := Report{Key: key, Title: title, Description: description, Records: []Record{}}
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.EntityType, &record.EntityID, &record.Label, &record.Detail, &record.UpdatedAt, &report.Count); err != nil {
			return Report{}, fmt.Errorf("scan %s data quality report: %w", key, err)
		}
		report.Records = append(report.Records, record)
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("iterate %s data quality report: %w", key, err)
	}
	return report, nil
}

type reportDefinition struct {
	key, title, description, sql string
	args                         []any
}

func businessRule(businessType string, organizationID int64) (reportDefinition, bool) {
	switch strings.TrimSpace(businessType) {
	case "services":
		return reportDefinition{"service_clients_without_people", "Clients without a linked person", "Organization clients marked as customers but missing a linked person for service communication.", serviceClientsWithoutPeopleSQL, []any{organizationID}}, true
	case "construction-services":
		return reportDefinition{"construction_clients_without_location", "Clients without a service location", "Customer clients missing street, city, state, postal code, and country information.", constructionClientsWithoutLocationSQL, []any{organizationID}}, true
	case "product-sales":
		return reportDefinition{"product_accounts_without_industry", "Accounts without an industry", "Active organization accounts missing industry context used for product-sales qualification.", productAccountsWithoutIndustrySQL, []any{organizationID}}, true
	default:
		return reportDefinition{}, false
	}
}

const missingOwnersSQL = `
	WITH issues AS (
		SELECT 'contact'::text entity_type,id,COALESCE(NULLIF(trim(first_name||' '||last_name),''),'Contact #'||id::text) label,'Contact has no owner'::text detail,updated_at FROM contacts WHERE organization_id=$1 AND archived_at IS NULL AND owner_user_id IS NULL
		UNION ALL SELECT 'company',id,name,'Client has no owner',updated_at FROM companies WHERE organization_id=$1 AND archived_at IS NULL AND owner_user_id IS NULL
		UNION ALL SELECT 'deal',d.id,d.name,'Open deal has no owner',d.updated_at FROM deals d JOIN deal_stages ds ON ds.id=d.stage_id AND ds.organization_id=d.organization_id WHERE d.organization_id=$1 AND d.archived_at IS NULL AND NOT ds.is_closed AND d.owner_user_id IS NULL
		UNION ALL SELECT 'task',id,title,'Open task has no assignee',updated_at FROM tasks WHERE organization_id=$1 AND archived_at IS NULL AND status='open' AND assigned_to_user_id IS NULL
	)
	SELECT entity_type,id,label,detail,updated_at,COUNT(*) OVER() FROM issues ORDER BY updated_at,id LIMIT 25`

const missingContactDetailsSQL = `
	SELECT 'contact',id,COALESCE(NULLIF(trim(first_name||' '||last_name),''),'Contact #'||id::text),
	       'Neither email nor phone is set',updated_at,COUNT(*) OVER()
	FROM contacts WHERE organization_id=$1 AND archived_at IS NULL AND trim(COALESCE(email,''))='' AND trim(COALESCE(phone,''))=''
	ORDER BY updated_at,id LIMIT 25`

const staleDealsSQL = `
	SELECT 'deal',d.id,d.name,'Last updated '||to_char(d.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD')||' UTC',d.updated_at,COUNT(*) OVER()
	FROM deals d JOIN deal_stages ds ON ds.id=d.stage_id AND ds.organization_id=d.organization_id
	WHERE d.organization_id=$1 AND d.archived_at IS NULL AND NOT ds.is_closed AND d.updated_at < NOW()-make_interval(days=>$2)
	ORDER BY d.updated_at,d.id LIMIT 25`

const incompleteDealsSQL = `
	SELECT 'deal',d.id,d.name,
	       'Missing '||concat_ws(', ',CASE WHEN d.company_id IS NULL THEN 'client' END,CASE WHEN d.primary_contact_id IS NULL THEN 'primary contact' END,CASE WHEN d.value_amount IS NULL THEN 'value' END,CASE WHEN d.expected_close_date IS NULL THEN 'expected close date' END),
	       d.updated_at,COUNT(*) OVER()
	FROM deals d JOIN deal_stages ds ON ds.id=d.stage_id AND ds.organization_id=d.organization_id
	WHERE d.organization_id=$1 AND d.archived_at IS NULL AND NOT ds.is_closed AND (d.company_id IS NULL OR d.primary_contact_id IS NULL OR d.value_amount IS NULL OR d.expected_close_date IS NULL)
	ORDER BY d.updated_at,d.id LIMIT 25`

const unscheduledTasksSQL = `
	SELECT 'task',id,title,'Open task has no due date',updated_at,COUNT(*) OVER()
	FROM tasks WHERE organization_id=$1 AND archived_at IS NULL AND status='open' AND due_at IS NULL
	ORDER BY updated_at,id LIMIT 25`

const serviceClientsWithoutPeopleSQL = `
	SELECT 'company',c.id,c.name,'Customer client has no linked person',c.updated_at,COUNT(*) OVER()
	FROM companies c WHERE c.organization_id=$1 AND c.archived_at IS NULL AND c.client_type='organization' AND c.status='customer'
	AND NOT EXISTS (SELECT 1 FROM contact_company_links l JOIN contacts p ON p.id=l.contact_id AND p.organization_id=l.organization_id AND p.archived_at IS NULL WHERE l.organization_id=$1 AND l.company_id=c.id)
	ORDER BY c.updated_at,c.id LIMIT 25`

const constructionClientsWithoutLocationSQL = `
	SELECT 'company',id,name,'Customer client has no service-location address',updated_at,COUNT(*) OVER()
	FROM companies WHERE organization_id=$1 AND archived_at IS NULL AND status='customer'
	AND trim(COALESCE(address_line1,''))='' AND trim(COALESCE(city,''))='' AND trim(COALESCE(state,''))='' AND trim(COALESCE(postal_code,''))='' AND trim(COALESCE(country,''))=''
	ORDER BY updated_at,id LIMIT 25`

const productAccountsWithoutIndustrySQL = `
	SELECT 'company',id,name,'Organization account has no industry',updated_at,COUNT(*) OVER()
	FROM companies WHERE organization_id=$1 AND archived_at IS NULL AND client_type='organization' AND trim(COALESCE(industry,''))=''
	ORDER BY updated_at,id LIMIT 25`
