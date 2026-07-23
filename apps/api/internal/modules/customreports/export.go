package customreports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const MaxExportRows = 10000

type CSVFile struct {
	Filename string
	Content  []byte
	RowCount int
}

func (s *Service) ExportCSV(ctx context.Context, organizationID, actorUserID, definitionID int64) (CSVFile, error) {
	if s == nil || s.pool == nil {
		return CSVFile{}, fmt.Errorf("custom reports service not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CSVFile{}, fmt.Errorf("begin custom report export: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveReportAdmin(ctx, tx, organizationID, actorUserID); err != nil {
		return CSVFile{}, err
	}
	definition, file, err := generateCSV(ctx, tx, organizationID, definitionID, time.Now().UTC())
	if err != nil {
		return CSVFile{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id,actor_user_id,event_type,entity_type,entity_id,summary,metadata_json)
		VALUES ($1,$2,'report.export_downloaded','report_definition',$3,'Downloaded saved report CSV',jsonb_build_object('sourceType',$4::text,'rowCount',$5::integer,'columnCount',$6::integer,'visualizationType',$7::text,'visualizationContract',$8::text))
	`, organizationID, actorUserID, definition.ID, definition.SourceType, file.RowCount, definitionColumnCount(definition), definition.VisualizationType, definition.VisualizationContract); err != nil {
		return CSVFile{}, fmt.Errorf("record custom report export audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CSVFile{}, fmt.Errorf("commit custom report export: %w", err)
	}

	return file, nil
}

func generateCSV(ctx context.Context, querier executionQuerier, organizationID, definitionID int64, generatedAt time.Time) (Definition, CSVFile, error) {
	definition, input, err := loadExecutableDefinition(ctx, querier, organizationID, definitionID)
	if err != nil {
		return Definition{}, CSVFile{}, err
	}
	statement, args, columns, err := buildExecutionStatementWindow(organizationID, input, MaxExportRows+1, 0)
	if err != nil {
		return Definition{}, CSVFile{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	rows, err := queryExecutionRows(queryCtx, querier, statement, args, columns)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return Definition{}, CSVFile{}, ErrQueryTimeout
		}
		return Definition{}, CSVFile{}, err
	}
	if len(rows) > MaxExportRows {
		return Definition{}, CSVFile{}, ErrTooManyRows
	}

	records := make([][]string, 0, len(rows)+1)
	header := make([]string, len(columns))
	for index, column := range columns {
		header[index] = column.Label
	}
	records = append(records, header)
	for _, row := range rows {
		record := make([]string, len(columns))
		for index, column := range columns {
			if value := row.Values[column.Key]; value != nil {
				record[index] = spreadsheetSafe(*value)
			}
		}
		records = append(records, record)
	}

	var content bytes.Buffer
	content.WriteString("\ufeff")
	writer := csv.NewWriter(&content)
	if err := writer.WriteAll(records); err != nil {
		return Definition{}, CSVFile{}, fmt.Errorf("write custom report CSV: %w", err)
	}
	if err := writer.Error(); err != nil {
		return Definition{}, CSVFile{}, fmt.Errorf("flush custom report CSV: %w", err)
	}
	return definition, CSVFile{
		Filename: "saved-report-" + strconv.FormatInt(definition.ID, 10) + "-" + generatedAt.UTC().Format("20060102") + ".csv",
		Content:  content.Bytes(),
		RowCount: len(rows),
	}, nil
}

func definitionColumnCount(definition Definition) int {
	if definition.Aggregation.Function != "none" {
		if definition.GroupBy != "" {
			return 2
		}
		return 1
	}
	return len(definition.Columns)
}

func spreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" || !strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return value
	}
	return "'" + value
}
