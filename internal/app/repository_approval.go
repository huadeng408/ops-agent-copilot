package app

import (
	"context"
	"database/sql"
	"time"
)

type ApprovalRepository struct {
	db DBTX
}

func NewApprovalRepository(db DBTX) *ApprovalRepository {
	return &ApprovalRepository{db: db}
}

func (r *ApprovalRepository) Create(ctx context.Context, approval *Approval) error {
	result, err := r.db.ExecContext(
		ctx,
		`INSERT INTO approvals (
			approval_no, idempotency_key, session_id, trace_id, action_type, target_type, target_id,
			payload, reason, status, requested_by, approved_by, approved_at, executed_at,
			execution_result, execution_error, version, rejected_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ApprovalNo,
		approval.IdempotencyKey,
		approval.SessionID,
		approval.TraceID,
		approval.ActionType,
		approval.TargetType,
		approval.TargetID,
		approval.Payload,
		approval.Reason,
		approval.Status,
		approval.RequestedBy,
		approval.ApprovedBy,
		approval.ApprovedAt,
		approval.ExecutedAt,
		nullIfEmpty(approval.ExecutionResult),
		nullIfEmpty(approval.ExecutionError),
		approval.Version,
		nullIfEmpty(approval.RejectedReason),
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err == nil {
		approval.ID = id
	}
	return nil
}

func (r *ApprovalRepository) GetByNo(ctx context.Context, approvalNo string) (*Approval, error) {
	return r.getOne(ctx, `SELECT * FROM approvals WHERE approval_no = ?`, approvalNo)
}

func (r *ApprovalRepository) GetByIdempotencyKey(ctx context.Context, key string) (*Approval, error) {
	return r.getOne(ctx, `SELECT * FROM approvals WHERE idempotency_key = ?`, key)
}

func (r *ApprovalRepository) ListRecent(ctx context.Context, status string, limit int) ([]Approval, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT * FROM approvals`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows := make([]approvalRow, 0)
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]Approval, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toApproval())
	}
	return result, nil
}

func (r *ApprovalRepository) UpdateWithVersion(ctx context.Context, approval *Approval, expectedVersion int) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE approvals
		 SET status = ?, approved_by = ?, approved_at = ?, executed_at = ?, execution_result = ?, execution_error = ?, rejected_reason = ?, version = version + 1
		 WHERE id = ? AND version = ?`,
		approval.Status,
		approval.ApprovedBy,
		approval.ApprovedAt,
		approval.ExecutedAt,
		nullIfEmpty(approval.ExecutionResult),
		nullIfEmpty(approval.ExecutionError),
		nullIfEmpty(approval.RejectedReason),
		approval.ID,
		expectedVersion,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return NewConflict("审批单已被并发更新，请刷新后重试")
	}
	approval.Version = expectedVersion + 1
	return nil
}

func (r *ApprovalRepository) getOne(ctx context.Context, query string, args ...any) (*Approval, error) {
	var row approvalRow
	if err := r.db.GetContext(ctx, &row, query, args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	approval := row.toApproval()
	return &approval, nil
}

type approvalRow struct {
	ID              int64          `db:"id"`
	ApprovalNo      string         `db:"approval_no"`
	IdempotencyKey  string         `db:"idempotency_key"`
	SessionID       string         `db:"session_id"`
	TraceID         string         `db:"trace_id"`
	ActionType      string         `db:"action_type"`
	TargetType      string         `db:"target_type"`
	TargetID        string         `db:"target_id"`
	Payload         string         `db:"payload"`
	Reason          string         `db:"reason"`
	Status          string         `db:"status"`
	RequestedBy     int64          `db:"requested_by"`
	ApprovedBy      sql.NullInt64  `db:"approved_by"`
	ApprovedAt      sql.NullTime   `db:"approved_at"`
	ExecutedAt      sql.NullTime   `db:"executed_at"`
	ExecutionResult sql.NullString `db:"execution_result"`
	ExecutionError  sql.NullString `db:"execution_error"`
	Version         int            `db:"version"`
	RejectedReason  sql.NullString `db:"rejected_reason"`
	CreatedAt       time.Time      `db:"created_at"`
}

func (r approvalRow) toApproval() Approval {
	var approvedBy *int64
	if r.ApprovedBy.Valid {
		value := r.ApprovedBy.Int64
		approvedBy = &value
	}
	return Approval{
		ID:              r.ID,
		ApprovalNo:      r.ApprovalNo,
		IdempotencyKey:  r.IdempotencyKey,
		SessionID:       r.SessionID,
		TraceID:         r.TraceID,
		ActionType:      r.ActionType,
		TargetType:      r.TargetType,
		TargetID:        r.TargetID,
		Payload:         r.Payload,
		Reason:          r.Reason,
		Status:          r.Status,
		RequestedBy:     r.RequestedBy,
		ApprovedBy:      approvedBy,
		ApprovedAt:      NullableTime(r.ApprovedAt),
		ExecutedAt:      NullableTime(r.ExecutedAt),
		ExecutionResult: NullableString(r.ExecutionResult),
		ExecutionError:  NullableString(r.ExecutionError),
		Version:         r.Version,
		RejectedReason:  NullableString(r.RejectedReason),
		CreatedAt:       r.CreatedAt,
	}
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
