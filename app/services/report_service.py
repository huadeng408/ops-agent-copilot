from datetime import datetime

from app.repositories.metric_repo import MetricRepository
from app.repositories.release_repo import ReleaseRepository
from app.repositories.ticket_repo import TicketRepository


class ReportService:
    def __init__(self, metric_repo: MetricRepository, ticket_repo: TicketRepository, release_repo: ReleaseRepository) -> None:
        self.metric_repo = metric_repo
        self.ticket_repo = ticket_repo
        self.release_repo = release_repo

    async def generate_daily_report(self) -> dict:
        today = datetime.now().date()
        refund_snapshot = await self.metric_repo.get_refund_snapshot(today)
        high_priority_tickets = await self.ticket_repo.get_high_priority_open_tickets(limit=8)
        recent_releases = await self.release_repo.get_recent_releases(limit=5)
        lines = ['今日运营日报', '', '1. 退款率异常概览']
        if refund_snapshot:
            for item in refund_snapshot[:5]:
                lines.append(f"- {item['region']}-{item['category']} 退款率 {item['refund_rate']:.2%}，订单量 {item['orders_cnt']}")
        else:
            lines.append('- 今日暂无退款指标数据')
        lines.extend(['', '2. 高优先级工单'])
        if high_priority_tickets:
            for item in high_priority_tickets[:5]:
                lines.append(f"- {item['ticket_no']} | {item['priority']} | {item['region']} | {item['root_cause'] or '待定位'}")
        else:
            lines.append('- 当前没有高优先级未关闭工单')
        lines.extend(['', '3. 最近发布'])
        if recent_releases:
            for item in recent_releases[:3]:
                lines.append(f"- {item['service_name']} {item['release_version']} @ {item['release_time']} | {item['change_summary']}")
        else:
            lines.append('- 最近暂无发布记录')
        return {'report_type': 'daily', 'content': '\n'.join(lines)}
