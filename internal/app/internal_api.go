package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jmoiron/sqlx"
)

type internalToolInvokeRequest struct {
	TraceID   string         `json:"trace_id"`
	SessionID string         `json:"session_id"`
	UserID    int64          `json:"user_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

type internalProposalRequest struct {
	TraceID    string         `json:"trace_id"`
	SessionID  string         `json:"session_id"`
	UserID     int64          `json:"user_id"`
	ActionType string         `json:"action_type"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Payload    map[string]any `json:"payload"`
	Reason     string         `json:"reason"`
}

func (a *Application) internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(a.Config.InternalAPIKey) == "" {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimSpace(r.Header.Get("X-Internal-API-Key"))
		if token != a.Config.InternalAPIKey {
			WriteError(w, NewPermissionDenied("internal api key invalid"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Application) handleInternalToolInvoke(w http.ResponseWriter, r *http.Request) {
	var payload internalToolInvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, NewValidation("请求体格式不合法"))
		return
	}

	a.withTx(w, r, func(ctx context.Context, tx *sqlx.Tx, _ *requestScope) error {
		scope := a.newRequestScopeWithConfig(tx, a.Config)
		user, err := scope.userRepo.GetByID(ctx, payload.UserID)
		if err != nil {
			return err
		}
		result, record, err := scope.toolRegistry.Invoke(ctx, payload.ToolName, scope.agentService.toolContext(payload.TraceID, payload.SessionID, *user), payload.Arguments)
		if err != nil {
			return err
		}
		RespondJSON(w, http.StatusOK, map[string]any{
			"tool_name":         record.ToolName,
			"tool_type":         record.ToolType,
			"success":           record.Success,
			"latency_ms":        record.LatencyMS,
			"message":           result.Message,
			"data":              result.Data,
			"rendered_answer":   renderToolAnswer(payload.ToolName, result.Data),
			"requires_approval": result.RequiresApproval,
			"action_type":       asString(result.Data["action_type"]),
			"target_type":       asString(result.Data["target_type"]),
			"target_id":         asString(result.Data["target_id"]),
			"proposal_payload":  mapValue(result.Data["payload"]),
			"proposal_reason":   asString(result.Data["reason"]),
		})
		return nil
	})
}

func (a *Application) handleInternalCreateProposal(w http.ResponseWriter, r *http.Request) {
	var payload internalProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, NewValidation("请求体格式不合法"))
		return
	}

	a.withTx(w, r, func(ctx context.Context, tx *sqlx.Tx, _ *requestScope) error {
		scope := a.newRequestScopeWithConfig(tx, a.Config)
		user, err := scope.userRepo.GetByID(ctx, payload.UserID)
		if err != nil {
			return err
		}
		proposal, err := scope.approvalService.CreateProposal(
			ctx,
			payload.SessionID,
			payload.TraceID,
			*user,
			payload.ActionType,
			payload.TargetType,
			payload.TargetID,
			payload.Payload,
			payload.Reason,
		)
		if err != nil {
			return err
		}
		RespondJSON(w, http.StatusOK, map[string]any{
			"approval_no": proposal.ApprovalNo,
			"action_type": proposal.ActionType,
			"target_id":   proposal.TargetID,
			"payload":     ParseJSONMap(proposal.Payload),
		})
		return nil
	})
}

func (a *Application) handleInternalDailyReport(w http.ResponseWriter, r *http.Request) {
	a.withTx(w, r, func(ctx context.Context, tx *sqlx.Tx, _ *requestScope) error {
		scope := a.newRequestScopeWithConfig(tx, a.Config)
		report, err := scope.reportService.GenerateDailyReport(ctx)
		if err != nil {
			return err
		}
		RespondJSON(w, http.StatusOK, report)
		return nil
	})
}
