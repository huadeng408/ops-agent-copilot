package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
)

type Application struct {
	Config  Config
	DB      *sqlx.DB
	Cache   *CacheService
	Metrics *MetricsRecorder
	LLM     *LLMService
	LangG   *LangGraphClient
	Router  http.Handler
}

func NewApplication(cfg Config, db *sqlx.DB) *Application {
	registry := prometheus.NewRegistry()
	metrics := NewMetricsRecorder(registry)
	cache := NewCacheService(cfg.CacheURL())
	llm := NewLLMService(cfg, metrics)
	app := &Application{
		Config:  cfg,
		DB:      db,
		Cache:   cache,
		Metrics: metrics,
		LLM:     llm,
		LangG:   NewLangGraphClient(cfg),
	}
	app.Router = app.buildRouter()
	return app
}

func (a *Application) buildRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(a.traceMiddleware)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	router.Get("/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsHTML))
	})
	router.Handle("/metrics", a.Metrics.Handler())
	router.Get("/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(adminHTML))
	})

	router.Route("/api/v1", func(r chi.Router) {
		r.Post("/chat", a.handleChat)
		r.Get("/approvals", a.handleListApprovals)
		r.Get("/approvals/{approval_no}", a.handleApprovalDetail)
		r.Post("/approvals/{approval_no}/approve", a.handleApproveApproval)
		r.Post("/approvals/{approval_no}/reject", a.handleRejectApproval)
		r.Get("/audit", a.handleAudit)
		r.Get("/tickets/{ticket_no}", a.handleTicketDetail)
	})
	router.Route("/internal/v1", func(r chi.Router) {
		r.Use(a.internalAuthMiddleware)
		r.Post("/tool-invoke", a.handleInternalToolInvoke)
		r.Post("/proposals", a.handleInternalCreateProposal)
		r.Post("/reports/daily", a.handleInternalDailyReport)
	})
	return WrapHandlerWithTelemetry(router)
}

func (a *Application) traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
		if traceID == "" {
			traceID = SpanTraceIDFromContext(r.Context())
		}
		if traceID == "" {
			traceID = "http_" + time.Now().Format("20060102150405.000000")
		}
		ctx := ContextWithTraceID(r.Context(), traceID)
		AnnotateCurrentSpan(ctx, attribute.String("app.trace_id", traceID))
		w.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Application) withTx(w http.ResponseWriter, r *http.Request, fn func(context.Context, *sqlx.Tx, *requestScope) error) {
	tx, err := a.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer tx.Rollback()

	scope := a.newRequestScope(tx)
	if err := fn(r.Context(), tx, scope); err != nil {
		WriteError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		WriteError(w, err)
	}
}

type requestScope struct {
	userRepo        *UserRepository
	sessionRepo     *SessionRepository
	ticketRepo      *TicketRepository
	metricRepo      *MetricRepository
	releaseRepo     *ReleaseRepository
	approvalRepo    *ApprovalRepository
	auditRepo       *AuditRepository
	auditService    *AuditService
	verifier        *VerifierService
	memoryService   *MemoryService
	reportService   *ReportService
	anomalyService  *AnomalyService
	toolRegistry    *ToolRegistry
	plannerService  *PlannerService
	approvalService *ApprovalService
	agentService    *AgentService
	langGraphChat   *LangGraphChatService
}

func (a *Application) newRequestScope(db DBTX) *requestScope {
	return a.newRequestScopeWithConfig(db, a.Config)
}

func (a *Application) newRequestScopeWithConfig(db DBTX, cfg Config) *requestScope {
	userRepo := NewUserRepository(db)
	sessionRepo := NewSessionRepository(db)
	ticketRepo := NewTicketRepository(db)
	metricRepo := NewMetricRepository(db)
	releaseRepo := NewReleaseRepository(db)
	approvalRepo := NewApprovalRepository(db)
	auditRepo := NewAuditRepository(db)
	auditService := NewAuditService(auditRepo)
	verifier := NewVerifierService(ticketRepo, a.Metrics, NewSQLGuard())
	memoryService := NewMemoryService(sessionRepo, a.Cache, cfg.KeepRecentMessageCount)
	reportService := NewReportService(metricRepo, ticketRepo, releaseRepo)
	anomalyService := NewAnomalyService(metricRepo, ticketRepo, releaseRepo)
	toolRegistry := BuildDefaultToolRegistry(auditService, a.Metrics)
	plannerService := NewPlannerService(cfg, a.Cache, a.LLM, toolRegistry, a.Metrics)
	approvalService := NewApprovalService(approvalRepo, ticketRepo, userRepo, verifier, auditService, a.Metrics)
	baseToolContext := ToolContext{
		MetricRepo:     metricRepo,
		TicketRepo:     ticketRepo,
		ReleaseRepo:    releaseRepo,
		Verifier:       verifier,
		AnomalyService: anomalyService,
		DB:             db,
		Config:         cfg,
	}
	agentService := NewAgentService(cfg, sessionRepo, auditService, memoryService, toolRegistry, approvalService, reportService, plannerService, a.Metrics, baseToolContext)
	langGraphChat := NewLangGraphChatService(cfg, sessionRepo, auditService, memoryService, a.LangG, a.Metrics)
	return &requestScope{
		userRepo:        userRepo,
		sessionRepo:     sessionRepo,
		ticketRepo:      ticketRepo,
		metricRepo:      metricRepo,
		releaseRepo:     releaseRepo,
		approvalRepo:    approvalRepo,
		auditRepo:       auditRepo,
		auditService:    auditService,
		verifier:        verifier,
		memoryService:   memoryService,
		reportService:   reportService,
		anomalyService:  anomalyService,
		toolRegistry:    toolRegistry,
		plannerService:  plannerService,
		approvalService: approvalService,
		agentService:    agentService,
		langGraphChat:   langGraphChat,
	}
}

func (a *Application) handleChat(w http.ResponseWriter, r *http.Request) {
	var payload ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, NewValidation("请求体格式不合法"))
		return
	}

	cfg := a.Config
	if strings.TrimSpace(payload.RuntimeMode) != "" {
		cfg.AgentRuntimeMode = normalizeRuntimeMode(payload.RuntimeMode)
	}

	if cfg.UseLangGraphRuntime() {
		scope := a.newRequestScopeWithConfig(a.DB, cfg)
		user, err := scope.userRepo.GetByID(r.Context(), payload.UserID)
		if err != nil {
			WriteError(w, err)
			return
		}
		response, err := scope.langGraphChat.HandleChat(r.Context(), payload.SessionID, *user, payload.Message)
		if err != nil {
			WriteError(w, err)
			return
		}
		RespondJSON(w, http.StatusOK, response)
		return
	}

	a.withTx(w, r, func(ctx context.Context, tx *sqlx.Tx, _ *requestScope) error {
		scope := a.newRequestScopeWithConfig(tx, cfg)
		user, err := scope.userRepo.GetByID(ctx, payload.UserID)
		if err != nil {
			return err
		}
		response, err := scope.agentService.HandleChat(ctx, payload.SessionID, *user, payload.Message)
		if err != nil {
			return err
		}
		RespondJSON(w, http.StatusOK, response)
		return nil
	})
}

func (a *Application) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := parseQueryInt(r, "limit", 20)
	scope := a.newRequestScope(a.DB)
	approvals, err := scope.approvalRepo.ListRecent(r.Context(), status, limit)
	if err != nil {
		WriteError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(approvals))
	for _, approval := range approvals {
		items = append(items, renderApprovalListItem(approval))
	}
	RespondJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *Application) handleApprovalDetail(w http.ResponseWriter, r *http.Request) {
	approvalNo := chi.URLParam(r, "approval_no")
	scope := a.newRequestScope(a.DB)
	approval, err := scope.approvalRepo.GetByNo(r.Context(), approvalNo)
	if err != nil {
		WriteError(w, err)
		return
	}
	if approval == nil {
		WriteError(w, NewNotFound("approval not found"))
		return
	}
	RespondJSON(w, http.StatusOK, renderApprovalDetail(*approval))
}

func (a *Application) handleApproveApproval(w http.ResponseWriter, r *http.Request) {
	approvalNo := chi.URLParam(r, "approval_no")
	var payload ApprovalApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, NewValidation("请求体格式不合法"))
		return
	}
	a.withTx(w, r, func(ctx context.Context, _ *sqlx.Tx, scope *requestScope) error {
		approver, err := scope.userRepo.GetByID(ctx, payload.ApproverUserID)
		if err != nil {
			return err
		}
		approval, result, err := scope.approvalService.Approve(ctx, approvalNo, *approver)
		if err != nil {
			return err
		}
		response := ApprovalResponse{
			ApprovalNo:      approval.ApprovalNo,
			IdempotencyKey:  approval.IdempotencyKey,
			Status:          approval.Status,
			Version:         approval.Version,
			ExecutionResult: map[string]any{"success": true},
			ExecutionError:  approval.ExecutionError,
		}
		for key, value := range result {
			response.ExecutionResult[key] = value
		}
		RespondJSON(w, http.StatusOK, response)
		return nil
	})
}

func (a *Application) handleRejectApproval(w http.ResponseWriter, r *http.Request) {
	approvalNo := chi.URLParam(r, "approval_no")
	var payload ApprovalRejectRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, NewValidation("请求体格式不合法"))
		return
	}
	a.withTx(w, r, func(ctx context.Context, _ *sqlx.Tx, scope *requestScope) error {
		approver, err := scope.userRepo.GetByID(ctx, payload.ApproverUserID)
		if err != nil {
			return err
		}
		approval, err := scope.approvalService.Reject(ctx, approvalNo, *approver, payload.Reason)
		if err != nil {
			return err
		}
		RespondJSON(w, http.StatusOK, ApprovalResponse{
			ApprovalNo:     approval.ApprovalNo,
			IdempotencyKey: approval.IdempotencyKey,
			Status:         approval.Status,
			Version:        approval.Version,
			RejectedReason: approval.RejectedReason,
		})
		return nil
	})
}

func (a *Application) handleAudit(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("trace_id")
	eventType := r.URL.Query().Get("event_type")
	limit := parseQueryInt(r, "limit", 50)
	scope := a.newRequestScope(a.DB)

	var (
		logs      []AuditLog
		toolCalls []ToolCallLog
		err       error
	)
	if traceID != "" {
		logs, err = scope.auditRepo.ListByTraceID(r.Context(), traceID, eventType)
		if err == nil {
			toolCalls, err = scope.auditRepo.ListToolCallsByTraceID(r.Context(), traceID)
		}
	} else {
		logs, err = scope.auditRepo.ListRecent(r.Context(), limit, eventType)
		toolCalls = []ToolCallLog{}
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	eventTypes, err := scope.auditRepo.ListEventTypes(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	renderedLogs := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		renderedLogs = append(renderedLogs, map[string]any{
			"id":         log.ID,
			"trace_id":   log.TraceID,
			"session_id": nullableAny(log.SessionID),
			"user_id":    log.UserID,
			"event_type": log.EventType,
			"event_data": ParseJSONMap(log.EventData),
			"created_at": log.CreatedAt.Format(time.RFC3339),
		})
	}
	renderedToolCalls := make([]map[string]any, 0, len(toolCalls))
	for _, item := range toolCalls {
		renderedToolCalls = append(renderedToolCalls, map[string]any{
			"tool_name":      item.ToolName,
			"tool_type":      item.ToolType,
			"success":        item.Success,
			"latency_ms":     item.LatencyMS,
			"error_message":  nullableAny(item.ErrorMessage),
			"input_payload":  ParseJSONMap(item.InputPayload),
			"output_payload": ParseJSONMap(item.OutputPayload),
			"created_at":     item.CreatedAt.Format(time.RFC3339),
		})
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"trace_id":              nullableAny(traceID),
		"event_type":            nullableAny(eventType),
		"count":                 len(logs),
		"available_event_types": eventTypes,
		"logs":                  renderedLogs,
		"tool_calls":            renderedToolCalls,
	})
}

func (a *Application) handleTicketDetail(w http.ResponseWriter, r *http.Request) {
	ticketNo := chi.URLParam(r, "ticket_no")
	commentsLimit := parseQueryInt(r, "comments_limit", 10)
	actionsLimit := parseQueryInt(r, "actions_limit", 10)
	scope := a.newRequestScope(a.DB)
	detail, err := scope.ticketRepo.GetTicketDetail(r.Context(), ticketNo)
	if err != nil {
		WriteError(w, err)
		return
	}
	comments, err := scope.ticketRepo.GetTicketComments(r.Context(), ticketNo, commentsLimit)
	if err != nil {
		WriteError(w, err)
		return
	}
	actions, err := scope.ticketRepo.GetRecentTicketActions(r.Context(), ticketNo, actionsLimit)
	if err != nil {
		WriteError(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"ticket":   detail,
		"comments": comments,
		"actions":  actions,
	})
}

func renderApprovalListItem(approval Approval) map[string]any {
	return map[string]any{
		"approval_no":     approval.ApprovalNo,
		"idempotency_key": approval.IdempotencyKey,
		"session_id":      approval.SessionID,
		"trace_id":        approval.TraceID,
		"action_type":     approval.ActionType,
		"target_id":       approval.TargetID,
		"status":          approval.Status,
		"version":         approval.Version,
		"requested_by":    approval.RequestedBy,
		"approved_by":     approval.ApprovedBy,
		"execution_error": nullableAny(approval.ExecutionError),
		"rejected_reason": nullableAny(approval.RejectedReason),
		"created_at":      approval.CreatedAt.Format(time.RFC3339),
		"approved_at":     approval.ApprovedAt,
		"executed_at":     approval.ExecutedAt,
	}
}

func renderApprovalDetail(approval Approval) map[string]any {
	return map[string]any{
		"approval_no":      approval.ApprovalNo,
		"idempotency_key":  approval.IdempotencyKey,
		"session_id":       approval.SessionID,
		"trace_id":         approval.TraceID,
		"action_type":      approval.ActionType,
		"target_type":      approval.TargetType,
		"target_id":        approval.TargetID,
		"payload":          ParseJSONMap(approval.Payload),
		"reason":           approval.Reason,
		"status":           approval.Status,
		"version":          approval.Version,
		"requested_by":     approval.RequestedBy,
		"approved_by":      approval.ApprovedBy,
		"approved_at":      approval.ApprovedAt,
		"executed_at":      approval.ExecutedAt,
		"execution_result": ParseJSONMap(approval.ExecutionResult),
		"execution_error":  nullableAny(approval.ExecutionError),
		"rejected_reason":  nullableAny(approval.RejectedReason),
		"created_at":       approval.CreatedAt,
	}
}

func parseQueryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ops-agent-copilot admin</title>
  <style>
    body { font-family: "Segoe UI", "PingFang SC", sans-serif; margin: 0; padding: 24px; background: #f5f7fb; color: #1b2430; }
    h1, h2, h3 { margin-top: 0; }
    .grid { display: grid; grid-template-columns: 1.2fr 1fr; gap: 20px; }
    .card { background: #fff; border: 1px solid #dce3ef; border-radius: 16px; padding: 16px; box-shadow: 0 8px 24px rgba(0,0,0,.04); }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { padding: 10px 8px; border-bottom: 1px solid #edf1f7; text-align: left; vertical-align: top; }
    input, select, button { font: inherit; padding: 8px 10px; border-radius: 10px; border: 1px solid #ccd6e5; }
    button { cursor: pointer; }
    .toolbar { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
    .pill { display: inline-block; padding: 2px 8px; border-radius: 999px; background: #eef4ff; color: #235cb8; }
    #ticket-modal { position: fixed; inset: 0; background: rgba(0,0,0,.35); display: none; align-items: center; justify-content: center; padding: 24px; }
    #ticket-modal.open { display: flex; }
    #ticket-panel { width: min(920px, 100%); max-height: 90vh; overflow: auto; background: #fff; border-radius: 18px; padding: 18px; }
    pre { white-space: pre-wrap; word-break: break-word; }
    @media (max-width: 1100px) { .grid { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <h1>ops-agent-copilot 管理页</h1>
  <p>审批、审计、工单详情都直接走当前 Go 主服务接口。</p>
  <div class="grid">
    <section class="card">
      <h2>审批列表</h2>
      <div class="toolbar">
        <select id="approval-status">
          <option value="">全部状态</option>
          <option value="pending" selected>pending</option>
          <option value="approved">approved</option>
          <option value="rejected">rejected</option>
          <option value="executed">executed</option>
          <option value="execution_failed">execution_failed</option>
        </select>
        <input id="approver-user-id" type="number" value="2" min="1" />
        <button id="refresh-approvals">刷新</button>
      </div>
      <table>
        <thead><tr><th>审批单</th><th>动作</th><th>目标</th><th>状态</th><th>trace_id</th><th>操作</th></tr></thead>
        <tbody id="approval-body"></tbody>
      </table>
    </section>
    <section class="card">
      <h2>审计日志</h2>
      <div class="toolbar">
        <input id="trace-id-input" placeholder="trace_id" />
        <select id="event-type-select"><option value="">全部事件</option></select>
        <button id="load-audit">查询</button>
      </div>
      <div id="audit-summary"></div>
      <div id="audit-list"></div>
    </section>
  </div>

  <div id="ticket-modal">
    <div id="ticket-panel">
      <div class="toolbar" style="justify-content:space-between">
        <h3 id="ticket-title">工单详情</h3>
        <button id="ticket-close">关闭</button>
      </div>
      <div id="ticket-content"></div>
    </div>
  </div>

  <script>
    const approvalBody = document.getElementById('approval-body');
    const auditList = document.getElementById('audit-list');
    const auditSummary = document.getElementById('audit-summary');
    const eventTypeSelect = document.getElementById('event-type-select');
    const ticketModal = document.getElementById('ticket-modal');

    function esc(v) { return String(v ?? '').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;'); }
    async function requestJson(url, options) {
      const resp = await fetch(url, options);
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.detail || JSON.stringify(data));
      return data;
    }

    async function loadApprovals() {
      const status = document.getElementById('approval-status').value;
      const params = new URLSearchParams({ limit: '20' });
      if (status) params.set('status', status);
      const data = await requestJson('/api/v1/approvals?' + params.toString());
      approvalBody.innerHTML = '';
      for (const item of data.items) {
        const actions = item.status === 'pending'
          ? '<button onclick="approveApproval(\'' + item.approval_no + '\')">通过</button><button onclick="rejectApproval(\'' + item.approval_no + '\')">拒绝</button>'
          : '<span class="pill">' + esc(item.status) + '</span>';
        approvalBody.insertAdjacentHTML('beforeend', '<tr><td>' + esc(item.approval_no) + '</td><td>' + esc(item.action_type) + '</td><td><button onclick="showTicket(\'' + esc(item.target_id) + '\')">' + esc(item.target_id) + '</button></td><td>' + esc(item.status) + '</td><td><button onclick="loadAuditByTrace(\'' + esc(item.trace_id) + '\')">' + esc(item.trace_id) + '</button></td><td>' + actions + '</td></tr>');
      }
      if (!data.items.length) approvalBody.innerHTML = '<tr><td colspan="6">暂无审批记录</td></tr>';
    }

    async function approveApproval(no) {
      const approver = Number(document.getElementById('approver-user-id').value || 2);
      await requestJson('/api/v1/approvals/' + encodeURIComponent(no) + '/approve', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ approver_user_id: approver }) });
      await loadApprovals();
    }

    async function rejectApproval(no) {
      const approver = Number(document.getElementById('approver-user-id').value || 2);
      const reason = prompt('请输入拒绝原因', '需要重新确认执行对象');
      if (!reason) return;
      await requestJson('/api/v1/approvals/' + encodeURIComponent(no) + '/reject', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ approver_user_id: approver, reason }) });
      await loadApprovals();
    }

    async function loadAudit(traceId='') {
      const params = new URLSearchParams();
      const actualTrace = traceId || document.getElementById('trace-id-input').value.trim();
      if (actualTrace) params.set('trace_id', actualTrace); else params.set('limit', '30');
      if (eventTypeSelect.value) params.set('event_type', eventTypeSelect.value);
      const data = await requestJson('/api/v1/audit?' + params.toString());
      eventTypeSelect.innerHTML = '<option value="">全部事件</option>' + (data.available_event_types || []).map(v => '<option value="' + esc(v) + '">' + esc(v) + '</option>').join('');
      if (data.event_type) eventTypeSelect.value = data.event_type;
      auditSummary.textContent = '共 ' + data.count + ' 条审计事件';
      auditList.innerHTML = (data.logs || []).map(item => '<div class="card" style="margin-top:12px"><div><span class="pill">' + esc(item.event_type) + '</span> ' + esc(item.created_at) + '</div><pre>' + esc(JSON.stringify(item.event_data, null, 2)) + '</pre></div>').join('');
      if (!data.logs.length) auditList.innerHTML = '<p>暂无日志</p>';
    }

    async function showTicket(ticketNo) {
      const data = await requestJson('/api/v1/tickets/' + encodeURIComponent(ticketNo));
      ticketModal.classList.add('open');
      document.getElementById('ticket-title').textContent = '工单详情 ' + data.ticket.ticket_no;
      document.getElementById('ticket-content').innerHTML =
        '<h4>' + esc(data.ticket.title) + '</h4>' +
        '<pre>' + esc(JSON.stringify(data.ticket, null, 2)) + '</pre>' +
        '<h4>评论</h4><pre>' + esc(JSON.stringify(data.comments, null, 2)) + '</pre>' +
        '<h4>操作</h4><pre>' + esc(JSON.stringify(data.actions, null, 2)) + '</pre>';
    }

    function loadAuditByTrace(traceId) {
      document.getElementById('trace-id-input').value = traceId;
      loadAudit(traceId);
    }

    document.getElementById('refresh-approvals').addEventListener('click', loadApprovals);
    document.getElementById('load-audit').addEventListener('click', () => loadAudit(''));
    document.getElementById('ticket-close').addEventListener('click', () => ticketModal.classList.remove('open'));
    ticketModal.addEventListener('click', (e) => { if (e.target === ticketModal) ticketModal.classList.remove('open'); });
    window.approveApproval = approveApproval;
    window.rejectApproval = rejectApproval;
    window.loadAuditByTrace = loadAuditByTrace;
    window.showTicket = showTicket;
    loadApprovals();
    loadAudit('');
  </script>
</body>
</html>`

const docsHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ops-agent-copilot docs</title>
  <style>
    body { font-family: "Segoe UI", "PingFang SC", sans-serif; margin: 0; padding: 32px; background: #f7f9fc; color: #1b2430; }
    section { background: #fff; border: 1px solid #dce3ef; border-radius: 16px; padding: 18px; margin-bottom: 16px; }
    code, pre { font-family: Consolas, monospace; }
    pre { background: #0f172a; color: #e2e8f0; padding: 14px; border-radius: 12px; overflow: auto; }
  </style>
</head>
<body>
  <h1>ops-agent-copilot API Docs</h1>
  <section>
    <h2>Base Endpoints</h2>
    <ul>
      <li><code>GET /healthz</code></li>
      <li><code>GET /metrics</code></li>
      <li><code>GET /admin</code></li>
      <li><code>GET /docs</code></li>
    </ul>
  </section>
  <section>
    <h2>Business Endpoints</h2>
    <ul>
      <li><code>POST /api/v1/chat</code></li>
      <li><code>GET /api/v1/approvals</code></li>
      <li><code>GET /api/v1/approvals/{approval_no}</code></li>
      <li><code>POST /api/v1/approvals/{approval_no}/approve</code></li>
      <li><code>POST /api/v1/approvals/{approval_no}/reject</code></li>
      <li><code>GET /api/v1/audit</code></li>
      <li><code>GET /api/v1/tickets/{ticket_no}</code></li>
    </ul>
  </section>
  <section>
    <h2>Chat Example</h2>
    <pre>{
  "session_id": "demo_metric",
  "user_id": 1,
  "message": "最近7天北京区退款率最高的类目是什么？"
}</pre>
  </section>
  <section>
    <h2>Approval Example</h2>
    <pre>{
  "approver_user_id": 2
}</pre>
  </section>
</body>
</html>`
