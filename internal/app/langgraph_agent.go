package app

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type LangGraphChatService struct {
	cfg           Config
	sessionRepo   *SessionRepository
	auditService  *AuditService
	memoryService *MemoryService
	client        *LangGraphClient
	metrics       *MetricsRecorder
}

func NewLangGraphChatService(
	cfg Config,
	sessionRepo *SessionRepository,
	auditService *AuditService,
	memoryService *MemoryService,
	client *LangGraphClient,
	metrics *MetricsRecorder,
) *LangGraphChatService {
	return &LangGraphChatService{
		cfg:           cfg,
		sessionRepo:   sessionRepo,
		auditService:  auditService,
		memoryService: memoryService,
		client:        client,
		metrics:       metrics,
	}
}

func (s *LangGraphChatService) HandleChat(ctx context.Context, sessionID string, user User, message string) (ChatResponse, error) {
	ctx, span := StartSpan(ctx, "langgraph.handle_chat")
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
	result, err := s.client.Chat(ctx, LangGraphChatRequest{
		TraceID:   traceID,
		SessionID: sessionID,
		UserID:    user.ID,
		Message:   message,
		Memory:    memory,
	})
	if err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}

	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = "已处理请求。"
	}
	if err := s.sessionRepo.AddMessage(ctx, sessionID, "assistant", map[string]any{"text": answer}, traceID); err != nil {
		RecordSpanError(span, err)
		return ChatResponse{}, err
	}
	_ = s.memoryService.RememberTurn(ctx, sessionID, message, result.PlannedCalls)
	_ = s.memoryService.MaybeUpdateSummary(ctx, sessionID)
	_ = s.auditService.LogEvent(ctx, traceID, sessionID, &user.ID, "response_returned", map[string]any{"status": result.Status})

	latencyMS := int(time.Since(started).Milliseconds())
	s.metrics.RecordChat(result.Status, latencyMS)
	span.SetAttributes(
		attribute.String("chat.status", result.Status),
		attribute.Int("chat.latency_ms", latencyMS),
		attribute.String("planner.source", result.PlanningSource),
		attribute.Int("planner.latency_ms", result.PlannerLatencyMS),
		attribute.Bool("planner.cache_hit", result.PlanCacheHit),
	)

	return ChatResponse{
		TraceID:          traceID,
		SessionID:        sessionID,
		Status:           result.Status,
		Answer:           answer,
		PlanningSource:   result.PlanningSource,
		PlannerLatencyMS: result.PlannerLatencyMS,
		PlanCacheHit:     result.PlanCacheHit,
		ToolCalls:        result.ToolCalls,
		Approval:         result.Approval,
	}, nil
}
