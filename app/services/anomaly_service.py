from datetime import date, datetime, time, timedelta

from app.repositories.metric_repo import MetricRepository
from app.repositories.release_repo import ReleaseRepository
from app.repositories.ticket_repo import TicketRepository


class AnomalyService:
    def __init__(
        self,
        metric_repo: MetricRepository,
        ticket_repo: TicketRepository,
        release_repo: ReleaseRepository,
    ) -> None:
        self.metric_repo = metric_repo
        self.ticket_repo = ticket_repo
        self.release_repo = release_repo

    async def analyze_operational_anomaly(self, *, target_date: date, region: str | None = None) -> dict:
        refund_spikes = await self.metric_repo.find_refund_spike_candidates(target_date=target_date, region=region, limit=5)
        sla_by_root_cause = await self.ticket_repo.list_sla_breached_tickets(
            target_date=target_date,
            region=region,
            group_by='root_cause',
        )
        sla_by_category = await self.ticket_repo.list_sla_breached_tickets(
            target_date=target_date,
            region=region,
            group_by='category',
        )
        affected_categories = [item['category'] for item in refund_spikes[:3]]
        breach_samples = await self.ticket_repo.list_sla_breach_samples(
            target_date=target_date,
            region=region,
            categories=affected_categories or None,
            limit=8,
        )
        releases = await self.release_repo.get_releases_between(
            start_time=datetime.combine(target_date - timedelta(days=1), time.min),
            end_time=datetime.combine(target_date + timedelta(days=1), time.max),
            limit=6,
        )
        correlation = self._build_correlation_summary(
            refund_spikes=refund_spikes,
            sla_by_root_cause=sla_by_root_cause,
            sla_by_category=sla_by_category,
            releases=releases,
        )
        return {
            'target_date': target_date.isoformat(),
            'region': region,
            'refund_spikes': refund_spikes,
            'sla_by_root_cause': sla_by_root_cause,
            'sla_by_category': sla_by_category,
            'breach_samples': breach_samples,
            'nearby_releases': releases,
            'correlation': correlation,
        }

    def _build_correlation_summary(
        self,
        *,
        refund_spikes: list[dict],
        sla_by_root_cause: list[dict],
        sla_by_category: list[dict],
        releases: list[dict],
    ) -> dict:
        top_spike = refund_spikes[0] if refund_spikes else None
        top_root_cause = sla_by_root_cause[0] if sla_by_root_cause else None
        top_category_counts = {item['group_key']: item['ticket_count'] for item in sla_by_category}
        matched_category = None
        if top_spike:
            matched_category = {
                'category': top_spike['category'],
                'ticket_count': top_category_counts.get(top_spike['category'], 0),
            }

        suspected_causes: list[dict] = []
        release_related = bool(releases) and top_root_cause and top_root_cause['group_key'] == '系统发布故障'
        if release_related:
            suspected_causes.append(
                {
                    'cause': '最近发布可能触发运营异常',
                    'confidence': 'high',
                    'evidence': [
                        '超 SLA 工单主因集中在系统发布故障',
                        '异常窗口内存在最近发布记录',
                        '退款异常与工单异常在时间窗口内重叠',
                    ],
                }
            )
        elif top_root_cause:
            suspected_causes.append(
                {
                    'cause': f"工单主因集中在{top_root_cause['group_key']}",
                    'confidence': 'medium',
                    'evidence': ['超 SLA 工单原因分布明显集中'],
                }
            )

        if matched_category and matched_category['ticket_count'] > 0:
            suspected_causes.append(
                {
                    'cause': f"{matched_category['category']}类目同时出现退款率抬升和超 SLA 工单堆积",
                    'confidence': 'medium',
                    'evidence': [
                        f"{matched_category['category']}退款率高于近 7 天基线",
                        f"{matched_category['category']}超 SLA 工单数量为 {matched_category['ticket_count']}",
                    ],
                }
            )

        return {
            'top_refund_spike': top_spike,
            'top_sla_root_cause': top_root_cause,
            'matched_category': matched_category,
            'suspected_causes': suspected_causes,
        }
