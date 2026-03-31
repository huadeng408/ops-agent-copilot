from time import perf_counter

from app.core.observability import get_tracer, metrics
from app.schemas.tool import ToolExecutionRecord, ToolSchema
from app.services.audit_service import AuditService
from app.tools.base import BaseTool, ToolContext, ToolResult


class ToolRegistry:
    def __init__(self, audit_service: AuditService) -> None:
        self.audit_service = audit_service
        self._tools: dict[str, BaseTool] = {}

    def register(self, tool: BaseTool) -> None:
        self._tools[tool.schema.name] = tool

    def get(self, name: str) -> BaseTool:
        return self._tools[name]

    def list_schemas(self) -> list[ToolSchema]:
        return [tool.schema for tool in self._tools.values()]

    async def invoke(self, name: str, context: ToolContext, arguments: dict) -> tuple[ToolResult, ToolExecutionRecord]:
        tool = self.get(name)
        started = perf_counter()
        tracer = get_tracer(__name__)
        with tracer.start_as_current_span(f'tool:{name}') as span:
            span.set_attribute('tool.name', name)
            span.set_attribute('tool.type', tool.tool_type)
            span.set_attribute('tool.trace_id', context.trace_id)
            try:
                result = await tool.execute(context, arguments)
                latency_ms = int((perf_counter() - started) * 1000)
                metrics.record_tool_call(tool_name=name, success=True, latency_ms=latency_ms)
                span.set_attribute('tool.success', True)
                span.set_attribute('tool.latency_ms', latency_ms)
                await self.audit_service.log_tool_call(
                    trace_id=context.trace_id,
                    session_id=context.session_id,
                    tool_name=name,
                    tool_type=tool.tool_type,
                    input_payload=arguments,
                    success=True,
                    latency_ms=latency_ms,
                    output_payload=result.data,
                )
                await self.audit_service.log_event(
                    trace_id=context.trace_id,
                    session_id=context.session_id,
                    user_id=context.user.id,
                    event_type='tool_called',
                    event_data={'tool_name': name, 'tool_type': tool.tool_type},
                )
                return result, ToolExecutionRecord(
                    tool_name=name,
                    tool_type=tool.tool_type,
                    success=True,
                    latency_ms=latency_ms,
                    output=result.data,
                )
            except Exception as exc:
                latency_ms = int((perf_counter() - started) * 1000)
                metrics.record_tool_call(tool_name=name, success=False, latency_ms=latency_ms)
                span.set_attribute('tool.success', False)
                span.set_attribute('tool.latency_ms', latency_ms)
                span.record_exception(exc)
                await self.audit_service.log_tool_call(
                    trace_id=context.trace_id,
                    session_id=context.session_id,
                    tool_name=name,
                    tool_type=tool.tool_type,
                    input_payload=arguments,
                    success=False,
                    latency_ms=latency_ms,
                    error_message=str(exc),
                )
                await self.audit_service.log_event(
                    trace_id=context.trace_id,
                    session_id=context.session_id,
                    user_id=context.user.id,
                    event_type='tool_failed',
                    event_data={'tool_name': name, 'error_message': str(exc)},
                )
                raise
