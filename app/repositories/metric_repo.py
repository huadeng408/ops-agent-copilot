from datetime import date, timedelta

from sqlalchemy import desc, func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.models import MetricRefundDaily


class MetricRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def query_refund_metrics(
        self,
        start_date: date,
        end_date: date,
        region: str | None = None,
        category: str | None = None,
    ) -> list[dict]:
        stmt = select(MetricRefundDaily).where(
            MetricRefundDaily.dt >= start_date,
            MetricRefundDaily.dt <= end_date,
        )
        if region:
            stmt = stmt.where(MetricRefundDaily.region == region)
        if category:
            stmt = stmt.where(MetricRefundDaily.category == category)
        stmt = stmt.order_by(MetricRefundDaily.dt.asc(), MetricRefundDaily.refund_rate.desc())
        result = await self.session.execute(stmt)
        rows = result.scalars().all()
        return [
            {
                'dt': row.dt.isoformat(),
                'region': row.region,
                'category': row.category,
                'orders_cnt': row.orders_cnt,
                'refund_orders_cnt': row.refund_orders_cnt,
                'refund_rate': float(row.refund_rate),
                'gmv': float(row.gmv),
            }
            for row in rows
        ]

    async def find_refund_anomalies(
        self,
        start_date: date,
        end_date: date,
        region: str | None = None,
        top_k: int = 5,
    ) -> list[dict]:
        stmt = (
            select(
                MetricRefundDaily.category.label('category'),
                func.avg(MetricRefundDaily.refund_rate).label('avg_refund_rate'),
                func.sum(MetricRefundDaily.refund_orders_cnt).label('refund_orders_cnt'),
                func.sum(MetricRefundDaily.orders_cnt).label('orders_cnt'),
            )
            .where(MetricRefundDaily.dt >= start_date, MetricRefundDaily.dt <= end_date)
            .group_by(MetricRefundDaily.category)
            .order_by(desc('avg_refund_rate'))
            .limit(top_k)
        )
        if region:
            stmt = stmt.where(MetricRefundDaily.region == region)
        result = await self.session.execute(stmt)
        return [
            {
                'category': row.category,
                'avg_refund_rate': float(row.avg_refund_rate or 0),
                'refund_orders_cnt': int(row.refund_orders_cnt or 0),
                'orders_cnt': int(row.orders_cnt or 0),
            }
            for row in result.all()
        ]

    async def get_refund_snapshot(self, target_date: date, region: str | None = None) -> list[dict]:
        stmt = select(MetricRefundDaily).where(MetricRefundDaily.dt == target_date)
        if region:
            stmt = stmt.where(MetricRefundDaily.region == region)
        stmt = stmt.order_by(MetricRefundDaily.refund_rate.desc()).limit(10)
        result = await self.session.execute(stmt)
        rows = result.scalars().all()
        return [
            {
                'category': row.category,
                'region': row.region,
                'refund_rate': float(row.refund_rate),
                'orders_cnt': row.orders_cnt,
            }
            for row in rows
        ]

    async def find_refund_spike_candidates(
        self,
        target_date: date,
        region: str | None = None,
        *,
        lookback_days: int = 7,
        limit: int = 5,
    ) -> list[dict]:
        baseline_start = target_date - timedelta(days=lookback_days)
        baseline_end = target_date - timedelta(days=1)

        baseline_stmt = (
            select(
                MetricRefundDaily.region.label('region'),
                MetricRefundDaily.category.label('category'),
                func.avg(MetricRefundDaily.refund_rate).label('baseline_refund_rate'),
            )
            .where(MetricRefundDaily.dt >= baseline_start, MetricRefundDaily.dt <= baseline_end)
            .group_by(MetricRefundDaily.region, MetricRefundDaily.category)
        )
        if region:
            baseline_stmt = baseline_stmt.where(MetricRefundDaily.region == region)
        baseline_subquery = baseline_stmt.subquery()

        stmt = (
            select(
                MetricRefundDaily.region.label('region'),
                MetricRefundDaily.category.label('category'),
                MetricRefundDaily.refund_rate.label('target_refund_rate'),
                MetricRefundDaily.refund_orders_cnt.label('refund_orders_cnt'),
                MetricRefundDaily.orders_cnt.label('orders_cnt'),
                baseline_subquery.c.baseline_refund_rate.label('baseline_refund_rate'),
            )
            .join(
                baseline_subquery,
                (MetricRefundDaily.region == baseline_subquery.c.region)
                & (MetricRefundDaily.category == baseline_subquery.c.category),
            )
            .where(MetricRefundDaily.dt == target_date)
            .order_by(desc(MetricRefundDaily.refund_rate - baseline_subquery.c.baseline_refund_rate))
            .limit(limit)
        )
        if region:
            stmt = stmt.where(MetricRefundDaily.region == region)

        result = await self.session.execute(stmt)
        rows = []
        for row in result.all():
            target_refund_rate = float(row.target_refund_rate or 0)
            baseline_refund_rate = float(row.baseline_refund_rate or 0)
            rows.append(
                {
                    'region': row.region,
                    'category': row.category,
                    'target_refund_rate': target_refund_rate,
                    'baseline_refund_rate': baseline_refund_rate,
                    'delta_refund_rate': round(target_refund_rate - baseline_refund_rate, 4),
                    'refund_orders_cnt': int(row.refund_orders_cnt or 0),
                    'orders_cnt': int(row.orders_cnt or 0),
                }
            )
        return rows
