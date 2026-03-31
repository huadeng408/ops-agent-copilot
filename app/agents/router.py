import re
from dataclasses import dataclass
from datetime import datetime, timedelta


REGIONS = ['北京', '上海', '广州']


@dataclass(slots=True)
class PlannedToolCall:
    tool_name: str
    arguments: dict


class MessageRouter:
    def route(self, message: str) -> list[PlannedToolCall]:
        if self._should_analyze_operational_anomaly(message):
            return [self._parse_operational_anomaly(message)]
        if '分派给' in message:
            return [self._parse_assign(message)]
        if '备注' in message and ('补充' in message or '添加' in message):
            return [self._parse_add_comment(message)]
        if '升级' in message and re.search(r'\bP[123]\b', message):
            return [self._parse_escalate(message)]
        if '日报' in message or '周报' in message:
            return [PlannedToolCall(tool_name='generate_report', arguments={'report_type': 'daily'})]
        if '超' in message and 'SLA' in message:
            return [self._parse_sla_query(message)]
        ticket_no = self._extract_ticket_no(message)
        if ticket_no and ('详情' in message or '操作记录' in message):
            calls = [PlannedToolCall(tool_name='get_ticket_detail', arguments={'ticket_no': ticket_no})]
            if '操作记录' in message or '备注' in message:
                calls.append(PlannedToolCall(tool_name='get_ticket_comments', arguments={'ticket_no': ticket_no}))
            return calls
        if '发布' in message:
            return [PlannedToolCall(tool_name='get_recent_releases', arguments={})]
        if '退款率' in message and '异常' in message:
            return [self._parse_refund_anomaly(message)]
        if '退款率' in message:
            return [self._parse_refund_metric(message)]
        return [PlannedToolCall(tool_name='generate_report', arguments={'report_type': 'daily'})]

    def _parse_refund_anomaly(self, message: str) -> PlannedToolCall:
        start_date, end_date = self._extract_date_range(message)
        return PlannedToolCall(
            tool_name='find_refund_anomalies',
            arguments={
                'start_date': start_date,
                'end_date': end_date,
                'region': self._extract_region(message),
                'top_k': self._extract_top_k(message, default=5),
            },
        )

    def _parse_refund_metric(self, message: str) -> PlannedToolCall:
        start_date, end_date = self._extract_date_range(message)
        return PlannedToolCall(
            tool_name='query_refund_metrics',
            arguments={
                'start_date': start_date,
                'end_date': end_date,
                'region': self._extract_region(message),
                'category': self._extract_category(message),
            },
        )

    def _parse_operational_anomaly(self, message: str) -> PlannedToolCall:
        return PlannedToolCall(
            tool_name='analyze_operational_anomaly',
            arguments={
                'date': self._extract_single_date(message),
                'region': self._extract_region(message),
            },
        )

    def _parse_sla_query(self, message: str) -> PlannedToolCall:
        target_date = self._extract_single_date(message)
        group_by = 'root_cause' if '原因' in message else None
        if '优先级' in message:
            group_by = 'priority'
        if group_by is None and ('类目' in message or '分类' in message):
            group_by = 'category'
        return PlannedToolCall(
            tool_name='list_sla_breached_tickets',
            arguments={'date': target_date, 'region': self._extract_region(message), 'group_by': group_by},
        )

    def _parse_assign(self, message: str) -> PlannedToolCall:
        ticket_no = self._extract_ticket_no(message)
        assignee_match = re.search(r'分派给([\u4e00-\u9fa5A-Za-z0-9_]+)', message)
        assignee_name = assignee_match.group(1) if assignee_match else '待确认'
        return PlannedToolCall(
            tool_name='propose_assign_ticket',
            arguments={
                'ticket_no': ticket_no,
                'assignee_name': assignee_name,
                'reason': f'根据用户指令将 {ticket_no} 分派给 {assignee_name}',
            },
        )

    def _parse_add_comment(self, message: str) -> PlannedToolCall:
        ticket_no = self._extract_ticket_no(message)
        comment_match = re.search(r'备注[:：]\s*(.+)$', message)
        comment_text = comment_match.group(1) if comment_match else message.split('备注', 1)[-1].strip(' ：:')
        return PlannedToolCall(
            tool_name='propose_add_ticket_comment',
            arguments={
                'ticket_no': ticket_no,
                'comment_text': comment_text,
                'reason': f'根据用户指令为 {ticket_no} 增加备注',
            },
        )

    def _parse_escalate(self, message: str) -> PlannedToolCall:
        ticket_no = self._extract_ticket_no(message)
        priority = re.search(r'\b(P[123])\b', message).group(1)
        return PlannedToolCall(
            tool_name='propose_escalate_ticket',
            arguments={
                'ticket_no': ticket_no,
                'new_priority': priority,
                'reason': f'根据用户指令将 {ticket_no} 升级为 {priority}',
            },
        )

    def _extract_ticket_no(self, message: str) -> str:
        match = re.search(r'T\d{6,}', message)
        return match.group(0) if match else ''

    def _extract_region(self, message: str) -> str | None:
        for region in REGIONS:
            if region in message:
                return region
        return None

    def _extract_category(self, message: str) -> str | None:
        for category in ['生鲜', '餐饮', '酒店', '到店综合']:
            if category in message:
                return category
        return None

    def _extract_top_k(self, message: str, default: int = 5) -> int:
        match = re.search(r'(\d+)\s*个', message)
        return int(match.group(1)) if match else default

    def _extract_date_range(self, message: str) -> tuple[str, str]:
        today = datetime.now().date()
        if '最近7天' in message or '最近 7 天' in message:
            return ((today - timedelta(days=6)).isoformat(), today.isoformat())
        if '上周' in message:
            weekday = today.weekday()
            end = today - timedelta(days=weekday + 1)
            start = end - timedelta(days=6)
            return (start.isoformat(), end.isoformat())
        if '昨天' in message:
            yesterday = today - timedelta(days=1)
            return (yesterday.isoformat(), yesterday.isoformat())
        return ((today - timedelta(days=6)).isoformat(), today.isoformat())

    def _extract_single_date(self, message: str) -> str:
        today = datetime.now().date()
        if '昨天' in message:
            return (today - timedelta(days=1)).isoformat()
        if '今天' in message:
            return today.isoformat()
        return today.isoformat()

    def _should_analyze_operational_anomaly(self, message: str) -> bool:
        keywords = ['归因', '关联分析', '关联一下', '异常原因', '发布影响']
        if any(keyword in message for keyword in keywords):
            return True
        return '退款率' in message and 'SLA' in message
