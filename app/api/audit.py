from fastapi import APIRouter, Depends, Query

from app.api.deps import get_audit_service
from app.services.audit_service import AuditService


router = APIRouter(tags=['audit'])


@router.get('/audit')
async def list_audit_logs(
    trace_id: str | None = Query(default=None),
    event_type: str | None = Query(default=None),
    limit: int = Query(default=50, ge=1, le=200),
    audit_service: AuditService = Depends(get_audit_service),
) -> dict:
    if trace_id:
        logs = await audit_service.audit_repo.list_by_trace_id(trace_id, event_type=event_type)
        tool_calls = await audit_service.audit_repo.list_tool_calls_by_trace_id(trace_id)
    else:
        logs = await audit_service.audit_repo.list_recent(limit=limit, event_type=event_type)
        tool_calls = []
    available_event_types = await audit_service.audit_repo.list_event_types()

    return {
        'trace_id': trace_id,
        'event_type': event_type,
        'count': len(logs),
        'available_event_types': available_event_types,
        'logs': [
            {
                'id': log.id,
                'trace_id': log.trace_id,
                'session_id': log.session_id,
                'user_id': log.user_id,
                'event_type': log.event_type,
                'event_data': log.event_data,
                'created_at': log.created_at.isoformat(),
            }
            for log in logs
        ],
        'tool_calls': [
            {
                'tool_name': item.tool_name,
                'tool_type': item.tool_type,
                'success': item.success,
                'latency_ms': item.latency_ms,
                'error_message': item.error_message,
                'input_payload': item.input_payload,
                'output_payload': item.output_payload,
                'created_at': item.created_at.isoformat(),
            }
            for item in tool_calls
        ],
    }
