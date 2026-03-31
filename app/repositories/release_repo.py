from datetime import datetime

from sqlalchemy import desc, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.models import Release


class ReleaseRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def get_recent_releases(self, limit: int = 10) -> list[dict]:
        result = await self.session.execute(select(Release).order_by(desc(Release.release_time)).limit(limit))
        return [
            {
                'service_name': row.service_name,
                'release_version': row.release_version,
                'release_time': row.release_time.isoformat(),
                'operator_name': row.operator_name,
                'change_summary': row.change_summary,
            }
            for row in result.scalars().all()
        ]

    async def get_releases_between(self, start_time: datetime, end_time: datetime, limit: int = 10) -> list[dict]:
        stmt = (
            select(Release)
            .where(Release.release_time >= start_time, Release.release_time <= end_time)
            .order_by(desc(Release.release_time))
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'service_name': row.service_name,
                'release_version': row.release_version,
                'release_time': row.release_time.isoformat(),
                'operator_name': row.operator_name,
                'change_summary': row.change_summary,
            }
            for row in result.scalars().all()
        ]
