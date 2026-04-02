package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type AgentService struct {
	cfg             Config
	sessionRepo     *SessionRepository
	auditService    *AuditService
	memoryService   *MemoryService
	toolRegistry    *ToolRegistry
	approvalService *ApprovalService
	reportService   *ReportService
	planner         *PlannerService
	metrics         *MetricsRecorder
	baseToolContext ToolContext
}

func NewAgentService(
	cfg Config,
	sessionRepo *SessionRepository,
	auditService *AuditService,
	memoryService *MemoryService,
	toolRegistry *ToolRegistry,
	approvalService *ApprovalService,
	reportService *ReportService,
	planner *PlannerService,
	metrics *MetricsRecorder,
	baseToolContext ToolContext,
) *AgentService {
	return &AgentService{
		cfg:             cfg,
		sessionRepo:     sessionRepo,
		auditService:    auditService,
		memoryService:   memoryService,
		toolRegistry:    toolRegistry,
		approvalService: approvalService,
		reportService:   reportService,
		planner:         planner,
		metrics:         metrics,
		baseToolContext: baseToolContext,
	}
}

func (s *AgentService) HandleChat(ctx context.Context, sessionID string, user User, message string) (ChatResponse, error) {
	ctx, span := StartSpan(ctx, "agent.handle_chat")
	defer span.End()

	started := time.Now()
	traceID := BusinessTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = "tr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	ctx = ContextWithTraceID(ctx, traceID)
	span.SetAttributes(
		attribute.String("app.trace_id", traceID),
		attribute.String("session.id", sessionID),
		attribute.Int64("user.id", user.ID),
		attribute.Int("chat.message_length", len([]rune(message))),
	)

	if _, err := s.sessionRepo.GetOrCreate(ctx, sessionID, user.ID); err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	if err := s.sessionRepo.AddMessage(ctx, sessionID, "user", map[string]any{"text": message}, traceID); err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	if err := s.auditService.LogEvent(ctx, traceID, sessionID, &user.ID, "chat_received", map[string]any{"message": message}); err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}

	memory, err := s.memoryService.BuildContext(ctx, sessionID)
	if err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	planning, err := s.planner.Plan(ctx, message, memory)
	if err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	span.SetAttributes(
		attribute.String("planner.source", planning.Source),
		attribute.Int("planner.latency_ms", planning.LatencyMS),
		attribute.Bool("planner.cache_hit", planning.CacheHit),
		attribute.Int("planner.call_count", len(planning.Calls)),
	)

	if len(planning.Calls) == 1 && planning.Calls[0].ToolName == "generate_report" {
		report, err := s.reportService.GenerateDailyReport(ctx)
		if err != nil {
			RecordSpanError(span, err)
			return ChatResponse{}, err
		}
		answer := asString(report["content"])
		if err := s.sessionRepo.AddMessage(ctx, sessionID, "assistant", map[string]any{"text": answer}, traceID); err != nil {
			RecordSpanError(span, err)
			return ChatResponse{}, err
		}
		_ = s.memoryService.RememberTurn(ctx, sessionID, message, planning.Calls)
		_ = s.memoryService.MaybeUpdateSummary(ctx, sessionID)
		_ = s.auditService.LogEvent(ctx, traceID, sessionID, &user.ID, "response_returned", map[string]any{"status": "completed"})

		latencyMS := int(time.Since(started).Milliseconds())
		s.metrics.RecordChat("completed", latencyMS)
		span.SetAttributes(
			attribute.String("chat.status", "completed"),
			attribute.Int("chat.latency_ms", latencyMS),
		)
		return ChatResponse{
			TraceID:          traceID,
			SessionID:        sessionID,
			Status:           "completed",
			Answer:           answer,
			PlanningSource:   planning.Source,
			PlannerLatencyMS: planning.LatencyMS,
			PlanCacheHit:     planning.CacheHit,
			ToolCalls:        []ToolCallSummary{},
		}, nil
	}

	answers := make([]string, 0, len(planning.Calls))
	toolCalls := make([]ToolCallSummary, 0, len(planning.Calls))
	status := "completed"
	var approvalBrief *ApprovalBrief

	for _, plannedCall := range planning.Calls {
		result, record, err := s.toolRegistry.Invoke(ctx, plannedCall.ToolName, s.toolContext(traceID, sessionID, user), plannedCall.Arguments)
		toolCalls = append(toolCalls, ToolCallSummary{
			ToolName:  record.ToolName,
			Success:   record.Success,
			ToolType:  record.ToolType,
			LatencyMS: record.LatencyMS,
		})
		if err != nil {
			RecordSpanError(span, err)
			return ChatResponse{}, err
		}
		if result.RequiresApproval {
			proposal, err := s.approvalService.CreateProposal(
				ctx,
				sessionID,
				traceID,
				user,
				asString(result.Data["action_type"]),
				asString(result.Data["target_type"]),
				asString(result.Data["target_id"]),
				mapValue(result.Data["payload"]),
				asString(result.Data["reason"]),
			)
			if err != nil {
				RecordSpanError(span, err)
				return ChatResponse{}, err
			}
			approvalBrief = &ApprovalBrief{
				ApprovalNo: proposal.ApprovalNo,
				ActionType: proposal.ActionType,
				TargetID:   proposal.TargetID,
				Payload:    ParseJSONMap(proposal.Payload),
			}
			answers = append(answers, "已生成写操作 proposal，需审批后执行。")
			status = "approval_required"
			span.AddEvent("approval_required")
			break
		}
		answers = append(answers, renderToolAnswer(plannedCall.ToolName, result.Data))
	}

	finalAnswer := strings.TrimSpace(strings.Join(answers, "\n\n"))
	if finalAnswer == "" {
		finalAnswer = "已处理请求。"
	}
	if err := s.sessionRepo.AddMessage(ctx, sessionID, "assistant", map[string]any{"text": finalAnswer}, traceID); err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	_ = s.memoryService.RememberTurn(ctx, sessionID, message, planning.Calls)
	_ = s.memoryService.MaybeUpdateSummary(ctx, sessionID)
	_ = s.auditService.LogEvent(ctx, traceID, sessionID, &user.ID, "response_returned", map[string]any{"status": status})

	latencyMS := int(time.Since(started).Milliseconds())
	s.metrics.RecordChat(status, latencyMS)
	span.SetAttributes(
		attribute.String("chat.status", status),
		attribute.Int("chat.latency_ms", latencyMS),
		attribute.Int("chat.tool_call_count", len(toolCalls)),
	)
	return ChatResponse{
		TraceID:          traceID,
		SessionID:        sessionID,
		Status:           status,
		Answer:           finalAnswer,
		PlanningSource:   planning.Source,
		PlannerLatencyMS: planning.LatencyMS,
		PlanCacheHit:     planning.CacheHit,
		ToolCalls:        toolCalls,
		Approval:         approvalBrief,
	}, nil
}

func (s *AgentService) toolContext(traceID string, sessionID string, user User) ToolContext {
	return ToolContext{
		TraceID:        traceID,
		SessionID:      sessionID,
		User:           user,
		MetricRepo:     s.baseToolContext.MetricRepo,
		TicketRepo:     s.baseToolContext.TicketRepo,
		ReleaseRepo:    s.baseToolContext.ReleaseRepo,
		Verifier:       s.baseToolContext.Verifier,
		AnomalyService: s.baseToolContext.AnomalyService,
		DB:             s.baseToolContext.DB,
		Config:         s.baseToolContext.Config,
	}
}

func renderToolAnswer(toolName string, data map[string]any) string {
	switch toolName {
	case "query_refund_metrics":
		rows := sliceValue(data["rows"])
		if len(rows) == 0 {
			return "没有查到符合条件的退款率数据。"
		}
		sort.Slice(rows, func(i int, j int) bool {
			return asFloat(rows[i]["refund_rate"]) > asFloat(rows[j]["refund_rate"])
		})
		lines := []string{"退款率查询结果："}
		for _, row := range rows[:min(len(rows), 5)] {
			lines = append(lines, fmt.Sprintf(
				"- %s %s-%s 退款率 %.2f%%，退款单量 %d",
				asString(row["dt"]),
				asString(row["region"]),
				asString(row["category"]),
				asFloat(row["refund_rate"])*100,
				asInt(row["refund_orders_cnt"], 0),
			))
		}
		return strings.Join(lines, "\n")

	case "find_refund_anomalies":
		rows := sliceValue(data["rows"])
		if len(rows) == 0 {
			return "未识别到明显的退款异常类目。"
		}
		lines := []string{"退款异常类目："}
		for _, row := range rows {
			lines = append(lines, fmt.Sprintf(
				"- %s 平均退款率 %.2f%%，退款单量 %d",
				asString(row["category"]),
				asFloat(row["avg_refund_rate"])*100,
				asInt(row["refund_orders_cnt"], 0),
			))
		}
		return strings.Join(lines, "\n")

	case "analyze_operational_anomaly":
		lines := []string{
			fmt.Sprintf(
				"异常归因分析（日维度：%s，区域：%s）",
				asString(data["target_date"]),
				defaultString(asString(data["region"]), "全部"),
			),
			"",
			"1. 退款异常",
		}
		spikes := sliceValue(data["refund_spikes"])
		if len(spikes) == 0 {
			lines = append(lines, "- 未识别到明显退款率突增类目")
		} else {
			for _, item := range spikes[:min(len(spikes), 3)] {
				lines = append(lines, fmt.Sprintf(
					"- %s-%s 当日退款率 %.2f%%，较近 7 天基线提升 %.2f%%",
					asString(item["region"]),
					asString(item["category"]),
					asFloat(item["target_refund_rate"])*100,
					asFloat(item["delta_refund_rate"])*100,
				))
			}
		}

		lines = append(lines, "", "2. 超 SLA 工单")
		rootCauses := sliceValue(data["sla_by_root_cause"])
		if len(rootCauses) == 0 {
			lines = append(lines, "- 未查询到超 SLA 工单")
		} else {
			for _, item := range rootCauses[:min(len(rootCauses), 4)] {
				lines = append(lines, fmt.Sprintf(
					"- %s：%d 单",
					defaultString(asString(item["group_key"]), "未归类"),
					asInt(item["ticket_count"], 0),
				))
			}
		}

		lines = append(lines, "", "3. 窗口内发布记录")
		releases := sliceValue(data["nearby_releases"])
		if len(releases) == 0 {
			lines = append(lines, "- 异常窗口附近没有发布记录")
		} else {
			for _, item := range releases[:min(len(releases), 3)] {
				lines = append(lines, fmt.Sprintf(
					"- %s | %s %s | %s",
					asString(item["release_time"]),
					asString(item["service_name"]),
					asString(item["release_version"]),
					asString(item["change_summary"]),
				))
			}
		}

		lines = append(lines, "", "4. 半自动归因结论")
		correlation := mapValue(data["correlation"])
		suspectedCauses := sliceValue(correlation["suspected_causes"])
		if len(suspectedCauses) == 0 {
			lines = append(lines, "- 当前证据不足，建议缩小时间窗口或补充更多上下文")
		} else {
			for _, item := range suspectedCauses {
				evidence := strings.Join(stringSliceValue(item["evidence"]), "；")
				lines = append(lines, fmt.Sprintf(
					"- %s（置信度：%s，证据：%s）",
					asString(item["cause"]),
					asString(item["confidence"]),
					evidence,
				))
			}
		}
		return strings.Join(lines, "\n")

	case "list_sla_breached_tickets":
		rows := sliceValue(data["rows"])
		if len(rows) == 0 {
			return "当前没有查到超 SLA 工单。"
		}
		if _, ok := rows[0]["group_key"]; ok {
			lines := []string{"超 SLA 工单分类结果："}
			for _, row := range rows {
				lines = append(lines, fmt.Sprintf(
					"- %s：%d 单",
					defaultString(asString(row["group_key"]), "未归类"),
					asInt(row["ticket_count"], 0),
				))
			}
			return strings.Join(lines, "\n")
		}
		lines := []string{"超 SLA 工单列表："}
		for _, row := range rows[:min(len(rows), 10)] {
			lines = append(lines, fmt.Sprintf(
				"- %s | %s | %s | %s",
				asString(row["ticket_no"]),
				asString(row["priority"]),
				asString(row["region"]),
				defaultString(asString(row["root_cause"]), "待定"),
			))
		}
		return strings.Join(lines, "\n")

	case "get_ticket_detail":
		return fmt.Sprintf(
			"工单 %s 详情：\n- 标题：%s\n- 区域：%s | 类目：%s\n- 状态：%s | 优先级：%s\n- 当前处理人：%s\n- 根因：%s\n- 描述：%s",
			asString(data["ticket_no"]),
			asString(data["title"]),
			asString(data["region"]),
			asString(data["category"]),
			asString(data["status"]),
			asString(data["priority"]),
			defaultString(asString(data["assignee_name"]), "未分派"),
			defaultString(asString(data["root_cause"]), "待定"),
			asString(data["description"]),
		)

	case "get_ticket_comments":
		lines := []string{"最近操作记录："}
		actions := sliceValue(data["actions"])
		for _, row := range actions[:min(len(actions), 5)] {
			lines = append(lines, fmt.Sprintf(
				"- %s | %s | %v",
				asString(row["created_at"]),
				asString(row["action_type"]),
				row["new_value"],
			))
		}
		comments := sliceValue(data["comments"])
		if len(comments) > 0 {
			lines = append(lines, "最近备注：")
			for _, row := range comments[:min(len(comments), 5)] {
				lines = append(lines, fmt.Sprintf(
					"- %s | %s：%s",
					asString(row["created_at"]),
					defaultString(asString(row["created_by"]), "未知"),
					asString(row["comment_text"]),
				))
			}
		}
		return strings.Join(lines, "\n")

	case "get_recent_releases":
		rows := sliceValue(data["rows"])
		if len(rows) == 0 {
			return "最近没有发布记录。"
		}
		lines := []string{"最近发布记录："}
		for _, row := range rows[:min(len(rows), 5)] {
			lines = append(lines, fmt.Sprintf(
				"- %s | %s %s | %s",
				asString(row["release_time"]),
				asString(row["service_name"]),
				asString(row["release_version"]),
				asString(row["change_summary"]),
			))
		}
		return strings.Join(lines, "\n")

	case "run_readonly_sql":
		return fmt.Sprintf("SQL 查询完成，共返回 %d 行。", len(sliceValue(data["rows"])))

	default:
		return "工具执行完成。"
	}
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return map[string]any{}
	}
}

func sliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				result = append(result, mapped)
			}
		}
		return result
	default:
		return []map[string]any{}
	}
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, asString(item))
		}
		return result
	default:
		return []string{}
	}
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
