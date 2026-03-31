from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.exceptions import NotFoundError
from app.db.models import Approval


class ApprovalRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def create(self, approval: Approval) -> Approval:
        self.session.add(approval)
        await self.session.flush()
        return approval

    async def get_by_no(self, approval_no: str) -> Approval:
        result = await self.session.execute(select(Approval).where(Approval.approval_no == approval_no))
        approval = result.scalar_one_or_none()
        if approval is None:
            raise NotFoundError(f'审批单不存在: {approval_no}')
        return approval

    async def get_by_idempotency_key(self, idempotency_key: str) -> Approval | None:
        result = await self.session.execute(select(Approval).where(Approval.idempotency_key == idempotency_key))
        return result.scalar_one_or_none()

    async def list_recent(self, *, status: str | None = None, limit: int = 20) -> list[Approval]:
        stmt = select(Approval)
        if status:
            stmt = stmt.where(Approval.status == status)
        stmt = stmt.order_by(Approval.created_at.desc(), Approval.id.desc()).limit(limit)
        result = await self.session.execute(stmt)
        return result.scalars().all()
