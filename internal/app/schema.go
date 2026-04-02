package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

func EnsureSchema(ctx context.Context, db *sqlx.DB, dialect string) error {
	statements := sqliteSchemaStatements()
	if dialect == "mysql" {
		statements = mysqlSchemaStatements()
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema failed: %w; stmt=%s", err, stmt)
		}
	}
	if err := ensureCompatibilityColumns(ctx, db, dialect); err != nil {
		return err
	}
	return nil
}

func sqliteSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS agent_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL UNIQUE,
			user_id INTEGER NOT NULL,
			summary TEXT NULL,
			memory_state TEXT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS agent_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			trace_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS metric_refund_daily (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dt DATE NOT NULL,
			region TEXT NOT NULL,
			category TEXT NOT NULL,
			orders_cnt INTEGER NOT NULL,
			refund_orders_cnt INTEGER NOT NULL,
			refund_rate REAL NOT NULL,
			gmv REAL NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(dt, region, category)
		)`,
		`CREATE TABLE IF NOT EXISTS releases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service_name TEXT NOT NULL,
			release_version TEXT NOT NULL,
			release_time DATETIME NOT NULL,
			operator_name TEXT NOT NULL,
			change_summary TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tickets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_no TEXT NOT NULL UNIQUE,
			region TEXT NOT NULL,
			category TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			priority TEXT NOT NULL,
			root_cause TEXT NULL,
			assignee_id INTEGER NULL,
			reporter_id INTEGER NULL,
			sla_deadline DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			resolved_at DATETIME NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ticket_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_id INTEGER NOT NULL,
			comment_text TEXT NOT NULL,
			created_by INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			approval_no TEXT NOT NULL UNIQUE,
			idempotency_key TEXT NOT NULL UNIQUE,
			session_id TEXT NOT NULL,
			trace_id TEXT NOT NULL,
			action_type TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			payload TEXT NOT NULL,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			requested_by INTEGER NOT NULL,
			approved_by INTEGER NULL,
			approved_at DATETIME NULL,
			executed_at DATETIME NULL,
			execution_result TEXT NULL,
			execution_error TEXT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			rejected_reason TEXT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ticket_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_id INTEGER NOT NULL,
			action_type TEXT NOT NULL,
			old_value TEXT NULL,
			new_value TEXT NULL,
			operator_id INTEGER NOT NULL,
			approval_id INTEGER NULL UNIQUE,
			trace_id TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id TEXT NOT NULL,
			session_id TEXT NULL,
			user_id INTEGER NULL,
			event_type TEXT NOT NULL,
			event_data TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tool_call_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			tool_type TEXT NOT NULL,
			input_payload TEXT NOT NULL,
			output_payload TEXT NULL,
			success INTEGER NOT NULL,
			error_message TEXT NULL,
			latency_ms INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_session_id ON agent_messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_trace_id ON agent_messages(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_metric_refund_daily_region_dt ON metric_refund_daily(region, dt)`,
		`CREATE INDEX IF NOT EXISTS idx_metric_refund_daily_category_dt ON metric_refund_daily(category, dt)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_region_status ON tickets(region, status)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_priority ON tickets(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_sla_deadline ON tickets(sla_deadline)`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status)`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_trace_id ON approvals(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_trace_id ON audit_logs(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type ON audit_logs(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_call_logs_trace_id ON tool_call_logs(trace_id)`,
		`DROP VIEW IF EXISTS v_refund_metrics_daily`,
		`CREATE VIEW IF NOT EXISTS v_refund_metrics_daily AS
			SELECT dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv FROM metric_refund_daily`,
		`DROP VIEW IF EXISTS v_ticket_sla`,
		`CREATE VIEW IF NOT EXISTS v_ticket_sla AS
			SELECT
				t.ticket_no,
				t.region,
				t.category,
				t.status,
				t.priority,
				t.root_cause,
				u.display_name AS assignee_name,
				t.sla_deadline,
				t.created_at,
				t.updated_at,
				CASE
					WHEN t.status NOT IN ('closed', 'resolved') AND CURRENT_TIMESTAMP > t.sla_deadline THEN 1
					ELSE 0
				END AS is_sla_breached
			FROM tickets t
			LEFT JOIN users u ON t.assignee_id = u.id`,
		`DROP VIEW IF EXISTS v_ticket_detail`,
		`CREATE VIEW IF NOT EXISTS v_ticket_detail AS
			SELECT
				t.ticket_no,
				t.region,
				t.category,
				t.title,
				t.description,
				t.status,
				t.priority,
				t.root_cause,
				u.display_name AS assignee_name,
				t.sla_deadline,
				t.created_at,
				t.updated_at,
				t.resolved_at
			FROM tickets t
			LEFT JOIN users u ON t.assignee_id = u.id`,
		`DROP VIEW IF EXISTS v_recent_releases`,
		`CREATE VIEW IF NOT EXISTS v_recent_releases AS
			SELECT service_name, release_version, release_time, operator_name, change_summary FROM releases`,
	}
}

func mysqlSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(64) NOT NULL UNIQUE,
			display_name VARCHAR(64) NOT NULL,
			role VARCHAR(32) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS agent_sessions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			session_id VARCHAR(64) NOT NULL UNIQUE,
			user_id BIGINT NOT NULL,
			summary TEXT NULL,
			memory_state JSON NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS agent_messages (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			session_id VARCHAR(64) NOT NULL,
			role VARCHAR(16) NOT NULL,
			content JSON NOT NULL,
			trace_id VARCHAR(64) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_agent_messages_session_id(session_id),
			INDEX idx_agent_messages_trace_id(trace_id)
		)`,
		`CREATE TABLE IF NOT EXISTS metric_refund_daily (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			dt DATE NOT NULL,
			region VARCHAR(32) NOT NULL,
			category VARCHAR(64) NOT NULL,
			orders_cnt INT NOT NULL,
			refund_orders_cnt INT NOT NULL,
			refund_rate DECIMAL(8,4) NOT NULL,
			gmv DECIMAL(18,2) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_metric_refund_daily (dt, region, category)
		)`,
		`CREATE TABLE IF NOT EXISTS releases (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			service_name VARCHAR(64) NOT NULL,
			release_version VARCHAR(64) NOT NULL,
			release_time DATETIME NOT NULL,
			operator_name VARCHAR(64) NOT NULL,
			change_summary VARCHAR(255) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tickets (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			ticket_no VARCHAR(32) NOT NULL UNIQUE,
			region VARCHAR(32) NOT NULL,
			category VARCHAR(64) NOT NULL,
			title VARCHAR(255) NOT NULL,
			description TEXT NOT NULL,
			status VARCHAR(32) NOT NULL,
			priority VARCHAR(16) NOT NULL,
			root_cause VARCHAR(64) NULL,
			assignee_id BIGINT NULL,
			reporter_id BIGINT NULL,
			sla_deadline DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			resolved_at DATETIME NULL,
			INDEX idx_tickets_region_status(region, status),
			INDEX idx_tickets_priority(priority),
			INDEX idx_tickets_sla_deadline(sla_deadline)
		)`,
		`CREATE TABLE IF NOT EXISTS ticket_comments (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			ticket_id BIGINT NOT NULL,
			comment_text TEXT NOT NULL,
			created_by BIGINT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS approvals (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			approval_no VARCHAR(32) NOT NULL UNIQUE,
			idempotency_key VARCHAR(96) NOT NULL UNIQUE,
			session_id VARCHAR(64) NOT NULL,
			trace_id VARCHAR(64) NOT NULL,
			action_type VARCHAR(32) NOT NULL,
			target_type VARCHAR(32) NOT NULL,
			target_id VARCHAR(64) NOT NULL,
			payload JSON NOT NULL,
			reason TEXT NOT NULL,
			status VARCHAR(16) NOT NULL,
			requested_by BIGINT NOT NULL,
			approved_by BIGINT NULL,
			approved_at DATETIME NULL,
			executed_at DATETIME NULL,
			execution_result JSON NULL,
			execution_error VARCHAR(255) NULL,
			version INT NOT NULL DEFAULT 1,
			rejected_reason VARCHAR(255) NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_approvals_status(status),
			INDEX idx_approvals_trace_id(trace_id)
		)`,
		`CREATE TABLE IF NOT EXISTS ticket_actions (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			ticket_id BIGINT NOT NULL,
			action_type VARCHAR(32) NOT NULL,
			old_value JSON NULL,
			new_value JSON NULL,
			operator_id BIGINT NOT NULL,
			approval_id BIGINT NULL UNIQUE,
			trace_id VARCHAR(64) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			trace_id VARCHAR(64) NOT NULL,
			session_id VARCHAR(64) NULL,
			user_id BIGINT NULL,
			event_type VARCHAR(64) NOT NULL,
			event_data JSON NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_audit_logs_trace_id(trace_id),
			INDEX idx_audit_logs_event_type(event_type)
		)`,
		`CREATE TABLE IF NOT EXISTS tool_call_logs (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			trace_id VARCHAR(64) NOT NULL,
			session_id VARCHAR(64) NOT NULL,
			tool_name VARCHAR(64) NOT NULL,
			tool_type VARCHAR(16) NOT NULL,
			input_payload JSON NOT NULL,
			output_payload JSON NULL,
			success BOOLEAN NOT NULL,
			error_message VARCHAR(255) NULL,
			latency_ms INT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_tool_call_logs_trace_id(trace_id)
		)`,
		`CREATE OR REPLACE VIEW v_refund_metrics_daily AS
			SELECT dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv FROM metric_refund_daily`,
		`CREATE OR REPLACE VIEW v_ticket_sla AS
			SELECT
				t.ticket_no,
				t.region,
				t.category,
				t.status,
				t.priority,
				t.root_cause,
				u.display_name AS assignee_name,
				t.sla_deadline,
				t.created_at,
				t.updated_at,
				CASE
					WHEN t.status NOT IN ('closed', 'resolved') AND CURRENT_TIMESTAMP > t.sla_deadline THEN 1
					ELSE 0
				END AS is_sla_breached
			FROM tickets t
			LEFT JOIN users u ON t.assignee_id = u.id`,
		`CREATE OR REPLACE VIEW v_ticket_detail AS
			SELECT
				t.ticket_no,
				t.region,
				t.category,
				t.title,
				t.description,
				t.status,
				t.priority,
				t.root_cause,
				u.display_name AS assignee_name,
				t.sla_deadline,
				t.created_at,
				t.updated_at,
				t.resolved_at
			FROM tickets t
			LEFT JOIN users u ON t.assignee_id = u.id`,
		`CREATE OR REPLACE VIEW v_recent_releases AS
			SELECT service_name, release_version, release_time, operator_name, change_summary FROM releases`,
	}
}

func ensureCompatibilityColumns(ctx context.Context, db *sqlx.DB, dialect string) error {
	switch dialect {
	case "mysql":
		return ensureMySQLColumn(ctx, db, "agent_sessions", "memory_state", "JSON NULL")
	default:
		return ensureSQLiteColumn(ctx, db, "agent_sessions", "memory_state", "TEXT NULL")
	}
}

func ensureMySQLColumn(ctx context.Context, db *sqlx.DB, tableName string, columnName string, definition string) error {
	var count int
	if err := db.GetContext(
		ctx,
		&count,
		`SELECT COUNT(*)
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		tableName,
		columnName,
	); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, definition))
	if err != nil {
		return fmt.Errorf("ensure mysql column failed: %w", err)
	}
	return nil
}

func ensureSQLiteColumn(ctx context.Context, db *sqlx.DB, tableName string, columnName string, definition string) error {
	rows, err := db.QueryxContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var row struct {
			CID       int    `db:"cid"`
			Name      string `db:"name"`
			Type      string `db:"type"`
			NotNull   int    `db:"notnull"`
			DfltValue any    `db:"dflt_value"`
			PK        int    `db:"pk"`
		}
		if err := rows.StructScan(&row); err != nil {
			return err
		}
		if row.Name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, definition))
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("ensure sqlite column failed: %w", err)
	}
	return nil
}
