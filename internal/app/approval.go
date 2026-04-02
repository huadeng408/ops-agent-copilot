package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

const (
	ApprovalStatusPending         = "pending"
	ApprovalStatusApproved        = "approved"
	ApprovalStatusRejected        = "rejected"
	ApprovalStatusExecuted        = "executed"
	ApprovalStatusExecutionFailed = "execution_failed"
)

var activeOrCompletedApprovalStatuses = map[string]struct{}{
	ApprovalStatusPending:  {},
	ApprovalStatusApproved: {},
	ApprovalStatusExecuted: {},
}

var approvalAllowedTransitions = map[string]map[string]struct{}{
	ApprovalStatusPending: {
		ApprovalStatusApproved: {},
		ApprovalStatusRejected: {},
	},
	ApprovalStatusApproved: {
		ApprovalStatusExecuted:        {},
		ApprovalStatusExecutionFailed: {},
	},
	ApprovalStatusRejected:        {},
	ApprovalStatusExecuted:        {},
	ApprovalStatusExecutionFailed: {},
}

type ApprovalService struct {
	approvalRepo *ApprovalRepository
	ticketRepo   *TicketRepository
	userRepo     *UserRepository
	verifier     *VerifierService
	auditService *AuditService
	metrics      *MetricsRecorder
}

func NewApprovalService(approvalRepo *ApprovalRepository, ticketRepo *TicketRepository, userRepo *UserRepository, verifier *VerifierService, auditService *AuditService, metrics *MetricsRecorder) *ApprovalService {
	return &ApprovalService{
		approvalRepo: approvalRepo,
		ticketRepo:   ticketRepo,
		userRepo:     userRepo,
		verifier:     verifier,
		auditService: auditService,
		metrics:      metrics,
	}
}

func (s *ApprovalService) CreateProposal(ctx context.Context, sessionID string, traceID string, requestedBy User, actionType string, targetType string, targetID string, payload map[string]any, reason string) (*Approval, error) {
	ctx, span := StartSpan(ctx, "approval.create_proposal")
	defer span.End()
	span.SetAttributes(
		attribute.String("app.trace_id", traceID),
		attribute.String("approval.action_type", actionType),
		attribute.String("approval.target_type", targetType),
		attribute.String("approval.target_id", targetID),
		attribute.Int64("approval.requested_by", requestedBy.ID),
	)
	if _, err := s.verifier.VerifyProposal(ctx, actionType, payload, reason, requestedBy); err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	idempotencyKey := buildProposalIDKey(sessionID, requestedBy.ID, actionType, targetType, targetID, payload, reason)
	existing, err := s.approvalRepo.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	if existing != nil {
		if _, ok := activeOrCompletedApprovalStatuses[existing.Status]; ok {
			_ = s.auditService.LogEvent(ctx, traceID, sessionID, &requestedBy.ID, "proposal_reused", map[string]any{
				"approval_no":     existing.ApprovalNo,
				"idempotency_key": existing.IdempotencyKey,
			})
			return existing, nil
		}
		idempotencyKey = buildRetryIDKey(idempotencyKey, traceID)
	}

	approval := &Approval{
		ApprovalNo:     generateApprovalNo(),
		IdempotencyKey: idempotencyKey,
		SessionID:      sessionID,
		TraceID:        traceID,
		ActionType:     actionType,
		TargetType:     targetType,
		TargetID:       targetID,
		Payload:        MustJSON(payload),
		Reason:         reason,
		Status:         ApprovalStatusPending,
		RequestedBy:    requestedBy.ID,
		Version:        1,
	}
	if err := s.approvalRepo.Create(ctx, approval); err != nil {
		reused, reusedErr := s.approvalRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if reusedErr == nil && reused != nil {
			return reused, nil
		}
		RecordSpanError(span, err)
		return nil, err
	}
	span.SetAttributes(attribute.String("approval.approval_no", approval.ApprovalNo))
	_ = s.auditService.LogEvent(ctx, traceID, sessionID, &requestedBy.ID, "proposal_created", map[string]any{
		"approval_no":     approval.ApprovalNo,
		"action_type":     actionType,
		"target_id":       targetID,
		"idempotency_key": approval.IdempotencyKey,
	})
	return approval, nil
}

func (s *ApprovalService) Approve(ctx context.Context, approvalNo string, approver User) (*Approval, map[string]any, error) {
	ctx, span := StartSpan(ctx, "approval.approve")
	defer span.End()
	span.SetAttributes(
		attribute.String("approval.approval_no", approvalNo),
		attribute.Int64("approval.approver_user_id", approver.ID),
	)
	if err := EnsureCanApprove(approver); err != nil {
		RecordSpanError(span, err)
		return nil, nil, err
	}
	approval, err := s.approvalRepo.GetByNo(ctx, approvalNo)
	if err != nil {
		RecordSpanError(span, err)
		return nil, nil, err
	}
	if approval == nil {
		return nil, nil, NewNotFound("审批单不存在: " + approvalNo)
	}
	span.SetAttributes(
		attribute.String("app.trace_id", approval.TraceID),
		attribute.String("approval.action_type", approval.ActionType),
		attribute.String("approval.status_before", approval.Status),
	)
	switch approval.Status {
	case ApprovalStatusExecuted:
		span.SetAttributes(attribute.Bool("approval.idempotent_hit", true))
		return approval, ParseJSONMap(approval.ExecutionResult), nil
	case ApprovalStatusRejected:
		return nil, nil, NewConflict("审批单已被拒绝，不能再次审批")
	case ApprovalStatusExecutionFailed:
		return nil, nil, NewConflict("审批单执行失败，请重新创建新的审批单")
	case ApprovalStatusApproved:
		if approval.ExecutionResult != "" {
			span.SetAttributes(attribute.Bool("approval.idempotent_hit", true))
			return approval, ParseJSONMap(approval.ExecutionResult), nil
		}
		return nil, nil, NewConflict("审批单正在执行中，请稍后重试")
	}

	previousVersion := approval.Version
	if err := ensureTransition(approval.Status, ApprovalStatusApproved); err != nil {
		RecordSpanError(span, err)
		return nil, nil, err
	}
	approval.Status = ApprovalStatusApproved
	approval.ApprovedBy = &approver.ID
	now := time.Now()
	approval.ApprovedAt = &now
	approval.RejectedReason = ""
	approval.ExecutionError = ""
	if err := s.approvalRepo.UpdateWithVersion(ctx, approval, previousVersion); err != nil {
		RecordSpanError(span, err)
		return nil, nil, err
	}
	_ = s.auditService.LogEvent(ctx, approval.TraceID, approval.SessionID, &approver.ID, "approval_approved", map[string]any{
		"approval_no": approval.ApprovalNo,
		"action_type": approval.ActionType,
		"version":     approval.Version,
	})
	_ = s.logStatusChange(ctx, approval, ApprovalStatusPending, ApprovalStatusApproved, approver.ID)

	result, execErr := s.executeApprovedAction(ctx, approval, approver.ID)
	if execErr != nil {
		previousVersion = approval.Version
		approval.Status = ApprovalStatusExecutionFailed
		approval.ExecutionError = execErr.Error()
		if err := s.approvalRepo.UpdateWithVersion(ctx, approval, previousVersion); err != nil {
			RecordSpanError(span, err)
			return nil, nil, err
		}
		_ = s.auditService.LogEvent(ctx, approval.TraceID, approval.SessionID, &approver.ID, "write_execution_failed", map[string]any{
			"approval_no":   approval.ApprovalNo,
			"error_message": approval.ExecutionError,
		})
		_ = s.logStatusChange(ctx, approval, ApprovalStatusApproved, ApprovalStatusExecutionFailed, approver.ID)
		RecordSpanError(span, execErr)
		return nil, nil, execErr
	}

	previousVersion = approval.Version
	approval.Status = ApprovalStatusExecuted
	executedAt := time.Now()
	approval.ExecutedAt = &executedAt
	approval.ExecutionResult = MustJSON(result)
	approval.ExecutionError = ""
	if err := s.approvalRepo.UpdateWithVersion(ctx, approval, previousVersion); err != nil {
		RecordSpanError(span, err)
		return nil, nil, err
	}
	_ = s.auditService.LogEvent(ctx, approval.TraceID, approval.SessionID, &approver.ID, "write_executed", map[string]any{
		"approval_no": approval.ApprovalNo,
		"result":      result,
		"version":     approval.Version,
	})
	_ = s.logStatusChange(ctx, approval, ApprovalStatusApproved, ApprovalStatusExecuted, approver.ID)
	span.SetAttributes(attribute.String("approval.status_after", approval.Status))
	return approval, result, nil
}

func (s *ApprovalService) Reject(ctx context.Context, approvalNo string, approver User, reason string) (*Approval, error) {
	ctx, span := StartSpan(ctx, "approval.reject")
	defer span.End()
	span.SetAttributes(
		attribute.String("approval.approval_no", approvalNo),
		attribute.Int64("approval.approver_user_id", approver.ID),
	)
	if err := EnsureCanApprove(approver); err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	approval, err := s.approvalRepo.GetByNo(ctx, approvalNo)
	if err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	if approval == nil {
		return nil, NewNotFound("审批单不存在: " + approvalNo)
	}
	switch approval.Status {
	case ApprovalStatusRejected:
		return approval, nil
	case ApprovalStatusExecuted:
		return nil, NewConflict("审批单已执行完成，不能再拒绝")
	case ApprovalStatusApproved:
		return nil, NewConflict("审批单已批准并进入执行阶段，不能再拒绝")
	case ApprovalStatusExecutionFailed:
		return nil, NewConflict("审批单已执行失败，不能再拒绝，请重新发起新审批单")
	}
	previousVersion := approval.Version
	if err := ensureTransition(approval.Status, ApprovalStatusRejected); err != nil {
		return nil, err
	}
	approval.Status = ApprovalStatusRejected
	approval.ApprovedBy = &approver.ID
	now := time.Now()
	approval.ApprovedAt = &now
	approval.RejectedReason = reason
	if err := s.approvalRepo.UpdateWithVersion(ctx, approval, previousVersion); err != nil {
		RecordSpanError(span, err)
		return nil, err
	}
	_ = s.auditService.LogEvent(ctx, approval.TraceID, approval.SessionID, &approver.ID, "approval_rejected", map[string]any{
		"approval_no": approval.ApprovalNo,
		"reason":      reason,
		"version":     approval.Version,
	})
	_ = s.logStatusChange(ctx, approval, ApprovalStatusPending, ApprovalStatusRejected, approver.ID)
	span.SetAttributes(attribute.String("approval.status_after", approval.Status))
	return approval, nil
}

func (s *ApprovalService) executeApprovedAction(ctx context.Context, approval *Approval, operatorID int64) (map[string]any, error) {
	ctx, span := StartSpan(ctx, "approval.execute_action")
	defer span.End()
	span.SetAttributes(
		attribute.String("approval.approval_no", approval.ApprovalNo),
		attribute.String("approval.action_type", approval.ActionType),
		attribute.String("approval.target_id", approval.TargetID),
		attribute.Int64("approval.operator_id", operatorID),
	)
	payload := ParseJSONMap(approval.Payload)
	switch approval.ActionType {
	case "assign_ticket":
		assignee, err := s.userRepo.GetByDisplayName(ctx, asString(payload["assignee_name"]))
		if err != nil {
			RecordSpanError(span, err)
			return nil, err
		}
		return s.ticketRepo.AssignTicket(ctx, asString(payload["ticket_no"]), *assignee, operatorID, approval.ID, approval.TraceID)
	case "add_ticket_comment":
		return s.ticketRepo.AddTicketComment(ctx, asString(payload["ticket_no"]), asString(payload["comment_text"]), operatorID, approval.ID, approval.TraceID)
	case "escalate_ticket":
		return s.ticketRepo.EscalateTicket(ctx, asString(payload["ticket_no"]), asString(payload["new_priority"]), operatorID, approval.ID, approval.TraceID)
	default:
		return nil, NewValidation("未知 action_type: " + approval.ActionType)
	}
}

func (s *ApprovalService) logStatusChange(ctx context.Context, approval *Approval, previousStatus string, nextStatus string, userID int64) error {
	s.metrics.RecordApprovalTransition(previousStatus, nextStatus)
	if nextStatus == ApprovalStatusRejected || nextStatus == ApprovalStatusExecuted {
		turnaround := time.Since(approval.CreatedAt).Seconds()
		if turnaround < 0 {
			turnaround = 0
		}
		s.metrics.RecordApprovalTurnaround(approval.ActionType, nextStatus, turnaround)
	}
	return s.auditService.LogEvent(ctx, approval.TraceID, approval.SessionID, &userID, "approval_status_changed", map[string]any{
		"approval_no": approval.ApprovalNo,
		"from_status": previousStatus,
		"to_status":   nextStatus,
		"version":     approval.Version,
	})
}

func ensureTransition(currentStatus string, nextStatus string) error {
	allowed := approvalAllowedTransitions[currentStatus]
	if _, ok := allowed[nextStatus]; ok {
		return nil
	}
	return NewConflict(fmt.Sprintf("审批单状态不允许从 %s 迁移到 %s", currentStatus, nextStatus))
}

func buildProposalIDKey(sessionID string, requestedBy int64, actionType string, targetType string, targetID string, payload map[string]any, reason string) string {
	canonical := MustJSON(map[string]any{
		"session_id":   sessionID,
		"requested_by": requestedBy,
		"action_type":  actionType,
		"target_type":  targetType,
		"target_id":    targetID,
		"payload":      payload,
		"reason":       reason,
	})
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func buildRetryIDKey(baseKey string, traceID string) string {
	sum := sha256.Sum256([]byte(traceID))
	suffix := hex.EncodeToString(sum[:])[:16]
	if len(baseKey) > 79 {
		baseKey = baseKey[:79]
	}
	return baseKey + "-" + suffix
}

func generateApprovalNo() string {
	return "APR" + time.Now().UTC().Format("20060102150405000000")[:20]
}
