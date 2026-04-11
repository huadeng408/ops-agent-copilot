package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

type ToolRegistry struct {
	auditService *AuditService
	metrics      *MetricsRecorder
	tools        map[string]Tool
}

func NewToolRegistry(auditService *AuditService, metrics *MetricsRecorder) *ToolRegistry {
	return &ToolRegistry{
		auditService: auditService,
		metrics:      metrics,
		tools:        make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Schema().Name] = tool
}

func (r *ToolRegistry) ListSchemas() []ToolSchema {
	result := make([]ToolSchema, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool.Schema())
	}
	return result
}

func (r *ToolRegistry) SchemaByName(name string) (ToolSchema, bool) {
	tool, ok := r.tools[name]
	if !ok {
		return ToolSchema{}, false
	}
	return tool.Schema(), true
}

func (r *ToolRegistry) ToolTypeByName(name string) (string, bool) {
	tool, ok := r.tools[name]
	if !ok {
		return "", false
	}
	return tool.ToolType(), true
}

func (r *ToolRegistry) Invoke(ctx context.Context, name string, toolContext ToolContext, arguments map[string]any) (ToolResult, ToolExecutionRecord, error) {
	tool, ok := r.tools[name]
	if !ok {
		return ToolResult{}, ToolExecutionRecord{}, NewValidation("未知工具: " + name)
	}
	ctx, span := StartSpan(ctx, "tool."+name)
	defer span.End()
	span.SetAttributes(
		attribute.String("tool.name", name),
		attribute.String("tool.type", tool.ToolType()),
	)
	started := time.Now()
	result, err := tool.Execute(ctx, toolContext, arguments)
	latencyMS := int(time.Since(started).Milliseconds())
	span.SetAttributes(
		attribute.Int("tool.latency_ms", latencyMS),
		attribute.Bool("tool.success", err == nil),
	)
	record := ToolExecutionRecord{
		ToolName:  name,
		ToolType:  tool.ToolType(),
		Success:   err == nil,
		LatencyMS: latencyMS,
	}
	if err == nil {
		record.Output = result.Data
		r.metrics.RecordToolCall(name, true, latencyMS)
		_ = r.auditService.LogToolCall(ctx, ToolCallLog{
			TraceID:       toolContext.TraceID,
			SessionID:     toolContext.SessionID,
			ToolName:      name,
			ToolType:      tool.ToolType(),
			InputPayload:  MustJSON(arguments),
			OutputPayload: MustJSON(result.Data),
			Success:       true,
			LatencyMS:     latencyMS,
		})
		_ = r.auditService.LogEvent(ctx, toolContext.TraceID, toolContext.SessionID, &toolContext.User.ID, "tool_called", map[string]any{
			"tool_name": name,
			"tool_type": tool.ToolType(),
		})
		return result, record, nil
	}
	RecordSpanError(span, err)
	record.ErrorMessage = err.Error()
	r.metrics.RecordToolCall(name, false, latencyMS)
	_ = r.auditService.LogToolCall(ctx, ToolCallLog{
		TraceID:      toolContext.TraceID,
		SessionID:    toolContext.SessionID,
		ToolName:     name,
		ToolType:     tool.ToolType(),
		InputPayload: MustJSON(arguments),
		Success:      false,
		ErrorMessage: err.Error(),
		LatencyMS:    latencyMS,
	})
	_ = r.auditService.LogEvent(ctx, toolContext.TraceID, toolContext.SessionID, &toolContext.User.ID, "tool_failed", map[string]any{
		"tool_name":     name,
		"error_message": err.Error(),
	})
	return ToolResult{}, record, err
}

type queryRefundMetricsTool struct{}

func (t queryRefundMetricsTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "query_refund_metrics",
		Description: "查询退款率指标",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start_date": map[string]any{"type": "string"},
				"end_date":   map[string]any{"type": "string"},
				"region":     map[string]any{"type": "string", "enum": regions},
				"category":   map[string]any{"type": "string", "enum": categories},
			},
			"required": []string{"start_date", "end_date"},
		},
	}
}

func (t queryRefundMetricsTool) ToolType() string { return "readonly" }

func (t queryRefundMetricsTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	rows, err := toolContext.MetricRepo.QueryRefundMetrics(
		ctx,
		asString(arguments["start_date"]),
		asString(arguments["end_date"]),
		asString(arguments["region"]),
		asString(arguments["category"]),
	)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: map[string]any{"rows": rows}, Message: fmt.Sprintf("查询到 %d 条退款指标", len(rows))}, nil
}

type findRefundAnomaliesTool struct{}

func (t findRefundAnomaliesTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "find_refund_anomalies",
		Description: "找出退款率异常的类目或区域",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start_date": map[string]any{"type": "string"},
				"end_date":   map[string]any{"type": "string"},
				"region":     map[string]any{"type": "string", "enum": regions},
				"top_k":      map[string]any{"type": "integer", "default": 5},
			},
			"required": []string{"start_date", "end_date"},
		},
	}
}

func (t findRefundAnomaliesTool) ToolType() string { return "readonly" }

func (t findRefundAnomaliesTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	rows, err := toolContext.MetricRepo.FindRefundAnomalies(
		ctx,
		asString(arguments["start_date"]),
		asString(arguments["end_date"]),
		asString(arguments["region"]),
		asInt(arguments["top_k"], 5),
	)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: map[string]any{"rows": rows}, Message: fmt.Sprintf("识别到 %d 个高退款率类目", len(rows))}, nil
}

type listSLABreachedTicketsTool struct{}

func (t listSLABreachedTicketsTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "list_sla_breached_tickets",
		Description: "列出超 SLA 工单",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region":   map[string]any{"type": "string", "enum": regions},
				"date":     map[string]any{"type": "string"},
				"group_by": map[string]any{"type": "string", "enum": []string{"root_cause", "priority", "category", "assignee_name"}},
			},
			"required": []string{"date"},
		},
	}
}

func (t listSLABreachedTicketsTool) ToolType() string { return "readonly" }

func (t listSLABreachedTicketsTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	rows, err := toolContext.TicketRepo.ListSLABreachedTickets(
		ctx,
		asString(arguments["date"]),
		asString(arguments["region"]),
		asString(arguments["group_by"]),
	)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: map[string]any{"rows": rows}, Message: fmt.Sprintf("查询到 %d 条超 SLA 结果", len(rows))}, nil
}

type getTicketDetailTool struct{}

func (t getTicketDetailTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "get_ticket_detail",
		Description: "查询工单详情",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket_no": map[string]any{"type": "string"},
			},
			"required": []string{"ticket_no"},
		},
	}
}

func (t getTicketDetailTool) ToolType() string { return "readonly" }

func (t getTicketDetailTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	detail, err := toolContext.TicketRepo.GetTicketDetail(ctx, asString(arguments["ticket_no"]))
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: detail, Message: "已查询工单详情"}, nil
}

type getTicketCommentsTool struct{}

func (t getTicketCommentsTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "get_ticket_comments",
		Description: "查询工单备注和最近操作",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket_no": map[string]any{"type": "string"},
			},
			"required": []string{"ticket_no"},
		},
	}
}

func (t getTicketCommentsTool) ToolType() string { return "readonly" }

func (t getTicketCommentsTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	ticketNo := asString(arguments["ticket_no"])
	comments, err := toolContext.TicketRepo.GetTicketComments(ctx, ticketNo, 10)
	if err != nil {
		return ToolResult{}, err
	}
	actions, err := toolContext.TicketRepo.GetRecentTicketActions(ctx, ticketNo, 10)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: map[string]any{"comments": comments, "actions": actions}, Message: "已查询工单备注与最近操作"}, nil
}

type getRecentReleasesTool struct{}

func (t getRecentReleasesTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "get_recent_releases",
		Description: "查询最近发布记录",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}
}

func (t getRecentReleasesTool) ToolType() string { return "readonly" }

func (t getRecentReleasesTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	rows, err := toolContext.ReleaseRepo.GetRecentReleases(ctx, asInt(arguments["limit"], 10))
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: map[string]any{"rows": rows}, Message: fmt.Sprintf("查询到 %d 条发布记录", len(rows))}, nil
}

type analyzeOperationalAnomalyTool struct{}

func (t analyzeOperationalAnomalyTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "analyze_operational_anomaly",
		Description: "关联退款异常、超SLA工单与最近发布记录，输出半自动归因结论",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":   map[string]any{"type": "string"},
				"region": map[string]any{"type": "string", "enum": regions},
			},
			"required": []string{"date"},
		},
	}
}

func (t analyzeOperationalAnomalyTool) ToolType() string { return "readonly" }

func (t analyzeOperationalAnomalyTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	if toolContext.AnomalyService == nil {
		return ToolResult{}, NewValidation("anomaly_service 未配置")
	}
	analysis, err := toolContext.AnomalyService.AnalyzeOperationalAnomaly(ctx, asString(arguments["date"]), asString(arguments["region"]))
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Data: analysis, Message: "已完成异常归因分析"}, nil
}

type runReadonlySQLTool struct {
	guard *SQLGuard
}

func (t runReadonlySQLTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        "run_readonly_sql",
		Description: "执行受限只读 SQL，仅允许 SELECT 白名单视图",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{"type": "string"},
			},
			"required": []string{"sql"},
		},
	}
}

func (t runReadonlySQLTool) ToolType() string { return "readonly" }

func (t runReadonlySQLTool) Execute(ctx context.Context, toolContext ToolContext, arguments map[string]any) (ToolResult, error) {
	sqlText := asString(arguments["sql"])
	verification := t.guard.Validate(sqlText, toolContext.Config.ReadonlySQLLimit)
	if !verification.Passed {
		return ToolResult{}, NewValidation(verification.Message)
	}
	rows, err := toolContext.DB.QueryxContext(ctx, sqlText)
	if err != nil {
		return ToolResult{}, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		item := make(map[string]any)
		if err := rows.MapScan(item); err != nil {
			return ToolResult{}, err
		}
		clean := make(map[string]any, len(item))
		for key, value := range item {
			switch typed := value.(type) {
			case []byte:
				clean[key] = string(typed)
			default:
				clean[key] = typed
			}
		}
		result = append(result, clean)
	}
	if verifier := toolContext.Verifier.VerifyResultSize(result); !verifier.Passed {
		return ToolResult{}, NewValidation(verifier.Message)
	}
	return ToolResult{Data: map[string]any{"rows": result}, Message: fmt.Sprintf("SQL 返回 %d 行结果", len(result))}, nil
}

type proposalTool struct {
	name        string
	description string
	parameters  map[string]any
	actionType  string
}

func (t proposalTool) Schema() ToolSchema {
	return ToolSchema{Name: t.name, Description: t.description, Parameters: t.parameters}
}

func (t proposalTool) ToolType() string { return "write" }

func (t proposalTool) Execute(_ context.Context, _ ToolContext, arguments map[string]any) (ToolResult, error) {
	return ToolResult{
		Data: map[string]any{
			"action_type": t.actionType,
			"target_type": "ticket",
			"target_id":   asString(arguments["ticket_no"]),
			"payload":     arguments,
			"reason":      asString(arguments["reason"]),
		},
		Message:          "已生成 proposal",
		RequiresApproval: true,
	}, nil
}

func BuildDefaultToolRegistry(auditService *AuditService, metrics *MetricsRecorder) *ToolRegistry {
	registry := NewToolRegistry(auditService, metrics)
	registry.Register(queryRefundMetricsTool{})
	registry.Register(findRefundAnomaliesTool{})
	registry.Register(analyzeOperationalAnomalyTool{})
	registry.Register(listSLABreachedTicketsTool{})
	registry.Register(getTicketDetailTool{})
	registry.Register(getTicketCommentsTool{})
	registry.Register(getRecentReleasesTool{})
	registry.Register(runReadonlySQLTool{guard: NewSQLGuard()})
	registry.Register(proposalTool{
		name:        "propose_assign_ticket",
		description: "提出工单分派建议，不直接执行",
		actionType:  "assign_ticket",
		parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket_no":     map[string]any{"type": "string"},
				"assignee_name": map[string]any{"type": "string"},
				"reason":        map[string]any{"type": "string"},
			},
			"required": []string{"ticket_no", "assignee_name", "reason"},
		},
	})
	registry.Register(proposalTool{
		name:        "propose_add_ticket_comment",
		description: "提出工单备注写入建议，不直接执行",
		actionType:  "add_ticket_comment",
		parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket_no":    map[string]any{"type": "string"},
				"comment_text": map[string]any{"type": "string"},
				"reason":       map[string]any{"type": "string"},
			},
			"required": []string{"ticket_no", "comment_text", "reason"},
		},
	})
	registry.Register(proposalTool{
		name:        "propose_escalate_ticket",
		description: "提出升级工单优先级建议，不直接执行",
		actionType:  "escalate_ticket",
		parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket_no":    map[string]any{"type": "string"},
				"new_priority": map[string]any{"type": "string", "enum": []string{"P1", "P2", "P3"}},
				"reason":       map[string]any{"type": "string"},
			},
			"required": []string{"ticket_no", "new_priority", "reason"},
		},
	})
	return registry
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func asInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}
