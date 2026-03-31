from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.models import AgentMessage, AgentSession


class SessionRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_or_create(self, session_id: str, user_id: int) -> AgentSession:
        result = await self.session.execute(select(AgentSession).where(AgentSession.session_id == session_id))
        item = result.scalar_one_or_none()
        if item:
            return item
        item = AgentSession(session_id=session_id, user_id=user_id, summary=None)
        self.session.add(item)
        await self.session.flush()
        return item

    async def add_message(self, session_id: str, role: str, content: dict, trace_id: str) -> AgentMessage:
        message = AgentMessage(session_id=session_id, role=role, content=content, trace_id=trace_id)
        self.session.add(message)
        await self.session.flush()
        return message

    async def list_recent_messages(self, session_id: str, limit: int = 8) -> list[AgentMessage]:
        stmt = (
            select(AgentMessage)
            .where(AgentMessage.session_id == session_id)
            .order_by(AgentMessage.created_at.desc(), AgentMessage.id.desc())
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return list(reversed(result.scalars().all()))

    async def update_summary(self, session_id: str, summary: str) -> None:
        result = await self.session.execute(select(AgentSession).where(AgentSession.session_id == session_id))
        item = result.scalar_one()
        item.summary = summary
        await self.session.flush()
