from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.models import AuditLog, ToolCallLog


class AuditRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create_audit_log(self, log: AuditLog) -> AuditLog:
        self.session.add(log)
        await self.session.flush()
        return log

    async def create_tool_call_log(self, log: ToolCallLog) -> ToolCallLog:
        self.session.add(log)
        await self.session.flush()
        return log

    async def list_by_trace_id(self, trace_id: str, *, event_type: str | None = None) -> list[AuditLog]:
        stmt = select(AuditLog).where(AuditLog.trace_id == trace_id)
        if event_type:
            stmt = stmt.where(AuditLog.event_type == event_type)
        stmt = stmt.order_by(AuditLog.created_at.asc(), AuditLog.id.asc())
        result = await self.session.execute(stmt)
        return result.scalars().all()

    async def list_recent(self, *, limit: int = 50, event_type: str | None = None) -> list[AuditLog]:
        stmt = select(AuditLog)
        if event_type:
            stmt = stmt.where(AuditLog.event_type == event_type)
        stmt = stmt.order_by(AuditLog.created_at.desc(), AuditLog.id.desc()).limit(limit)
        result = await self.session.execute(stmt)
        return result.scalars().all()

    async def list_tool_calls_by_trace_id(self, trace_id: str) -> list[ToolCallLog]:
        stmt = (
            select(ToolCallLog)
            .where(ToolCallLog.trace_id == trace_id)
            .order_by(ToolCallLog.created_at.asc(), ToolCallLog.id.asc())
        )
        result = await self.session.execute(stmt)
        return result.scalars().all()

    async def list_event_types(self) -> list[str]:
        stmt = select(AuditLog.event_type).distinct().order_by(AuditLog.event_type.asc())
        result = await self.session.execute(stmt)
        return [row[0] for row in result.all()]
