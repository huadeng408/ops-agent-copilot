from time import perf_counter
from uuid import uuid4

from app.agents.assistant_factory import create_planner
from app.agents.router import MessageRouter
from app.core.config import Settings
from app.core.observability import get_tracer, metrics
from app.db.models import User
from app.repositories.session_repo import SessionRepository
from app.schemas.chat import ApprovalBrief, ChatResponse, ToolCallSummary
from app.schemas.tool import ToolSchema
from app.services.approval_service import ApprovalService
from app.services.audit_service import AuditService
from app.services.llm_service import LLMService
from app.services.memory_service import MemoryService
from app.services.report_service import ReportService
from app.services.tool_registry import ToolRegistry
from app.tools.base import ToolContext


REPORT_TOOL_SCHEMA = ToolSchema(
    name='generate_report',
    description='生成运营日报或周报',
    parameters={
        'type': 'object',
        'properties': {
            'report_type': {'type': 'string', 'enum': ['daily']},
        },
        'required': ['report_type'],
    },
)


class AgentService:
    def __init__(
        self,
        settings: Settings,
        session_repo: SessionRepository,
        audit_service: AuditService,
        memory_service: MemoryService,
        tool_registry: ToolRegistry,
        approval_service: ApprovalService,
        report_service: ReportService,
        tool_context: ToolContext,
    ) -> None:
        self.settings = settings
        self.session_repo = session_repo
        self.audit_service = audit_service
        self.memory_service = memory_service
        self.tool_registry = tool_registry
        self.approval_service = approval_service
        self.report_service = report_service
        self.tool_context = tool_context
        self.router = MessageRouter()
        llm_service = LLMService(settings)
        planner_tools = [*self.tool_registry.list_schemas(), REPORT_TOOL_SCHEMA]
        self.planner = create_planner(settings, llm_service=llm_service, tool_schemas=planner_tools)

    async def handle_chat(self, session_id: str, user: User, message: str) -> ChatResponse:
        started = perf_counter()
        trace_id = f'tr_{uuid4().hex}'
        tracer = get_tracer(__name__)

        with tracer.start_as_current_span('agent.handle_chat') as span:
            span.set_attribute('chat.session_id', session_id)
            span.set_attribute('chat.trace_id', trace_id)
            span.set_attribute('chat.user_id', user.id)

            await self.session_repo.get_or_create(session_id, user.id)
            await self.session_repo.add_message(session_id, 'user', {'text': message}, trace_id)
            await self.audit_service.log_event(
                trace_id=trace_id,
                session_id=session_id,
                user_id=user.id,
                event_type='chat_received',
                event_data={'message': message},
            )

            memory = await self.memory_service.build_context(session_id)
            heuristic_calls = self.router.route(message)
            if heuristic_calls and not (len(heuristic_calls) == 1 and heuristic_calls[0].tool_name == 'generate_report'):
                planned_calls = heuristic_calls
                planning_source = 'heuristic_router'
            else:
                planned_calls = await self.planner.plan(message, memory)
                planning_source = 'planner'
            span.set_attribute('chat.planning_source', planning_source)

            tool_call_summaries: list[ToolCallSummary] = []
            if len(planned_calls) == 1 and planned_calls[0].tool_name == 'generate_report':
                report = await self.report_service.generate_daily_report()
                answer = report['content']
                await self.session_repo.add_message(session_id, 'assistant', {'text': answer}, trace_id)
                await self.memory_service.maybe_update_summary(session_id)
                await self.audit_service.log_event(
                    trace_id=trace_id,
                    session_id=session_id,
                    user_id=user.id,
                    event_type='response_returned',
                    event_data={'status': 'completed'},
                )
                latency_ms = int((perf_counter() - started) * 1000)
                metrics.record_chat(status='completed', latency_ms=latency_ms)
                span.set_attribute('chat.status', 'completed')
                span.set_attribute('chat.latency_ms', latency_ms)
                return ChatResponse(trace_id=trace_id, session_id=session_id, status='completed', answer=answer)

            answers: list[str] = []
            approval_brief: ApprovalBrief | None = None
            status = 'completed'

            for planned_call in planned_calls:
                result, record = await self.tool_registry.invoke(
                    planned_call.tool_name,
                    self.tool_context_with(trace_id, session_id, user),
                    planned_call.arguments,
                )
                tool_call_summaries.append(
                    ToolCallSummary(
                        tool_name=record.tool_name,
                        tool_type=record.tool_type,
                        success=record.success,
                        latency_ms=record.latency_ms,
                    )
                )
                if result.requires_approval:
                    approval = await self.approval_service.create_proposal(
                        session_id=session_id,
                        trace_id=trace_id,
                        requested_by=user,
                        action_type=result.data['action_type'],
                        target_type=result.data['target_type'],
                        target_id=result.data['target_id'],
                        payload=result.data['payload'],
                        reason=result.data['reason'],
                    )
                    approval_brief = ApprovalBrief(
                        approval_no=approval.approval_no,
                        action_type=approval.action_type,
                        target_id=approval.target_id,
                        payload=approval.payload,
                    )
                    answers.append('我已生成操作建议，需审批后执行。')
                    status = 'approval_required'
                    break
                answers.append(self._render_tool_answer(planned_call.tool_name, result.data))

            final_answer = '\n\n'.join(chunk for chunk in answers if chunk).strip() or '已处理请求。'
            await self.session_repo.add_message(session_id, 'assistant', {'text': final_answer}, trace_id)
            await self.memory_service.maybe_update_summary(session_id)
            await self.audit_service.log_event(
                trace_id=trace_id,
                session_id=session_id,
                user_id=user.id,
                event_type='response_returned',
                event_data={'status': status},
            )

            latency_ms = int((perf_counter() - started) * 1000)
            metrics.record_chat(status=status, latency_ms=latency_ms)
            span.set_attribute('chat.status', status)
            span.set_attribute('chat.latency_ms', latency_ms)

            return ChatResponse(
                trace_id=trace_id,
                session_id=session_id,
                status=status,
                answer=final_answer,
                tool_calls=tool_call_summaries,
                approval=approval_brief,
            )

    def tool_context_with(self, trace_id: str, session_id: str, user: User) -> ToolContext:
        return ToolContext(
            trace_id=trace_id,
            session_id=session_id,
            user=user,
            metric_repo=self.tool_context.metric_repo,
            ticket_repo=self.tool_context.ticket_repo,
            release_repo=self.tool_context.release_repo,
            verifier=self.tool_context.verifier,
            anomaly_service=self.tool_context.anomaly_service,
        )

    def _render_tool_answer(self, tool_name: str, data: dict) -> str:
        if tool_name == 'query_refund_metrics':
            rows = data.get('rows', [])
            if not rows:
                return '没有查到符合条件的退款率数据。'
            top = sorted(rows, key=lambda item: item['refund_rate'], reverse=True)[:5]
            lines = ['退款率查询结果：']
            for row in top:
                lines.append(
                    f"- {row['dt']} {row['region']}-{row['category']} 退款率 {row['refund_rate']:.2%}，退款单量 {row['refund_orders_cnt']}"
                )
            return '\n'.join(lines)

        if tool_name == 'find_refund_anomalies':
            rows = data.get('rows', [])
            if not rows:
                return '未发现明显退款率异常类目。'
            lines = ['退款率异常类目：']
            for row in rows:
                lines.append(
                    f"- {row['category']} 平均退款率 {row['avg_refund_rate']:.2%}，退款单量 {row['refund_orders_cnt']}"
                )
            return '\n'.join(lines)

        if tool_name == 'analyze_operational_anomaly':
            spikes = data.get('refund_spikes', [])
            root_causes = data.get('sla_by_root_cause', [])
            releases = data.get('nearby_releases', [])
            correlation = data.get('correlation', {})
            lines = [f"异常归因分析（日期：{data['target_date']}，区域：{data.get('region') or '全部'}）", '', '1. 退款异常']
            if spikes:
                for item in spikes[:3]:
                    lines.append(
                        f"- {item['region']}-{item['category']} 当日退款率 {item['target_refund_rate']:.2%}，较近 7 天基线提升 {item['delta_refund_rate']:.2%}"
                    )
            else:
                lines.append('- 未识别到明显退款率突增类目')

            lines.extend(['', '2. 超 SLA 工单'])
            if root_causes:
                for item in root_causes[:4]:
                    lines.append(f"- {item['group_key']}：{item['ticket_count']} 单")
            else:
                lines.append('- 未查询到超 SLA 工单')

            lines.extend(['', '3. 窗口内发布记录'])
            if releases:
                for item in releases[:3]:
                    lines.append(
                        f"- {item['release_time']} | {item['service_name']} {item['release_version']} | {item['change_summary']}"
                    )
            else:
                lines.append('- 异常窗口附近没有发布记录')

            lines.extend(['', '4. 半自动归因结论'])
            suspected_causes = correlation.get('suspected_causes', [])
            if suspected_causes:
                for item in suspected_causes:
                    evidence = '；'.join(item.get('evidence', []))
                    lines.append(f"- {item['cause']}（置信度：{item['confidence']}，证据：{evidence}）")
            else:
                lines.append('- 当前证据不足，建议进一步缩小时间窗口或补充更多上下文')
            return '\n'.join(lines)

        if tool_name == 'list_sla_breached_tickets':
            rows = data.get('rows', [])
            if not rows:
                return '当前没有查到超 SLA 工单。'
            if 'group_key' in rows[0]:
                lines = ['超 SLA 工单分类结果：']
                for row in rows:
                    lines.append(f"- {row['group_key']}：{row['ticket_count']} 单")
                return '\n'.join(lines)
            lines = ['超 SLA 工单列表：']
            for row in rows[:10]:
                lines.append(f"- {row['ticket_no']} | {row['priority']} | {row['region']} | {row['root_cause'] or '待定位'}")
            return '\n'.join(lines)

        if tool_name == 'get_ticket_detail':
            return (
                f"工单 {data['ticket_no']} 详情：\n"
                f"- 标题：{data['title']}\n"
                f"- 区域：{data['region']} | 类目：{data['category']}\n"
                f"- 状态：{data['status']} | 优先级：{data['priority']}\n"
                f"- 当前处理人：{data['assignee_name'] or '未分派'}\n"
                f"- 根因：{data['root_cause'] or '待定位'}\n"
                f"- 描述：{data['description']}"
            )

        if tool_name == 'get_ticket_comments':
            comments = data.get('comments', [])
            actions = data.get('actions', [])
            lines = ['最近操作记录：']
            for row in actions[:5]:
                lines.append(f"- {row['created_at']} | {row['action_type']} | {row['new_value']}")
            if comments:
                lines.append('最近备注：')
                for row in comments[:5]:
                    lines.append(f"- {row['created_at']} | {row['created_by']}：{row['comment_text']}")
            return '\n'.join(lines)

        if tool_name == 'get_recent_releases':
            rows = data.get('rows', [])
            if not rows:
                return '最近没有发布记录。'
            lines = ['最近发布记录：']
            for row in rows[:5]:
                lines.append(f"- {row['release_time']} | {row['service_name']} {row['release_version']} | {row['change_summary']}")
            return '\n'.join(lines)

        if tool_name == 'run_readonly_sql':
            return f"SQL 查询完成，共返回 {len(data.get('rows', []))} 行。"

        return '工具执行完成。'
