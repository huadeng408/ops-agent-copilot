package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

const plannerSystemPrompt = `你是企业运营 Copilot 的工具规划器。
你的唯一任务是选择工具，不要直接回答业务结论。

硬性规则：
1. 第一条回复必须是 function call，不要输出自然语言，不要解释，不要道歉，不要追问。
2. 如果提供了 Heuristic tool hints，优先复用第一条 hint 的 tool_name 和 arguments；除非它与最新用户请求明显冲突，否则直接照抄。
3. 优先只调用一个工具；只有用户明确要求组合归因时才调用 analyze_operational_anomaly。
4. 对写请求只能调用 propose_* 工具，绝不能直接执行真实业务写入。
5. 参数键名必须严格匹配工具 schema；不要添加额外字段；枚举值必须使用 schema 中的原值。
6. 只有用户明确提到“日报”“周报”“report”时，才能调用 generate_report。
7. 如果 heuristic hint 已经给出了所需参数，不允许再追问用户。
8. 绝不要返回空回复。`

const ollamaPlannerSystemPrompt = `You are a tool router for an operations copilot.
Return a tool call only. Never answer with prose.

Rules:
1. Your first response must be a function call.
2. If the conversation includes "Preferred tool call", call that tool with those arguments unless it clearly contradicts the latest user request.
3. If heuristic hints are present, prefer the first hint.
4. Do not ask follow-up questions when the preferred tool call already includes arguments.
5. Use exactly the tool name and argument keys from the schema.
6. Prefer one tool call.`

type AuditService struct {
	repo *AuditRepository
}

func NewAuditService(repo *AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) LogEvent(ctx context.Context, traceID string, sessionID string, userID *int64, eventType string, eventData map[string]any) error {
	return s.repo.CreateAuditLog(ctx, AuditLog{
		TraceID:   traceID,
		SessionID: sessionID,
		UserID:    userID,
		EventType: eventType,
		EventData: MustJSON(eventData),
	})
}

func (s *AuditService) LogToolCall(ctx context.Context, log ToolCallLog) error {
	return s.repo.CreateToolCallLog(ctx, log)
}

type VerifierService struct {
	ticketRepo *TicketRepository
	metrics    *MetricsRecorder
	guard      *SQLGuard
}

func NewVerifierService(ticketRepo *TicketRepository, metrics *MetricsRecorder, guard *SQLGuard) *VerifierService {
	return &VerifierService{ticketRepo: ticketRepo, metrics: metrics, guard: guard}
}

func (s *VerifierService) VerifySQL(sqlText string, maxLimit int) VerifierResult {
	result := s.guard.Validate(sqlText, maxLimit)
	if !result.Passed {
		s.metrics.RecordVerifierRejection("sql")
	}
	return VerifierResult(result)
}

func (s *VerifierService) VerifyResultSize(rows []map[string]any) VerifierResult {
	if len(rows) == 0 {
		s.metrics.RecordVerifierRejection("result_size")
		return VerifierResult{Passed: false, Severity: "warn", Message: "结果为空，请缩小范围或补充条件"}
	}
	if len(rows) > 200 {
		s.metrics.RecordVerifierRejection("result_size")
		return VerifierResult{Passed: false, Severity: "error", Message: "结果过大，请缩小时间范围或增加过滤条件"}
	}
	return VerifierResult{Passed: true, Severity: "info", Message: "结果规模正常"}
}

func (s *VerifierService) VerifyProposal(ctx context.Context, actionType string, payload map[string]any, reason string, currentUser User) (VerifierResult, error) {
	if err := EnsureCanSubmitWrite(currentUser); err != nil {
		s.metrics.RecordVerifierRejection("proposal")
		return VerifierResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		s.metrics.RecordVerifierRejection("proposal")
		return VerifierResult{}, NewValidation("proposal reason 不能为空")
	}
	ticketNo := asString(payload["ticket_no"])
	if strings.TrimSpace(ticketNo) == "" {
		s.metrics.RecordVerifierRejection("proposal")
		return VerifierResult{}, NewValidation("proposal payload 缺少 ticket_no")
	}
	if _, err := s.ticketRepo.GetTicketByNo(ctx, ticketNo); err != nil {
		s.metrics.RecordVerifierRejection("proposal")
		return VerifierResult{}, err
	}
	switch actionType {
	case "assign_ticket":
		if strings.TrimSpace(asString(payload["assignee_name"])) == "" {
			s.metrics.RecordVerifierRejection("proposal")
			return VerifierResult{}, NewValidation("分派 proposal 缺少 assignee_name")
		}
	case "escalate_ticket":
		switch asString(payload["new_priority"]) {
		case "P1", "P2", "P3":
		default:
			s.metrics.RecordVerifierRejection("proposal")
			return VerifierResult{}, NewValidation("优先级不合法")
		}
	case "add_ticket_comment":
		if strings.TrimSpace(asString(payload["comment_text"])) == "" {
			s.metrics.RecordVerifierRejection("proposal")
			return VerifierResult{}, NewValidation("备注内容不能为空")
		}
	}
	return VerifierResult{Passed: true, Severity: "info", Message: "proposal 校验通过"}, nil
}

type MemoryService struct {
	sessionRepo            *SessionRepository
	cache                  *CacheService
	keepRecentMessageCount int
}

func NewMemoryService(sessionRepo *SessionRepository, cache *CacheService, keepRecentMessageCount int) *MemoryService {
	if keepRecentMessageCount <= 0 {
		keepRecentMessageCount = 8
	}
	return &MemoryService{
		sessionRepo:            sessionRepo,
		cache:                  cache,
		keepRecentMessageCount: keepRecentMessageCount,
	}
}

func (s *MemoryService) BuildContext(ctx context.Context, sessionID string) (map[string]any, error) {
	messages, err := s.sessionRepo.ListRecentMessages(ctx, sessionID, s.keepRecentMessageCount)
	if err != nil {
		return nil, err
	}
	session, err := s.sessionRepo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	summary := ""
	memoryState := map[string]any{}
	if session != nil {
		summary = session.Summary
		memoryState = ParseJSONMap(session.MemoryState)
	}

	if s.cache != nil {
		var cachedSummary string
		if s.cache.GetJSON(ctx, s.summaryCacheKey(sessionID), &cachedSummary) && strings.TrimSpace(cachedSummary) != "" {
			summary = cachedSummary
		}
		var cachedMemoryState map[string]any
		if s.cache.GetJSON(ctx, s.memoryCacheKey(sessionID), &cachedMemoryState) && cachedMemoryState != nil {
			memoryState = cachedMemoryState
		}
	}

	serializedMessages := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		serializedMessages = append(serializedMessages, map[string]any{
			"role": message.Role,
			"text": asString(ParseJSONMap(message.Content)["text"]),
		})
	}

	return map[string]any{
		"messages":     serializedMessages,
		"summary":      summary,
		"memory_state": memoryState,
	}, nil
}

func (s *MemoryService) MaybeUpdateSummary(ctx context.Context, sessionID string) error {
	messages, err := s.sessionRepo.ListRecentMessages(ctx, sessionID, s.keepRecentMessageCount+6)
	if err != nil {
		return err
	}
	if len(messages) <= s.keepRecentMessageCount {
		return nil
	}
	older := messages[:len(messages)-s.keepRecentMessageCount]
	parts := make([]string, 0, len(older))
	for _, message := range older {
		text := asString(ParseJSONMap(message.Content)["text"])
		if len(text) > 80 {
			text = text[:80]
		}
		parts = append(parts, fmt.Sprintf("%s:%s", message.Role, text))
	}
	summary := strings.Join(parts, " | ")
	if len(summary) > 2000 {
		summary = summary[:2000]
	}
	if err := s.sessionRepo.UpdateSummary(ctx, sessionID, summary); err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.SetJSON(ctx, s.summaryCacheKey(sessionID), summary, time.Hour)
	}
	return nil
}

func (s *MemoryService) RememberTurn(ctx context.Context, sessionID string, message string, plannedCalls []PlannedToolCall) error {
	current, err := s.BuildContext(ctx, sessionID)
	if err != nil {
		return err
	}
	memoryState, _ := current["memory_state"].(map[string]any)
	if memoryState == nil {
		memoryState = map[string]any{}
	}
	for _, call := range plannedCalls {
		arguments := call.Arguments
		if ticketNo := asString(arguments["ticket_no"]); ticketNo != "" {
			memoryState["last_ticket_no"] = ticketNo
		}
		if targetID := asString(arguments["target_id"]); targetID != "" {
			memoryState["last_ticket_no"] = targetID
		}
		if region := asString(arguments["region"]); region != "" {
			memoryState["last_region"] = region
		}
		if category := asString(arguments["category"]); category != "" {
			memoryState["last_category"] = category
		}
		if singleDate := asString(arguments["date"]); singleDate != "" {
			memoryState["last_date"] = singleDate
		}
		startDate := asString(arguments["start_date"])
		endDate := asString(arguments["end_date"])
		if startDate != "" && endDate != "" {
			memoryState["last_date_range"] = map[string]any{"start_date": startDate, "end_date": endDate}
			memoryState["last_date"] = endDate
		}
		if reportType := asString(arguments["report_type"]); reportType != "" {
			memoryState["last_report_type"] = reportType
		}
	}
	if trimmed := strings.TrimSpace(message); trimmed != "" {
		if len(trimmed) > 500 {
			trimmed = trimmed[:500]
		}
		memoryState["last_user_message"] = trimmed
	}
	if err := s.sessionRepo.UpdateMemoryState(ctx, sessionID, memoryState); err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.SetJSON(ctx, s.memoryCacheKey(sessionID), memoryState, time.Hour)
	}
	return nil
}

func (s *MemoryService) summaryCacheKey(sessionID string) string {
	return "session:summary:" + sessionID
}

func (s *MemoryService) memoryCacheKey(sessionID string) string {
	return "session:memory:" + sessionID
}

type ReportService struct {
	metricRepo  *MetricRepository
	ticketRepo  *TicketRepository
	releaseRepo *ReleaseRepository
}

func NewReportService(metricRepo *MetricRepository, ticketRepo *TicketRepository, releaseRepo *ReleaseRepository) *ReportService {
	return &ReportService{metricRepo: metricRepo, ticketRepo: ticketRepo, releaseRepo: releaseRepo}
}

func (s *ReportService) GenerateDailyReport(ctx context.Context) (map[string]any, error) {
	today := time.Now().Format("2006-01-02")
	refundSnapshot, err := s.metricRepo.GetRefundSnapshot(ctx, today, "")
	if err != nil {
		return nil, err
	}
	highPriorityTickets, err := s.ticketRepo.GetHighPriorityOpenTickets(ctx, 8)
	if err != nil {
		return nil, err
	}
	recentReleases, err := s.releaseRepo.GetRecentReleases(ctx, 5)
	if err != nil {
		return nil, err
	}

	lines := []string{"今日运营日报", "", "1. 退款率异常概览"}
	if len(refundSnapshot) == 0 {
		lines = append(lines, "- 今日暂无退款指标数据")
	} else {
		for _, item := range refundSnapshot[:min(len(refundSnapshot), 5)] {
			lines = append(lines, fmt.Sprintf("- %s-%s 退款率 %.2f%%，订单量 %d", asString(item["region"]), asString(item["category"]), asFloat(item["refund_rate"])*100, asInt(item["orders_cnt"], 0)))
		}
	}
	lines = append(lines, "", "2. 高优先级工单")
	if len(highPriorityTickets) == 0 {
		lines = append(lines, "- 当前没有高优先级未关闭工单")
	} else {
		for _, item := range highPriorityTickets[:min(len(highPriorityTickets), 5)] {
			rootCause := asString(item["root_cause"])
			if rootCause == "" {
				rootCause = "待定位"
			}
			lines = append(lines, fmt.Sprintf("- %s | %s | %s | %s", asString(item["ticket_no"]), asString(item["priority"]), asString(item["region"]), rootCause))
		}
	}
	lines = append(lines, "", "3. 最近发布")
	if len(recentReleases) == 0 {
		lines = append(lines, "- 最近暂无发布记录")
	} else {
		for _, item := range recentReleases[:min(len(recentReleases), 3)] {
			lines = append(lines, fmt.Sprintf("- %s %s @ %s | %s", asString(item["service_name"]), asString(item["release_version"]), asString(item["release_time"]), asString(item["change_summary"])))
		}
	}
	return map[string]any{"report_type": "daily", "content": strings.Join(lines, "\n")}, nil
}

type AnomalyService struct {
	metricRepo  *MetricRepository
	ticketRepo  *TicketRepository
	releaseRepo *ReleaseRepository
}

func NewAnomalyService(metricRepo *MetricRepository, ticketRepo *TicketRepository, releaseRepo *ReleaseRepository) *AnomalyService {
	return &AnomalyService{metricRepo: metricRepo, ticketRepo: ticketRepo, releaseRepo: releaseRepo}
}

func (s *AnomalyService) AnalyzeOperationalAnomaly(ctx context.Context, targetDate string, region string) (map[string]any, error) {
	refundSpikes, err := s.metricRepo.FindRefundSpikeCandidates(ctx, targetDate, region, 7, 5)
	if err != nil {
		return nil, err
	}
	slaByRootCause, err := s.ticketRepo.ListSLABreachedTickets(ctx, targetDate, region, "root_cause")
	if err != nil {
		return nil, err
	}
	slaByCategory, err := s.ticketRepo.ListSLABreachedTickets(ctx, targetDate, region, "category")
	if err != nil {
		return nil, err
	}
	affectedCategories := make([]string, 0)
	for _, item := range refundSpikes[:min(len(refundSpikes), 3)] {
		if category := asString(item["category"]); category != "" {
			affectedCategories = append(affectedCategories, category)
		}
	}
	breachSamples, err := s.ticketRepo.ListSLABreachSamples(ctx, targetDate, region, affectedCategories, 8)
	if err != nil {
		return nil, err
	}
	target, err := time.Parse("2006-01-02", targetDate)
	if err != nil {
		return nil, err
	}
	releases, err := s.releaseRepo.GetReleasesBetween(ctx, target.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"), target.AddDate(0, 0, 1).Format("2006-01-02 23:59:59"), 6)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"target_date":       targetDate,
		"region":            nullableAny(region),
		"refund_spikes":     refundSpikes,
		"sla_by_root_cause": slaByRootCause,
		"sla_by_category":   slaByCategory,
		"breach_samples":    breachSamples,
		"nearby_releases":   releases,
		"correlation":       buildCorrelationSummary(refundSpikes, slaByRootCause, slaByCategory, releases),
	}, nil
}

func buildCorrelationSummary(refundSpikes []map[string]any, slaByRootCause []map[string]any, slaByCategory []map[string]any, releases []map[string]any) map[string]any {
	var topSpike map[string]any
	var topRootCause map[string]any
	if len(refundSpikes) > 0 {
		topSpike = refundSpikes[0]
	}
	if len(slaByRootCause) > 0 {
		topRootCause = slaByRootCause[0]
	}
	categoryCounts := make(map[string]int)
	for _, item := range slaByCategory {
		categoryCounts[asString(item["group_key"])] = asInt(item["ticket_count"], 0)
	}
	var matchedCategory map[string]any
	if topSpike != nil {
		category := asString(topSpike["category"])
		matchedCategory = map[string]any{
			"category":     category,
			"ticket_count": categoryCounts[category],
		}
	}
	suspectedCauses := make([]map[string]any, 0)
	if len(releases) > 0 && topRootCause != nil && asString(topRootCause["group_key"]) == "系统发布故障" {
		suspectedCauses = append(suspectedCauses, map[string]any{
			"cause":      "最近发布可能触发运营异常",
			"confidence": "high",
			"evidence": []string{
				"超 SLA 工单主因集中在系统发布故障",
				"异常窗口内存在最近发布记录",
				"退款异常与工单异常在时间窗口内重叠",
			},
		})
	} else if topRootCause != nil {
		suspectedCauses = append(suspectedCauses, map[string]any{
			"cause":      "工单主因集中在 " + asString(topRootCause["group_key"]),
			"confidence": "medium",
			"evidence":   []string{"超 SLA 工单原因分布明显集中"},
		})
	}
	if matchedCategory != nil && asInt(matchedCategory["ticket_count"], 0) > 0 {
		category := asString(matchedCategory["category"])
		suspectedCauses = append(suspectedCauses, map[string]any{
			"cause":      category + "类目同时出现退款率抬升和超 SLA 工单堆积",
			"confidence": "medium",
			"evidence": []string{
				category + "退款率高于近 7 天基线",
				fmt.Sprintf("%s 超 SLA 工单数量为 %d", category, asInt(matchedCategory["ticket_count"], 0)),
			},
		})
	}
	return map[string]any{
		"top_refund_spike":   topSpike,
		"top_sla_root_cause": topRootCause,
		"matched_category":   matchedCategory,
		"suspected_causes":   suspectedCauses,
	}
}

type PlannerService struct {
	cfg      Config
	cache    *CacheService
	llm      *LLMService
	router   *MessageRouter
	registry *ToolRegistry
	metrics  *MetricsRecorder
}

type PlannerResult struct {
	Calls     []PlannedToolCall
	Source    string
	LatencyMS int
	CacheHit  bool
}

func NewPlannerService(cfg Config, cache *CacheService, llm *LLMService, registry *ToolRegistry, metrics *MetricsRecorder) *PlannerService {
	return &PlannerService{
		cfg:      cfg,
		cache:    cache,
		llm:      llm,
		router:   &MessageRouter{},
		registry: registry,
		metrics:  metrics,
	}
}

func (s *PlannerService) Plan(ctx context.Context, message string, memory map[string]any) (PlannerResult, error) {
	ctx, span := StartSpan(ctx, "planner.plan")
	defer span.End()
	span.SetAttributes(
		attribute.String("planner.mode", s.cfg.AgentRuntimeMode),
		attribute.Int("planner.message_length", len([]rune(message))),
	)

	started := time.Now()
	routeAnalysis := s.router.Analyze(message, memory)
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ROUTER_DISABLE_FAST_PATH")), "true") {
		routeAnalysis.FastPath = nil
	}
	heuristicCalls := routeAnalysis.Fallback
	useHeuristic := s.cfg.AgentRuntimeMode == "heuristic" || !s.cfg.HasUsableLLMConfig()
	if useHeuristic {
		source := "heuristic_router"
		if len(routeAnalysis.FastPath) > 0 {
			source = "l0_rule_router"
			heuristicCalls = routeAnalysis.FastPath
		}
		latencyMS := int(time.Since(started).Milliseconds())
		s.metrics.RecordPlanner(source, latencyMS)
		span.SetAttributes(
			attribute.String("planner.source", source),
			attribute.Int("planner.latency_ms", latencyMS),
		)
		return PlannerResult{Calls: heuristicCalls, Source: source, LatencyMS: latencyMS}, nil
	}

	if len(routeAnalysis.FastPath) > 0 {
		latencyMS := int(time.Since(started).Milliseconds())
		s.metrics.RecordPlanner("l0_rule_router", latencyMS)
		span.SetAttributes(
			attribute.String("planner.source", "l0_rule_router"),
			attribute.Int("planner.latency_ms", latencyMS),
		)
		return PlannerResult{Calls: routeAnalysis.FastPath, Source: "l0_rule_router", LatencyMS: latencyMS}, nil
	}

	selectedSchemas, includeGenerateReport := selectSchemasForRouteDecision(message, routeAnalysis.Hints, s.registry.ListSchemas())
	cachePayload := map[string]any{
		"message":          message,
		"memory_state":     compactMemoryState(memory),
		"recent_turns":     compactRecentTurns(memory, s.cfg.RouterRecentMessageCount),
		"hints":            routeAnalysis.Hints,
		"schemas":          selectedSchemas,
		"include_report":   includeGenerateReport,
		"strong_only":      routeAnalysis.RequiresStrongModel,
		"primary_model":    s.cfg.RouterPrimaryModel,
		"fallback_model":   s.cfg.RouterFallbackModel,
		"confidence_cutoff": s.cfg.RouterConfidenceCutoff,
	}
	cacheKey := "planner:" + HashKey(MustJSON(cachePayload))
	var cached []PlannedToolCall
	if s.cache != nil && s.cache.GetJSON(ctx, cacheKey, &cached) && len(cached) > 0 {
		latencyMS := int(time.Since(started).Milliseconds())
		s.metrics.RecordPlanner("planner_cache", latencyMS)
		s.metrics.RecordPlannerCache(true)
		span.SetAttributes(
			attribute.String("planner.source", "planner_cache"),
			attribute.Bool("planner.cache_hit", true),
			attribute.Int("planner.latency_ms", latencyMS),
		)
		return PlannerResult{Calls: cached, Source: "planner_cache", LatencyMS: latencyMS, CacheHit: true}, nil
	}
	s.metrics.RecordPlannerCache(false)
	span.SetAttributes(attribute.Bool("planner.cache_hit", false))

	runRouteModel := func(model string, strongModel bool) ([]PlannedToolCall, RouteDecision, string, error) {
		inputItems := buildRouteInput(message, memory, routeAnalysis.Hints, s.cfg.RouterRecentMessageCount, strongModel)
		tools := buildRouteDecisionTools(selectedSchemas, includeGenerateReport)
		source := routeTraceLabel(model)
		response, err := s.llm.ResponsesCreateWithOptions(
			ctx,
			inputItems,
			tools,
			buildRouteSystemPrompt(s.cfg.RouterNoThink, strongModel),
			false,
			LLMRequestOptions{Model: model},
		)
		if err != nil {
			return nil, RouteDecision{}, source, err
		}
		decision, ok := parseRouteDecision(response)
		if !ok {
			return nil, RouteDecision{}, source, nil
		}
		calls, ok := routeDecisionToPlannedCalls(decision, s.registry, routeAnalysis.Hints, message)
		if !ok {
			return nil, decision, source, nil
		}
		return calls, decision, source, nil
	}

	source := ""
	finalCalls := []PlannedToolCall(nil)

	if !routeAnalysis.RequiresStrongModel {
		primaryCalls, decision, primarySource, err := runRouteModel(s.cfg.RouterPrimaryModel, false)
		if err != nil {
			if s.cfg.AgentRuntimeMode == "llm" {
				RecordSpanError(span, err)
				return PlannerResult{}, err
			}
			s.metrics.RecordLLMFallback("l1_exception")
		} else if !shouldEscalateToStrongModel(decision, s.cfg, primaryCalls) {
			source = primarySource
			finalCalls = primaryCalls
		} else {
			s.metrics.RecordLLMFallback(describeRouteFailure(decision, primaryCalls))
		}
	}

	if len(finalCalls) == 0 {
		if strings.EqualFold(strings.TrimSpace(s.cfg.RouterPrimaryModel), strings.TrimSpace(s.cfg.RouterFallbackModel)) && !routeAnalysis.RequiresStrongModel {
			if s.cfg.AgentRuntimeMode == "llm" {
				err := NewValidation("route decision failed and strong fallback model is identical to the primary router model")
				RecordSpanError(span, err)
				return PlannerResult{}, err
			}
		} else {
			fallbackCalls, _, fallbackSource, err := runRouteModel(s.cfg.RouterFallbackModel, true)
			if err != nil {
				if s.cfg.AgentRuntimeMode == "llm" {
					RecordSpanError(span, err)
					return PlannerResult{}, err
				}
				s.metrics.RecordLLMFallback("l2_exception")
			} else if len(fallbackCalls) > 0 {
				source = fallbackSource
				finalCalls = fallbackCalls
			}
		}
	}

	if len(finalCalls) == 0 {
		latencyMS := int(time.Since(started).Milliseconds())
		s.metrics.RecordPlanner("heuristic_router", latencyMS)
		span.SetAttributes(
			attribute.String("planner.source", "heuristic_router"),
			attribute.Int("planner.latency_ms", latencyMS),
		)
		span.AddEvent("planner_fallback_heuristic")
		return PlannerResult{Calls: heuristicCalls, Source: "heuristic_router", LatencyMS: latencyMS}, nil
	}

	if s.cache != nil {
		s.cache.SetJSON(ctx, cacheKey, finalCalls, 10*time.Minute)
	}
	latencyMS := int(time.Since(started).Milliseconds())
	s.metrics.RecordPlanner(source, latencyMS)
	span.SetAttributes(
		attribute.String("planner.source", source),
		attribute.Int("planner.latency_ms", latencyMS),
		attribute.Int("planner.call_count", len(finalCalls)),
	)
	return PlannerResult{Calls: finalCalls, Source: source, LatencyMS: latencyMS}, nil
}

func buildPlannerInput(message string, memory map[string]any, hints []PlannedToolCall) []map[string]any {
	result := make([]map[string]any, 0)
	if summary := asString(memory["summary"]); summary != "" {
		result = append(result, map[string]any{"role": "system", "content": "Conversation summary:\n" + summary})
	}
	if memoryState, ok := memory["memory_state"].(map[string]any); ok && len(memoryState) > 0 {
		result = append(result, map[string]any{"role": "system", "content": "Session memory:\n" + MustJSON(memoryState)})
	}
	if messages, ok := memory["messages"].([]map[string]any); ok {
		for _, item := range messages {
			text := asString(item["text"])
			if strings.TrimSpace(text) == "" {
				continue
			}
			role := asString(item["role"])
			if role == "" {
				role = "user"
			}
			result = append(result, map[string]any{"role": role, "content": text})
		}
	}
	if len(hints) > 0 {
		result = append(result, map[string]any{
			"role":    "system",
			"content": "Heuristic tool hints (use as strong guidance, not final answer): " + MustJSON(hints),
		})
	}
	result = append(result, map[string]any{"role": "user", "content": message})
	return result
}

func buildPlannerInputForOllama(message string, memory map[string]any, hints []PlannedToolCall) []map[string]any {
	result := make([]map[string]any, 0, 3)
	contextParts := make([]string, 0, 4)
	if len(hints) == 0 {
		if summary := strings.TrimSpace(asString(memory["summary"])); summary != "" {
			contextParts = append(contextParts, "Conversation summary:\n"+summary)
		}
		if memoryState, ok := memory["memory_state"].(map[string]any); ok && len(memoryState) > 0 {
			contextParts = append(contextParts, "Session memory JSON:\n"+MustJSON(memoryState))
		}
		if messages, ok := memory["messages"].([]map[string]any); ok && len(messages) > 0 {
			recentLines := make([]string, 0, len(messages))
			for _, item := range messages {
				text := strings.TrimSpace(asString(item["text"]))
				if text == "" {
					continue
				}
				role := asString(item["role"])
				if role == "" {
					role = "user"
				}
				recentLines = append(recentLines, role+": "+text)
			}
			if len(recentLines) > 0 {
				contextParts = append(contextParts, "Recent turns:\n"+strings.Join(recentLines, "\n"))
			}
		}
	}
	if len(hints) > 0 {
		contextParts = append(contextParts, "Heuristic tool hints. Prefer the first hint and copy its arguments exactly:\n"+MustJSON(hints))
	}
	if len(contextParts) > 0 {
		result = append(result, map[string]any{
			"role":    "system",
			"content": strings.Join(contextParts, "\n\n"),
		})
	}
	userContent := "Latest user request:\n" + strings.TrimSpace(message)
	if len(hints) > 0 {
		firstHint := hints[0]
		userContent += "\n\nPreferred tool call:\n" +
			"tool_name=" + firstHint.ToolName + "\n" +
			"arguments_json=" + MustJSON(firstHint.Arguments) + "\n\n" +
			"Call the preferred tool now unless it clearly conflicts with the latest user request."
	}
	result = append(result, map[string]any{
		"role":    "user",
		"content": userContent + "\n\nReturn tool call(s) now.",
	})
	return result
}

func buildPlannerToolsForProvider(provider string, message string, hints []PlannedToolCall, schemas []ToolSchema) []map[string]any {
	includeGenerateReport := isExplicitReportRequest(message)
	selectedSchemas := schemas
	if provider == "ollama" {
		selectedSchemas, includeGenerateReport = selectSchemasForOllamaPlanner(message, hints, schemas)
	}
	return buildPlannerTools(selectedSchemas, includeGenerateReport)
}

func buildPlannerTools(schemas []ToolSchema, includeGenerateReport bool) []map[string]any {
	result := make([]map[string]any, 0, len(schemas)+1)
	sort.Slice(schemas, func(i int, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})
	for _, schema := range schemas {
		parameters := schema.Parameters
		if parameters["type"] == "object" {
			if _, ok := parameters["properties"]; !ok {
				parameters["properties"] = map[string]any{}
			}
			if _, ok := parameters["required"]; !ok {
				parameters["required"] = []string{}
			}
			parameters["additionalProperties"] = false
		}
		result = append(result, map[string]any{
			"type":        "function",
			"name":        schema.Name,
			"description": plannerToolDescription(schema.Name, schema.Description),
			"parameters":  parameters,
		})
	}
	if includeGenerateReport {
		result = append(result, map[string]any{
			"type":        "function",
			"name":        "generate_report",
			"description": plannerToolDescription("generate_report", "生成运营日报或周报"),
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"report_type": map[string]any{"type": "string", "enum": []string{"daily"}},
				},
				"required": []string{"report_type"},
			},
		})
	}
	return result
}

func plannerSystemPromptFor(provider string) string {
	if provider == "ollama" {
		return ollamaPlannerSystemPrompt
	}
	return plannerSystemPrompt
}

func plannerToolDescription(name string, description string) string {
	routingHints := map[string]string{
		"query_refund_metrics":        "适用于查询退款率指标、退款率最高类目、指定区域/类目的退款率表现。Use for refund metric lookup.",
		"find_refund_anomalies":       "适用于退款率异常、异常类目、异常区域。Use for refund anomaly detection.",
		"list_sla_breached_tickets":   "适用于超SLA工单查询。按原因分类 -> group_by=root_cause；按优先级 -> priority；按类目/分类 -> category；按处理人 -> assignee_name.",
		"get_ticket_detail":           "适用于查询单个工单详情。Use for ticket detail lookup.",
		"get_ticket_comments":         "适用于查询工单备注、评论、最近操作记录。Use for comments and recent actions.",
		"get_recent_releases":         "适用于查询最近发布记录。Use for recent release lookup.",
		"analyze_operational_anomaly": "仅在用户明确要求归因分析，且问题同时涉及退款率、超SLA工单、发布影响时使用。",
		"run_readonly_sql":            "仅在用户明确要求SQL查询时使用，并且必须是只读白名单SQL。",
		"propose_assign_ticket":       "写请求专用。适用于把工单分派给某人，只能生成 proposal。",
		"propose_add_ticket_comment":  "写请求专用。适用于给工单添加备注，只能生成 proposal。",
		"propose_escalate_ticket":     "写请求专用。适用于升级工单优先级，只能生成 proposal。",
		"generate_report":             "只在用户明确要求日报、周报、report时使用。Never use as fallback for generic analysis or lookup.",
	}
	if hint, ok := routingHints[name]; ok {
		return description + "。路由提示：" + hint
	}
	return description
}

func selectSchemasForOllamaPlanner(message string, hints []PlannedToolCall, schemas []ToolSchema) ([]ToolSchema, bool) {
	schemaByName := make(map[string]ToolSchema, len(schemas))
	for _, schema := range schemas {
		schemaByName[schema.Name] = schema
	}

	includeGenerateReport := isExplicitReportRequest(message)
	if len(hints) == 0 {
		filtered := make([]ToolSchema, 0, len(schemas))
		for _, schema := range schemas {
			if schema.Name == "generate_report" {
				continue
			}
			filtered = append(filtered, schema)
		}
		return filtered, includeGenerateReport
	}

	selected := make([]ToolSchema, 0, len(hints))
	seen := make(map[string]bool, len(hints))
	for _, hint := range hints {
		if hint.ToolName == "generate_report" {
			includeGenerateReport = true
			continue
		}
		if seen[hint.ToolName] {
			continue
		}
		schema, ok := schemaByName[hint.ToolName]
		if !ok {
			continue
		}
		selected = append(selected, schema)
		seen[hint.ToolName] = true
	}
	if len(selected) > 0 || includeGenerateReport {
		return selected, includeGenerateReport
	}
	return schemas, false
}

func isExplicitReportRequest(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(message, "日报") || strings.Contains(message, "周报") || strings.Contains(lower, "report")
}

func parsePlannedCalls(response map[string]any) []PlannedToolCall {
	result := make([]PlannedToolCall, 0)
	switch output := response["output"].(type) {
	case []any:
		for _, item := range output {
			call, _ := item.(map[string]any)
			if planned, ok := parsePlannedCall(call); ok {
				result = append(result, planned)
			}
		}
	case []map[string]any:
		for _, call := range output {
			if planned, ok := parsePlannedCall(call); ok {
				result = append(result, planned)
			}
		}
	}
	return result
}

func parsePlannedCall(call map[string]any) (PlannedToolCall, bool) {
	if call == nil {
		return PlannedToolCall{}, false
	}
	if asString(call["type"]) != "function_call" {
		return PlannedToolCall{}, false
	}
	arguments := map[string]any{}
	if rawArguments := asString(call["arguments"]); strings.TrimSpace(rawArguments) != "" {
		_ = json.Unmarshal([]byte(rawArguments), &arguments)
	}
	return PlannedToolCall{
		ToolName:  asString(call["name"]),
		Arguments: arguments,
	}, true
}

func mergeWithHeuristic(planned []PlannedToolCall, heuristic []PlannedToolCall) []PlannedToolCall {
	heuristicMap := make(map[string]PlannedToolCall, len(heuristic))
	for _, call := range heuristic {
		heuristicMap[call.ToolName] = call
	}
	result := make([]PlannedToolCall, 0, len(planned))
	for _, call := range planned {
		mergedArguments := make(map[string]any, len(call.Arguments))
		for key, value := range call.Arguments {
			mergedArguments[key] = value
		}
		if hint, ok := heuristicMap[call.ToolName]; ok {
			for key, value := range hint.Arguments {
				mergedArguments[key] = value
			}
		}
		result = append(result, PlannedToolCall{ToolName: call.ToolName, Arguments: mergedArguments})
	}
	return result
}

func nullableAny(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
