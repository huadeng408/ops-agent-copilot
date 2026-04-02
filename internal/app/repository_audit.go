package app

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type AuditRepository struct {
	db DBTX
}

func NewAuditRepository(db DBTX) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) CreateAuditLog(ctx context.Context, log AuditLog) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO audit_logs (trace_id, session_id, user_id, event_type, event_data) VALUES (?, ?, ?, ?, ?)`,
		log.TraceID,
		nullIfEmpty(log.SessionID),
		log.UserID,
		log.EventType,
		log.EventData,
	)
	return err
}

func (r *AuditRepository) CreateToolCallLog(ctx context.Context, log ToolCallLog) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO tool_call_logs (trace_id, session_id, tool_name, tool_type, input_payload, output_payload, success, error_message, latency_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.TraceID,
		log.SessionID,
		log.ToolName,
		log.ToolType,
		log.InputPayload,
		nullIfEmpty(log.OutputPayload),
		log.Success,
		nullIfEmpty(log.ErrorMessage),
		log.LatencyMS,
	)
	return err
}

func (r *AuditRepository) ListByTraceID(ctx context.Context, traceID string, eventType string) ([]AuditLog, error) {
	query := `SELECT id, trace_id, session_id, user_id, event_type, event_data, created_at FROM audit_logs WHERE trace_id = ?`
	args := []any{traceID}
	if strings.TrimSpace(eventType) != "" {
		query += ` AND event_type = ?`
		args = append(args, eventType)
	}
	query += ` ORDER BY created_at ASC, id ASC`
	return r.selectAuditLogs(ctx, query, args...)
}

func (r *AuditRepository) ListRecent(ctx context.Context, limit int, eventType string) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, trace_id, session_id, user_id, event_type, event_data, created_at FROM audit_logs`
	args := []any{}
	if strings.TrimSpace(eventType) != "" {
		query += ` WHERE event_type = ?`
		args = append(args, eventType)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	return r.selectAuditLogs(ctx, query, args...)
}

func (r *AuditRepository) ListToolCallsByTraceID(ctx context.Context, traceID string) ([]ToolCallLog, error) {
	rows := make([]struct {
		ID            int64          `db:"id"`
		TraceID       string         `db:"trace_id"`
		SessionID     string         `db:"session_id"`
		ToolName      string         `db:"tool_name"`
		ToolType      string         `db:"tool_type"`
		InputPayload  string         `db:"input_payload"`
		OutputPayload sql.NullString `db:"output_payload"`
		Success       bool           `db:"success"`
		ErrorMessage  sql.NullString `db:"error_message"`
		LatencyMS     int            `db:"latency_ms"`
		CreatedAt     time.Time      `db:"created_at"`
	}, 0)
	err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, trace_id, session_id, tool_name, tool_type, input_payload, output_payload, success, error_message, latency_ms, created_at
		 FROM tool_call_logs
		 WHERE trace_id = ?
		 ORDER BY created_at ASC, id ASC`,
		traceID,
	)
	if err != nil {
		return nil, err
	}
	result := make([]ToolCallLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, ToolCallLog{
			ID:            row.ID,
			TraceID:       row.TraceID,
			SessionID:     row.SessionID,
			ToolName:      row.ToolName,
			ToolType:      row.ToolType,
			InputPayload:  row.InputPayload,
			OutputPayload: NullableString(row.OutputPayload),
			Success:       row.Success,
			ErrorMessage:  NullableString(row.ErrorMessage),
			LatencyMS:     row.LatencyMS,
			CreatedAt:     row.CreatedAt,
		})
	}
	return result, nil
}

func (r *AuditRepository) ListEventTypes(ctx context.Context) ([]string, error) {
	rows := make([]struct {
		EventType string `db:"event_type"`
	}, 0)
	if err := r.db.SelectContext(ctx, &rows, `SELECT DISTINCT event_type FROM audit_logs ORDER BY event_type ASC`); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.EventType)
	}
	return result, nil
}

func (r *AuditRepository) selectAuditLogs(ctx context.Context, query string, args ...any) ([]AuditLog, error) {
	type row struct {
		ID        int64          `db:"id"`
		TraceID   string         `db:"trace_id"`
		SessionID sql.NullString `db:"session_id"`
		UserID    sql.NullInt64  `db:"user_id"`
		EventType string         `db:"event_type"`
		EventData string         `db:"event_data"`
		CreatedAt time.Time      `db:"created_at"`
	}
	buffer := make([]row, 0)
	if err := r.db.SelectContext(ctx, &buffer, query, args...); err != nil {
		return nil, err
	}
	result := make([]AuditLog, 0, len(buffer))
	for _, item := range buffer {
		var userID *int64
		if item.UserID.Valid {
			value := item.UserID.Int64
			userID = &value
		}
		result = append(result, AuditLog{
			ID:        item.ID,
			TraceID:   item.TraceID,
			SessionID: item.SessionID.String,
			UserID:    userID,
			EventType: item.EventType,
			EventData: item.EventData,
			CreatedAt: item.CreatedAt,
		})
	}
	return result, nil
}
