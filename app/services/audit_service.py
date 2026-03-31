import logging
from typing import Any

from app.db.models import AuditLog, ToolCallLog
from app.repositories.audit_repo import AuditRepository


logger = logging.getLogger(__name__)


class AuditService:
    def __init__(self, audit_repo: AuditRepository) -> None:
        self.audit_repo = audit_repo

    async def log_event(
        self,
        trace_id: str,
        event_type: str,
        event_data: dict[str, Any],
        session_id: str | None = None,
        user_id: int | None = None,
    ) -> None:
        await self.audit_repo.create_audit_log(
            AuditLog(
                trace_id=trace_id,
                session_id=session_id,
                user_id=user_id,
                event_type=event_type,
                event_data=event_data,
            )
        )
        logger.info(event_type, extra={'trace_id': trace_id, 'session_id': session_id, 'user_id': user_id})

    async def log_tool_call(
        self,
        trace_id: str,
        session_id: str,
        tool_name: str,
        tool_type: str,
        input_payload: dict[str, Any],
        success: bool,
        latency_ms: int,
        output_payload: dict[str, Any] | None = None,
        error_message: str | None = None,
    ) -> None:
        await self.audit_repo.create_tool_call_log(
            ToolCallLog(
                trace_id=trace_id,
                session_id=session_id,
                tool_name=tool_name,
                tool_type=tool_type,
                input_payload=input_payload,
                output_payload=output_payload,
                success=success,
                error_message=error_message,
                latency_ms=latency_ms,
            )
        )
