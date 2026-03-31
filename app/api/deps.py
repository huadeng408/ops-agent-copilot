from fastapi import Depends
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.config import Settings, get_settings
from app.core.exceptions import NotFoundError
from app.db.models import User
from app.db.session import get_db_session
from app.repositories.approval_repo import ApprovalRepository
from app.repositories.audit_repo import AuditRepository
from app.repositories.metric_repo import MetricRepository
from app.repositories.release_repo import ReleaseRepository
from app.repositories.session_repo import SessionRepository
from app.repositories.ticket_repo import TicketRepository
from app.services.agent_service import AgentService
from app.services.anomaly_service import AnomalyService
from app.services.approval_service import ApprovalService
from app.services.audit_service import AuditService
from app.services.memory_service import MemoryService
from app.services.report_service import ReportService
from app.services.tool_registry import ToolRegistry
from app.services.verifier_service import VerifierService
from app.tools.base import ToolContext
from app.tools.readonly_tools import (
    FindRefundAnomaliesTool,
    AnalyzeOperationalAnomalyTool,
    GetRecentReleasesTool,
    GetTicketCommentsTool,
    GetTicketDetailTool,
    ListSlaBreachedTicketsTool,
    QueryRefundMetricsTool,
    RunReadonlySqlTool,
)
from app.tools.write_tools import ProposeAddTicketCommentTool, ProposeAssignTicketTool, ProposeEscalateTicketTool


def get_app_settings() -> Settings:
    return get_settings()


async def get_user(user_id: int, session: AsyncSession) -> User:
    result = await session.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if user is None:
        raise NotFoundError(f'用户不存在: {user_id}')
    return user


def build_tool_registry(session: AsyncSession, audit_service: AuditService) -> ToolRegistry:
    registry = ToolRegistry(audit_service=audit_service)
    registry.register(QueryRefundMetricsTool())
    registry.register(FindRefundAnomaliesTool())
    registry.register(AnalyzeOperationalAnomalyTool())
    registry.register(ListSlaBreachedTicketsTool())
    registry.register(GetTicketDetailTool())
    registry.register(GetTicketCommentsTool())
    registry.register(GetRecentReleasesTool())
    registry.register(RunReadonlySqlTool(session))
    registry.register(ProposeAssignTicketTool())
    registry.register(ProposeAddTicketCommentTool())
    registry.register(ProposeEscalateTicketTool())
    return registry


async def get_agent_service(
    session: AsyncSession = Depends(get_db_session),
    settings: Settings = Depends(get_app_settings),
) -> AgentService:
    metric_repo = MetricRepository(session)
    ticket_repo = TicketRepository(session)
    release_repo = ReleaseRepository(session)
    approval_repo = ApprovalRepository(session)
    audit_repo = AuditRepository(session)
    session_repo = SessionRepository(session)
    audit_service = AuditService(audit_repo)
    verifier = VerifierService(ticket_repo)
    memory_service = MemoryService(session_repo, settings.keep_recent_message_count)
    report_service = ReportService(metric_repo, ticket_repo, release_repo)
    anomaly_service = AnomalyService(metric_repo, ticket_repo, release_repo)
    approval_service = ApprovalService(approval_repo, ticket_repo, verifier, audit_service, session)
    tool_registry = build_tool_registry(session, audit_service)
    tool_context = ToolContext(
        trace_id='bootstrap',
        session_id='bootstrap',
        user=User(id=0, username='system', display_name='system', role='admin'),
        metric_repo=metric_repo,
        ticket_repo=ticket_repo,
        release_repo=release_repo,
        verifier=verifier,
        anomaly_service=anomaly_service,
    )
    return AgentService(
        settings=settings,
        session_repo=session_repo,
        audit_service=audit_service,
        memory_service=memory_service,
        tool_registry=tool_registry,
        approval_service=approval_service,
        report_service=report_service,
        tool_context=tool_context,
    )


async def get_approval_service(session: AsyncSession = Depends(get_db_session)) -> ApprovalService:
    ticket_repo = TicketRepository(session)
    approval_repo = ApprovalRepository(session)
    audit_repo = AuditRepository(session)
    audit_service = AuditService(audit_repo)
    verifier = VerifierService(ticket_repo)
    return ApprovalService(approval_repo, ticket_repo, verifier, audit_service, session)


async def get_audit_service(session: AsyncSession = Depends(get_db_session)) -> AuditService:
    return AuditService(AuditRepository(session))
