from datetime import date

from sqlalchemy import text

from app.schemas.tool import ToolSchema
from app.tools.base import BaseTool, ToolContext, ToolResult
from app.tools.sql_guard import SQLGuard


REGION_ENUM = ['北京', '上海', '广州']
CATEGORY_ENUM = ['生鲜', '餐饮', '酒店', '到店综合']


class QueryRefundMetricsTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='query_refund_metrics',
            description='查询退款率指标',
            parameters={
                'type': 'object',
                'properties': {
                    'start_date': {'type': 'string'},
                    'end_date': {'type': 'string'},
                    'region': {'type': 'string', 'enum': REGION_ENUM},
                    'category': {'type': 'string', 'enum': CATEGORY_ENUM},
                },
                'required': ['start_date', 'end_date', 'region', 'category'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        rows = await context.metric_repo.query_refund_metrics(
            start_date=date.fromisoformat(arguments['start_date']),
            end_date=date.fromisoformat(arguments['end_date']),
            region=arguments.get('region'),
            category=arguments.get('category'),
        )
        return ToolResult(data={'rows': rows}, message=f'查询到 {len(rows)} 条退款指标')


class FindRefundAnomaliesTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='find_refund_anomalies',
            description='找出退款率异常的类目或区域',
            parameters={
                'type': 'object',
                'properties': {
                    'start_date': {'type': 'string'},
                    'end_date': {'type': 'string'},
                    'region': {'type': 'string', 'enum': REGION_ENUM},
                    'top_k': {'type': 'integer', 'default': 5},
                },
                'required': ['start_date', 'end_date', 'region', 'top_k'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        rows = await context.metric_repo.find_refund_anomalies(
            start_date=date.fromisoformat(arguments['start_date']),
            end_date=date.fromisoformat(arguments['end_date']),
            region=arguments.get('region'),
            top_k=int(arguments.get('top_k', 5)),
        )
        return ToolResult(data={'rows': rows}, message=f'识别到 {len(rows)} 个高退款率类目')


class ListSlaBreachedTicketsTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='list_sla_breached_tickets',
            description='列出超 SLA 工单',
            parameters={
                'type': 'object',
                'properties': {
                    'region': {'type': 'string', 'enum': REGION_ENUM},
                    'date': {'type': 'string'},
                    'group_by': {'type': 'string', 'enum': ['root_cause', 'priority', 'category', 'assignee_name']},
                },
                'required': ['region', 'date', 'group_by'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        rows = await context.ticket_repo.list_sla_breached_tickets(
            target_date=date.fromisoformat(arguments['date']),
            region=arguments.get('region'),
            group_by=arguments.get('group_by'),
        )
        return ToolResult(data={'rows': rows}, message=f'查询到 {len(rows)} 条超 SLA 结果')


class GetTicketDetailTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='get_ticket_detail',
            description='查询工单详情',
            parameters={'type': 'object', 'properties': {'ticket_no': {'type': 'string'}}, 'required': ['ticket_no']},
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        detail = await context.ticket_repo.get_ticket_detail(arguments['ticket_no'])
        return ToolResult(data=detail, message='已查询工单详情')


class GetTicketCommentsTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='get_ticket_comments',
            description='查询工单备注和最近操作',
            parameters={'type': 'object', 'properties': {'ticket_no': {'type': 'string'}}, 'required': ['ticket_no']},
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        comments = await context.ticket_repo.get_ticket_comments(arguments['ticket_no'])
        actions = await context.ticket_repo.get_recent_ticket_actions(arguments['ticket_no'])
        return ToolResult(data={'comments': comments, 'actions': actions}, message='已查询工单备注与最近操作')


class GetRecentReleasesTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='get_recent_releases',
            description='查询最近发布记录',
            parameters={'type': 'object', 'properties': {}, 'required': []},
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        rows = await context.release_repo.get_recent_releases(limit=int(arguments.get('limit', 10)))
        return ToolResult(data={'rows': rows}, message=f'查询到 {len(rows)} 条发布记录')


class AnalyzeOperationalAnomalyTool(BaseTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='analyze_operational_anomaly',
            description='关联退款异常、超SLA工单与最近发布记录，输出半自动归因结论',
            parameters={
                'type': 'object',
                'properties': {
                    'date': {'type': 'string'},
                    'region': {'type': 'string', 'enum': REGION_ENUM},
                },
                'required': ['date'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        if context.anomaly_service is None:
            raise ValueError('anomaly_service is not configured')
        analysis = await context.anomaly_service.analyze_operational_anomaly(
            target_date=date.fromisoformat(arguments['date']),
            region=arguments.get('region'),
        )
        return ToolResult(data=analysis, message='已完成异常归因分析')


class RunReadonlySqlTool(BaseTool):
    def __init__(self, session) -> None:
        self.session = session
        self.sql_guard = SQLGuard()

    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='run_readonly_sql',
            description='执行受限只读 SQL，仅允许 SELECT 白名单视图',
            parameters={'type': 'object', 'properties': {'sql': {'type': 'string'}}, 'required': ['sql']},
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        verification = self.sql_guard.validate(arguments['sql'])
        if not verification['passed']:
            raise ValueError(verification['message'])
        result = await self.session.execute(text(arguments['sql']))
        rows = [dict(row) for row in result.mappings().all()]
        return ToolResult(data={'rows': rows}, message=f'SQL 返回 {len(rows)} 行结果')
