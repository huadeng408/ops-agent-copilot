from datetime import UTC, datetime
import hashlib
import json

from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm.exc import StaleDataError

from app.core.exceptions import ConflictError, ValidationAppError
from app.core.observability import metrics
from app.core.security import ensure_can_approve
from app.db.models import Approval, User
from app.repositories.approval_repo import ApprovalRepository
from app.repositories.ticket_repo import TicketRepository
from app.services.approval_state_machine import (
    APPROVAL_STATUS_APPROVED,
    APPROVAL_STATUS_EXECUTED,
    APPROVAL_STATUS_EXECUTION_FAILED,
    APPROVAL_STATUS_PENDING,
    APPROVAL_STATUS_REJECTED,
    ensure_transition,
)
from app.services.audit_service import AuditService
from app.services.verifier_service import VerifierService


ACTIVE_OR_COMPLETED_APPROVAL_STATUSES = {
    APPROVAL_STATUS_PENDING,
    APPROVAL_STATUS_APPROVED,
    APPROVAL_STATUS_EXECUTED,
}


class ApprovalService:
    def __init__(
        self,
        approval_repo: ApprovalRepository,
        ticket_repo: TicketRepository,
        verifier: VerifierService,
        audit_service: AuditService,
        session,
    ) -> None:
        self.approval_repo = approval_repo
        self.ticket_repo = ticket_repo
        self.verifier = verifier
        self.audit_service = audit_service
        self.session = session

    async def create_proposal(
        self,
        session_id: str,
        trace_id: str,
        requested_by: User,
        action_type: str,
        target_type: str,
        target_id: str,
        payload: dict,
        reason: str,
    ) -> Approval:
        verifier_result = await self.verifier.verify_proposal(action_type, payload, reason, requested_by)
        if not verifier_result.passed:
            raise ValidationAppError(verifier_result.message)

        idempotency_key = self._build_proposal_idempotency_key(
            session_id=session_id,
            requested_by=requested_by.id,
            action_type=action_type,
            target_type=target_type,
            target_id=target_id,
            payload=payload,
            reason=reason,
        )
        existing = await self.approval_repo.get_by_idempotency_key(idempotency_key)
        if existing and existing.status in ACTIVE_OR_COMPLETED_APPROVAL_STATUSES:
            await self.audit_service.log_event(
                trace_id=trace_id,
                session_id=session_id,
                user_id=requested_by.id,
                event_type='proposal_reused',
                event_data={'approval_no': existing.approval_no, 'idempotency_key': existing.idempotency_key},
            )
            return existing

        if existing:
            idempotency_key = self._build_retry_idempotency_key(idempotency_key, trace_id)

        approval = Approval(
            approval_no=self._generate_approval_no(),
            idempotency_key=idempotency_key,
            session_id=session_id,
            trace_id=trace_id,
            action_type=action_type,
            target_type=target_type,
            target_id=target_id,
            payload=payload,
            reason=reason,
            status=APPROVAL_STATUS_PENDING,
            requested_by=requested_by.id,
        )
        savepoint = await self.session.begin_nested()
        try:
            await self.approval_repo.create(approval)
            await savepoint.commit()
        except IntegrityError:
            await savepoint.rollback()
            reused = await self.approval_repo.get_by_idempotency_key(idempotency_key)
            if reused is not None:
                return reused
            raise

        await self.audit_service.log_event(
            trace_id=trace_id,
            session_id=session_id,
            user_id=requested_by.id,
            event_type='proposal_created',
            event_data={
                'approval_no': approval.approval_no,
                'action_type': action_type,
                'target_id': target_id,
                'idempotency_key': approval.idempotency_key,
            },
        )
        return approval

    async def approve(self, approval_no: str, approver: User) -> tuple[Approval, dict]:
        ensure_can_approve(approver)
        approval = await self.approval_repo.get_by_no(approval_no)

        if approval.status == APPROVAL_STATUS_EXECUTED:
            return approval, approval.execution_result or {}
        if approval.status == APPROVAL_STATUS_REJECTED:
            raise ConflictError('审批单已被拒绝，不能再次审批')
        if approval.status == APPROVAL_STATUS_EXECUTION_FAILED:
            raise ConflictError('审批单执行失败，请重新创建新的审批单')
        if approval.status == APPROVAL_STATUS_APPROVED:
            if approval.execution_result:
                return approval, approval.execution_result
            raise ConflictError('审批单正在执行中，请稍后重试')

        self._transition_status(approval, APPROVAL_STATUS_APPROVED)
        approval.approved_by = approver.id
        approval.approved_at = self._now()
        approval.rejected_reason = None
        approval.execution_error = None
        try:
            await self.session.flush()
        except StaleDataError as exc:
            raise ConflictError('审批单已被其他审批请求更新，请刷新后重试') from exc

        await self.audit_service.log_event(
            trace_id=approval.trace_id,
            session_id=approval.session_id,
            user_id=approver.id,
            event_type='approval_approved',
            event_data={'approval_no': approval.approval_no, 'action_type': approval.action_type, 'version': approval.version},
        )
        await self._log_status_change(approval, APPROVAL_STATUS_PENDING, APPROVAL_STATUS_APPROVED, approver.id)

        try:
            result = await self._execute_approved_action(approval, approver.id)
        except Exception as exc:
            self._transition_status(approval, APPROVAL_STATUS_EXECUTION_FAILED)
            approval.execution_error = str(exc)[:255]
            try:
                await self.session.flush()
            except StaleDataError as stale_exc:
                raise ConflictError('审批单执行状态被并发更新，请刷新后重试') from stale_exc
            await self.audit_service.log_event(
                trace_id=approval.trace_id,
                session_id=approval.session_id,
                user_id=approver.id,
                event_type='write_execution_failed',
                event_data={'approval_no': approval.approval_no, 'error_message': approval.execution_error},
            )
            await self._log_status_change(approval, APPROVAL_STATUS_APPROVED, APPROVAL_STATUS_EXECUTION_FAILED, approver.id)
            raise

        self._transition_status(approval, APPROVAL_STATUS_EXECUTED)
        approval.executed_at = self._now()
        approval.execution_result = result
        approval.execution_error = None
        try:
            await self.session.flush()
        except StaleDataError as exc:
            raise ConflictError('审批单执行结果被并发更新，请刷新后重试') from exc

        await self.audit_service.log_event(
            trace_id=approval.trace_id,
            session_id=approval.session_id,
            user_id=approver.id,
            event_type='write_executed',
            event_data={'approval_no': approval.approval_no, 'result': result, 'version': approval.version},
        )
        await self._log_status_change(approval, APPROVAL_STATUS_APPROVED, APPROVAL_STATUS_EXECUTED, approver.id)
        return approval, result

    async def reject(self, approval_no: str, approver: User, reason: str) -> Approval:
        ensure_can_approve(approver)
        approval = await self.approval_repo.get_by_no(approval_no)

        if approval.status == APPROVAL_STATUS_REJECTED:
            return approval
        if approval.status == APPROVAL_STATUS_EXECUTED:
            raise ConflictError('审批单已执行完成，不能再拒绝')
        if approval.status == APPROVAL_STATUS_APPROVED:
            raise ConflictError('审批单已批准并进入执行阶段，不能再拒绝')
        if approval.status == APPROVAL_STATUS_EXECUTION_FAILED:
            raise ConflictError('审批单已执行失败，不能再拒绝，请重新发起新审批单')

        self._transition_status(approval, APPROVAL_STATUS_REJECTED)
        approval.approved_by = approver.id
        approval.approved_at = self._now()
        approval.rejected_reason = reason
        try:
            await self.session.flush()
        except StaleDataError as exc:
            raise ConflictError('审批单已被其他请求更新，请刷新后重试') from exc

        await self.audit_service.log_event(
            trace_id=approval.trace_id,
            session_id=approval.session_id,
            user_id=approver.id,
            event_type='approval_rejected',
            event_data={'approval_no': approval.approval_no, 'reason': reason, 'version': approval.version},
        )
        await self._log_status_change(approval, APPROVAL_STATUS_PENDING, APPROVAL_STATUS_REJECTED, approver.id)
        return approval

    async def _execute_approved_action(self, approval: Approval, operator_id: int) -> dict:
        payload = approval.payload
        if approval.action_type == 'assign_ticket':
            assignee = await self._find_user_by_display_name(payload['assignee_name'])
            return await self.ticket_repo.assign_ticket(
                ticket_no=payload['ticket_no'],
                assignee=assignee,
                operator_id=operator_id,
                approval_id=approval.id,
                trace_id=approval.trace_id,
            )
        if approval.action_type == 'add_ticket_comment':
            return await self.ticket_repo.add_ticket_comment(
                ticket_no=payload['ticket_no'],
                comment_text=payload['comment_text'],
                operator_id=operator_id,
                approval_id=approval.id,
                trace_id=approval.trace_id,
            )
        if approval.action_type == 'escalate_ticket':
            return await self.ticket_repo.escalate_ticket(
                ticket_no=payload['ticket_no'],
                new_priority=payload['new_priority'],
                operator_id=operator_id,
                approval_id=approval.id,
                trace_id=approval.trace_id,
            )
        raise ValidationAppError(f'未知 action_type: {approval.action_type}')

    async def _find_user_by_display_name(self, display_name: str) -> User:
        result = await self.session.execute(select(User).where(User.display_name == display_name))
        user = result.scalar_one_or_none()
        if user is None:
            raise ValidationAppError(f'找不到分派对象: {display_name}')
        return user

    async def _log_status_change(self, approval: Approval, previous_status: str, next_status: str, user_id: int) -> None:
        metrics.record_approval_transition(from_status=previous_status, to_status=next_status)
        if next_status in {APPROVAL_STATUS_REJECTED, APPROVAL_STATUS_EXECUTED}:
            turnaround_seconds = max((self._now() - approval.created_at).total_seconds(), 0.0)
            metrics.record_approval_turnaround(
                action_type=approval.action_type,
                final_status=next_status,
                seconds=turnaround_seconds,
            )
        await self.audit_service.log_event(
            trace_id=approval.trace_id,
            session_id=approval.session_id,
            user_id=user_id,
            event_type='approval_status_changed',
            event_data={
                'approval_no': approval.approval_no,
                'from_status': previous_status,
                'to_status': next_status,
                'version': approval.version,
            },
        )

    def _transition_status(self, approval: Approval, next_status: str) -> None:
        ensure_transition(approval, next_status)
        approval.status = next_status

    def _build_proposal_idempotency_key(
        self,
        *,
        session_id: str,
        requested_by: int,
        action_type: str,
        target_type: str,
        target_id: str,
        payload: dict,
        reason: str,
    ) -> str:
        canonical = json.dumps(
            {
                'session_id': session_id,
                'requested_by': requested_by,
                'action_type': action_type,
                'target_type': target_type,
                'target_id': target_id,
                'payload': payload,
                'reason': reason,
            },
            ensure_ascii=False,
            sort_keys=True,
            separators=(',', ':'),
        )
        return hashlib.sha256(canonical.encode('utf-8')).hexdigest()

    def _build_retry_idempotency_key(self, base_key: str, trace_id: str) -> str:
        suffix = hashlib.sha256(trace_id.encode('utf-8')).hexdigest()[:16]
        return f'{base_key[:79]}-{suffix}'

    def _generate_approval_no(self) -> str:
        return f"APR{datetime.now(UTC).strftime('%Y%m%d%H%M%S%f')[:20]}"

    def _now(self) -> datetime:
        return datetime.now(UTC).replace(tzinfo=None)
