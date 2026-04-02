package app

import "context"

type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolExecutionRecord struct {
	ToolName     string         `json:"tool_name"`
	ToolType     string         `json:"tool_type"`
	Success      bool           `json:"success"`
	LatencyMS    int            `json:"latency_ms"`
	Output       map[string]any `json:"output,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

type VerifierResult struct {
	Passed   bool   `json:"passed"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ToolContext struct {
	TraceID        string
	SessionID      string
	User           User
	MetricRepo     *MetricRepository
	TicketRepo     *TicketRepository
	ReleaseRepo    *ReleaseRepository
	Verifier       *VerifierService
	AnomalyService *AnomalyService
	DB             DBTX
	Config         Config
}

type ToolResult struct {
	Data             map[string]any
	Message          string
	RequiresApproval bool
}

type Tool interface {
	Schema() ToolSchema
	ToolType() string
	Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error)
}
