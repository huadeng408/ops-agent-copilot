package app

import "time"

type User struct {
	ID          int64     `db:"id" json:"id"`
	Username    string    `db:"username" json:"username"`
	DisplayName string    `db:"display_name" json:"display_name"`
	Role        string    `db:"role" json:"role"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type AgentSession struct {
	ID          int64     `db:"id"`
	SessionID   string    `db:"session_id"`
	UserID      int64     `db:"user_id"`
	Summary     string    `db:"summary"`
	MemoryState string    `db:"memory_state"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type AgentMessage struct {
	ID        int64     `db:"id"`
	SessionID string    `db:"session_id"`
	Role      string    `db:"role"`
	Content   string    `db:"content"`
	TraceID   string    `db:"trace_id"`
	CreatedAt time.Time `db:"created_at"`
}

type Approval struct {
	ID              int64      `db:"id"`
	ApprovalNo      string     `db:"approval_no"`
	IdempotencyKey  string     `db:"idempotency_key"`
	SessionID       string     `db:"session_id"`
	TraceID         string     `db:"trace_id"`
	ActionType      string     `db:"action_type"`
	TargetType      string     `db:"target_type"`
	TargetID        string     `db:"target_id"`
	Payload         string     `db:"payload"`
	Reason          string     `db:"reason"`
	Status          string     `db:"status"`
	RequestedBy     int64      `db:"requested_by"`
	ApprovedBy      *int64     `db:"approved_by"`
	ApprovedAt      *time.Time `db:"approved_at"`
	ExecutedAt      *time.Time `db:"executed_at"`
	ExecutionResult string     `db:"execution_result"`
	ExecutionError  string     `db:"execution_error"`
	Version         int        `db:"version"`
	RejectedReason  string     `db:"rejected_reason"`
	CreatedAt       time.Time  `db:"created_at"`
}

type Ticket struct {
	ID          int64      `db:"id"`
	TicketNo    string     `db:"ticket_no"`
	Region      string     `db:"region"`
	Category    string     `db:"category"`
	Title       string     `db:"title"`
	Description string     `db:"description"`
	Status      string     `db:"status"`
	Priority    string     `db:"priority"`
	RootCause   string     `db:"root_cause"`
	AssigneeID  *int64     `db:"assignee_id"`
	ReporterID  *int64     `db:"reporter_id"`
	SLADeadline time.Time  `db:"sla_deadline"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	ResolvedAt  *time.Time `db:"resolved_at"`
}

type TicketComment struct {
	ID          int64     `db:"id"`
	TicketID    int64     `db:"ticket_id"`
	CommentText string    `db:"comment_text"`
	CreatedBy   int64     `db:"created_by"`
	CreatedAt   time.Time `db:"created_at"`
}

type TicketAction struct {
	ID         int64     `db:"id"`
	TicketID   int64     `db:"ticket_id"`
	ActionType string    `db:"action_type"`
	OldValue   string    `db:"old_value"`
	NewValue   string    `db:"new_value"`
	OperatorID int64     `db:"operator_id"`
	ApprovalID *int64    `db:"approval_id"`
	TraceID    string    `db:"trace_id"`
	CreatedAt  time.Time `db:"created_at"`
}

type Release struct {
	ID             int64     `db:"id"`
	ServiceName    string    `db:"service_name"`
	ReleaseVersion string    `db:"release_version"`
	ReleaseTime    time.Time `db:"release_time"`
	OperatorName   string    `db:"operator_name"`
	ChangeSummary  string    `db:"change_summary"`
	CreatedAt      time.Time `db:"created_at"`
}

type MetricRefundDaily struct {
	ID              int64     `db:"id"`
	DT              string    `db:"dt"`
	Region          string    `db:"region"`
	Category        string    `db:"category"`
	OrdersCnt       int       `db:"orders_cnt"`
	RefundOrdersCnt int       `db:"refund_orders_cnt"`
	RefundRate      float64   `db:"refund_rate"`
	GMV             float64   `db:"gmv"`
	CreatedAt       time.Time `db:"created_at"`
}

type AuditLog struct {
	ID        int64     `db:"id"`
	TraceID   string    `db:"trace_id"`
	SessionID string    `db:"session_id"`
	UserID    *int64    `db:"user_id"`
	EventType string    `db:"event_type"`
	EventData string    `db:"event_data"`
	CreatedAt time.Time `db:"created_at"`
}

type ToolCallLog struct {
	ID            int64     `db:"id"`
	TraceID       string    `db:"trace_id"`
	SessionID     string    `db:"session_id"`
	ToolName      string    `db:"tool_name"`
	ToolType      string    `db:"tool_type"`
	InputPayload  string    `db:"input_payload"`
	OutputPayload string    `db:"output_payload"`
	Success       bool      `db:"success"`
	ErrorMessage  string    `db:"error_message"`
	LatencyMS     int       `db:"latency_ms"`
	CreatedAt     time.Time `db:"created_at"`
}

type ChatRequest struct {
	SessionID   string `json:"session_id"`
	UserID      int64  `json:"user_id"`
	Message     string `json:"message"`
	RuntimeMode string `json:"runtime_mode,omitempty"`
}

type ToolCallSummary struct {
	ToolName  string `json:"tool_name"`
	Success   bool   `json:"success"`
	ToolType  string `json:"tool_type,omitempty"`
	LatencyMS int    `json:"latency_ms,omitempty"`
}

type ApprovalBrief struct {
	ApprovalNo string         `json:"approval_no"`
	ActionType string         `json:"action_type"`
	TargetID   string         `json:"target_id"`
	Payload    map[string]any `json:"payload"`
}

type ChatResponse struct {
	TraceID          string            `json:"trace_id"`
	SessionID        string            `json:"session_id"`
	Status           string            `json:"status"`
	Answer           string            `json:"answer"`
	PlanningSource   string            `json:"planning_source,omitempty"`
	PlannerLatencyMS int               `json:"planner_latency_ms,omitempty"`
	PlanCacheHit     bool              `json:"plan_cache_hit"`
	ToolCalls        []ToolCallSummary `json:"tool_calls"`
	Approval         *ApprovalBrief    `json:"approval,omitempty"`
}

type ApprovalApproveRequest struct {
	ApproverUserID int64 `json:"approver_user_id"`
}

type ApprovalRejectRequest struct {
	ApproverUserID int64  `json:"approver_user_id"`
	Reason         string `json:"reason"`
}

type ApprovalResponse struct {
	ApprovalNo      string         `json:"approval_no"`
	IdempotencyKey  string         `json:"idempotency_key,omitempty"`
	Status          string         `json:"status"`
	Version         int            `json:"version,omitempty"`
	ExecutionResult map[string]any `json:"execution_result,omitempty"`
	ExecutionError  string         `json:"execution_error,omitempty"`
	RejectedReason  string         `json:"rejected_reason,omitempty"`
}
