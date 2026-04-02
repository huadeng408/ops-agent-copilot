package app

import (
	"context"
	"database/sql"
	"time"
)

type SessionRepository struct {
	db DBTX
}

func NewSessionRepository(db DBTX) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) GetSession(ctx context.Context, sessionID string) (*AgentSession, error) {
	var row struct {
		ID          int64          `db:"id"`
		SessionID   string         `db:"session_id"`
		UserID      int64          `db:"user_id"`
		Summary     sql.NullString `db:"summary"`
		MemoryState sql.NullString `db:"memory_state"`
		CreatedAt   time.Time      `db:"created_at"`
		UpdatedAt   time.Time      `db:"updated_at"`
	}
	err := r.db.GetContext(ctx, &row, `SELECT id, session_id, user_id, summary, memory_state, created_at, updated_at FROM agent_sessions WHERE session_id = ?`, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &AgentSession{
		ID:          row.ID,
		SessionID:   row.SessionID,
		UserID:      row.UserID,
		Summary:     row.Summary.String,
		MemoryState: row.MemoryState.String,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

func (r *SessionRepository) GetOrCreate(ctx context.Context, sessionID string, userID int64) (*AgentSession, error) {
	item, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if item != nil {
		return item, nil
	}
	if _, err := r.db.ExecContext(
		ctx,
		`INSERT INTO agent_sessions (session_id, user_id, summary, memory_state) VALUES (?, ?, NULL, ?)`,
		sessionID,
		userID,
		MustJSON(map[string]any{}),
	); err != nil {
		return nil, err
	}
	return r.GetSession(ctx, sessionID)
}

func (r *SessionRepository) AddMessage(ctx context.Context, sessionID string, role string, content map[string]any, traceID string) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO agent_messages (session_id, role, content, trace_id) VALUES (?, ?, ?, ?)`,
		sessionID,
		role,
		MustJSON(content),
		traceID,
	)
	return err
}

func (r *SessionRepository) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]AgentMessage, error) {
	if limit <= 0 {
		limit = 8
	}
	rows := make([]AgentMessage, 0)
	if err := r.db.SelectContext(
		ctx,
		&rows,
		`SELECT id, session_id, role, content, trace_id, created_at
		 FROM agent_messages
		 WHERE session_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		sessionID,
		limit,
	); err != nil {
		return nil, err
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, nil
}

func (r *SessionRepository) UpdateSummary(ctx context.Context, sessionID string, summary string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agent_sessions SET summary = ?, updated_at = CURRENT_TIMESTAMP WHERE session_id = ?`, summary, sessionID)
	return err
}

func (r *SessionRepository) UpdateMemoryState(ctx context.Context, sessionID string, memoryState map[string]any) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE agent_sessions SET memory_state = ?, updated_at = CURRENT_TIMESTAMP WHERE session_id = ?`,
		MustJSON(memoryState),
		sessionID,
	)
	return err
}
