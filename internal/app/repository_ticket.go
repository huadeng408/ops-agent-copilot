package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type TicketRepository struct {
	db DBTX
}

func NewTicketRepository(db DBTX) *TicketRepository {
	return &TicketRepository{db: db}
}

func (r *TicketRepository) ListSLABreachedTickets(ctx context.Context, targetDate string, region string, groupBy string) ([]map[string]any, error) {
	dayEnd, err := endOfDay(targetDate)
	if err != nil {
		return nil, err
	}
	filters := []string{`t.sla_deadline <= ?`, `t.status NOT IN ('closed', 'resolved')`}
	args := []any{dayEnd}
	if strings.TrimSpace(region) != "" {
		filters = append(filters, `t.region = ?`)
		args = append(args, region)
	}
	if groupBy != "" {
		fieldMap := map[string]string{
			"root_cause":    "t.root_cause",
			"priority":      "t.priority",
			"category":      "t.category",
			"assignee_name": "u.display_name",
		}
		field, ok := fieldMap[groupBy]
		if !ok {
			return nil, NewValidation("group_by 不合法")
		}
		query := fmt.Sprintf(`SELECT %s AS group_key, COUNT(t.id) AS ticket_count
			FROM tickets t
			LEFT JOIN users u ON t.assignee_id = u.id
			WHERE %s
			GROUP BY %s
			ORDER BY ticket_count DESC, group_key ASC`, field, strings.Join(filters, " AND "), field)
		rows := make([]struct {
			GroupKey    sql.NullString `db:"group_key"`
			TicketCount int            `db:"ticket_count"`
		}, 0)
		if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			groupKey := row.GroupKey.String
			if strings.TrimSpace(groupKey) == "" {
				groupKey = "未归类"
			}
			result = append(result, map[string]any{
				"group_key":    groupKey,
				"ticket_count": row.TicketCount,
			})
		}
		return result, nil
	}

	query := fmt.Sprintf(`SELECT t.ticket_no, t.region, t.category, t.status, t.priority, t.root_cause, u.display_name AS assignee_name, t.sla_deadline
		FROM tickets t
		LEFT JOIN users u ON t.assignee_id = u.id
		WHERE %s
		ORDER BY t.sla_deadline ASC
		LIMIT 50`, strings.Join(filters, " AND "))
	rows := make([]struct {
		TicketNo     string         `db:"ticket_no"`
		Region       string         `db:"region"`
		Category     string         `db:"category"`
		Status       string         `db:"status"`
		Priority     string         `db:"priority"`
		RootCause    sql.NullString `db:"root_cause"`
		AssigneeName sql.NullString `db:"assignee_name"`
		SLADeadline  time.Time      `db:"sla_deadline"`
	}, 0)
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"ticket_no":     row.TicketNo,
			"region":        row.Region,
			"category":      row.Category,
			"status":        row.Status,
			"priority":      row.Priority,
			"root_cause":    NullableString(row.RootCause),
			"assignee_name": NullableString(row.AssigneeName),
			"sla_deadline":  row.SLADeadline.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (r *TicketRepository) GetTicketByNo(ctx context.Context, ticketNo string) (*Ticket, error) {
	row := Ticket{}
	if err := r.db.GetContext(ctx, &row, `SELECT * FROM tickets WHERE ticket_no = ?`, ticketNo); err != nil {
		if err == sql.ErrNoRows {
			return nil, NewNotFound(fmt.Sprintf("工单不存在: %s", ticketNo))
		}
		return nil, err
	}
	return &row, nil
}

func (r *TicketRepository) GetTicketDetail(ctx context.Context, ticketNo string) (map[string]any, error) {
	row := struct {
		TicketNo     string         `db:"ticket_no"`
		Region       string         `db:"region"`
		Category     string         `db:"category"`
		Title        string         `db:"title"`
		Description  string         `db:"description"`
		Status       string         `db:"status"`
		Priority     string         `db:"priority"`
		RootCause    sql.NullString `db:"root_cause"`
		AssigneeName sql.NullString `db:"assignee_name"`
		ReporterName sql.NullString `db:"reporter_name"`
		SLADeadline  time.Time      `db:"sla_deadline"`
		CreatedAt    time.Time      `db:"created_at"`
		UpdatedAt    time.Time      `db:"updated_at"`
		ResolvedAt   sql.NullTime   `db:"resolved_at"`
	}{}
	err := r.db.GetContext(
		ctx,
		&row,
		`SELECT t.ticket_no, t.region, t.category, t.title, t.description, t.status, t.priority, t.root_cause,
		        assignee.display_name AS assignee_name, reporter.display_name AS reporter_name,
				t.sla_deadline, t.created_at, t.updated_at, t.resolved_at
		 FROM tickets t
		 LEFT JOIN users assignee ON t.assignee_id = assignee.id
		 LEFT JOIN users reporter ON t.reporter_id = reporter.id
		 WHERE t.ticket_no = ?`,
		ticketNo,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, NewNotFound(fmt.Sprintf("工单不存在: %s", ticketNo))
		}
		return nil, err
	}
	return map[string]any{
		"ticket_no":     row.TicketNo,
		"region":        row.Region,
		"category":      row.Category,
		"title":         row.Title,
		"description":   row.Description,
		"status":        row.Status,
		"priority":      row.Priority,
		"root_cause":    NullableString(row.RootCause),
		"assignee_name": NullableString(row.AssigneeName),
		"reporter_name": NullableString(row.ReporterName),
		"sla_deadline":  row.SLADeadline.Format(time.RFC3339),
		"created_at":    row.CreatedAt.Format(time.RFC3339),
		"updated_at":    row.UpdatedAt.Format(time.RFC3339),
		"resolved_at":   formatNullTime(row.ResolvedAt),
	}, nil
}

func (r *TicketRepository) GetTicketComments(ctx context.Context, ticketNo string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	ticket, err := r.GetTicketByNo(ctx, ticketNo)
	if err != nil {
		return nil, err
	}
	rows := make([]struct {
		CommentText string         `db:"comment_text"`
		CreatedBy   sql.NullString `db:"created_by_name"`
		CreatedAt   time.Time      `db:"created_at"`
	}, 0)
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT c.comment_text, u.display_name AS created_by_name, c.created_at
		 FROM ticket_comments c
		 LEFT JOIN users u ON c.created_by = u.id
		 WHERE c.ticket_id = ?
		 ORDER BY c.created_at DESC
		 LIMIT ?`,
		ticket.ID,
		limit,
	); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"comment_text": row.CommentText,
			"created_by":   NullableString(row.CreatedBy),
			"created_at":   row.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (r *TicketRepository) GetRecentTicketActions(ctx context.Context, ticketNo string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	ticket, err := r.GetTicketByNo(ctx, ticketNo)
	if err != nil {
		return nil, err
	}
	rows := make([]struct {
		ActionType string         `db:"action_type"`
		OldValue   sql.NullString `db:"old_value"`
		NewValue   sql.NullString `db:"new_value"`
		TraceID    string         `db:"trace_id"`
		CreatedAt  time.Time      `db:"created_at"`
	}, 0)
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT action_type, old_value, new_value, trace_id, created_at
		 FROM ticket_actions
		 WHERE ticket_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		ticket.ID,
		limit,
	); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"action_type": row.ActionType,
			"old_value":   ParseJSONMap(NullableString(row.OldValue)),
			"new_value":   ParseJSONMap(NullableString(row.NewValue)),
			"trace_id":    row.TraceID,
			"created_at":  row.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

func (r *TicketRepository) AssignTicket(ctx context.Context, ticketNo string, assignee User, operatorID int64, approvalID int64, traceID string) (map[string]any, error) {
	ticket, err := r.GetTicketByNo(ctx, ticketNo)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE tickets SET assignee_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, assignee.ID, ticket.ID); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO ticket_actions (ticket_id, action_type, old_value, new_value, operator_id, approval_id, trace_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ticket.ID,
		"assign_ticket",
		MustJSON(map[string]any{"assignee_id": ticket.AssigneeID}),
		MustJSON(map[string]any{"assignee_id": assignee.ID, "assignee_name": assignee.DisplayName}),
		operatorID,
		approvalID,
		traceID,
	); err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": ticket.TicketNo, "assignee_name": assignee.DisplayName}, nil
}

func (r *TicketRepository) AddTicketComment(ctx context.Context, ticketNo string, commentText string, operatorID int64, approvalID int64, traceID string) (map[string]any, error) {
	ticket, err := r.GetTicketByNo(ctx, ticketNo)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO ticket_comments (ticket_id, comment_text, created_by) VALUES (?, ?, ?)`, ticket.ID, commentText, operatorID); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO ticket_actions (ticket_id, action_type, old_value, new_value, operator_id, approval_id, trace_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ticket.ID,
		"add_ticket_comment",
		nil,
		MustJSON(map[string]any{"comment_text": commentText}),
		operatorID,
		approvalID,
		traceID,
	); err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": ticket.TicketNo, "comment_text": commentText}, nil
}

func (r *TicketRepository) EscalateTicket(ctx context.Context, ticketNo string, newPriority string, operatorID int64, approvalID int64, traceID string) (map[string]any, error) {
	ticket, err := r.GetTicketByNo(ctx, ticketNo)
	if err != nil {
		return nil, err
	}
	oldPriority := ticket.Priority
	if _, err := r.db.ExecContext(ctx, `UPDATE tickets SET priority = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, newPriority, ticket.ID); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO ticket_actions (ticket_id, action_type, old_value, new_value, operator_id, approval_id, trace_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ticket.ID,
		"escalate_ticket",
		MustJSON(map[string]any{"priority": oldPriority}),
		MustJSON(map[string]any{"priority": newPriority}),
		operatorID,
		approvalID,
		traceID,
	); err != nil {
		return nil, err
	}
	return map[string]any{"ticket_no": ticket.TicketNo, "old_priority": oldPriority, "new_priority": newPriority}, nil
}

func (r *TicketRepository) GetHighPriorityOpenTickets(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	rows := make([]Ticket, 0)
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT * FROM tickets
		 WHERE priority IN ('P1', 'P2') AND status NOT IN ('closed', 'resolved')
		 ORDER BY priority ASC, sla_deadline ASC
		 LIMIT ?`,
		limit,
	); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"ticket_no":  row.TicketNo,
			"priority":   row.Priority,
			"region":     row.Region,
			"category":   row.Category,
			"status":     row.Status,
			"root_cause": row.RootCause,
		})
	}
	return result, nil
}

func (r *TicketRepository) ListSLABreachSamples(ctx context.Context, targetDate string, region string, categories []string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	dayEnd, err := endOfDay(targetDate)
	if err != nil {
		return nil, err
	}
	filters := []string{`t.sla_deadline <= ?`, `t.status NOT IN ('closed', 'resolved')`}
	args := []any{dayEnd}
	if strings.TrimSpace(region) != "" {
		filters = append(filters, `t.region = ?`)
		args = append(args, region)
	}
	if len(categories) > 0 {
		placeholders := make([]string, 0, len(categories))
		for _, category := range categories {
			if strings.TrimSpace(category) == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, category)
		}
		if len(placeholders) > 0 {
			filters = append(filters, fmt.Sprintf(`t.category IN (%s)`, strings.Join(placeholders, ",")))
		}
	}
	args = append(args, limit)
	query := fmt.Sprintf(`SELECT t.ticket_no, t.region, t.category, t.priority, t.status, t.root_cause, u.display_name AS assignee_name, t.sla_deadline
		FROM tickets t
		LEFT JOIN users u ON t.assignee_id = u.id
		WHERE %s
		ORDER BY t.priority ASC, t.sla_deadline ASC
		LIMIT ?`, strings.Join(filters, " AND "))

	rows := make([]struct {
		TicketNo     string         `db:"ticket_no"`
		Region       string         `db:"region"`
		Category     string         `db:"category"`
		Priority     string         `db:"priority"`
		Status       string         `db:"status"`
		RootCause    sql.NullString `db:"root_cause"`
		AssigneeName sql.NullString `db:"assignee_name"`
		SLADeadline  time.Time      `db:"sla_deadline"`
	}, 0)
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"ticket_no":     row.TicketNo,
			"region":        row.Region,
			"category":      row.Category,
			"priority":      row.Priority,
			"status":        row.Status,
			"root_cause":    NullableString(row.RootCause),
			"assignee_name": NullableString(row.AssigneeName),
			"sla_deadline":  row.SLADeadline.Format(time.RFC3339),
		})
	}
	return result, nil
}

func formatNullTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.Format(time.RFC3339)
}

func endOfDay(dateValue string) (time.Time, error) {
	value, err := time.Parse("2006-01-02", dateValue)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 23, 59, 59, 0, value.Location()), nil
}
