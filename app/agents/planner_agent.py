import json
import logging

from app.agents.prompts import SYSTEM_PROMPT
from app.agents.router import MessageRouter, PlannedToolCall
from app.core.config import Settings
from app.core.observability import get_tracer, metrics
from app.schemas.tool import ToolSchema
from app.services.llm_service import LLMService


logger = logging.getLogger(__name__)


class HeuristicPlannerAgent:
    def __init__(self) -> None:
        self.router = MessageRouter()

    async def plan(self, message: str, memory: dict | None = None) -> list[PlannedToolCall]:
        return self.router.route(message)


class OpenAIPlannerAgent:
    def __init__(self, settings: Settings, llm_service: LLMService | None = None, tool_schemas: list[ToolSchema] | None = None) -> None:
        self.settings = settings
        self.llm_service = llm_service or LLMService(settings)
        self.tool_schemas = tool_schemas or []
        self.router = MessageRouter()
        self.is_available = settings.has_real_openai_api_key

    async def plan(self, message: str, memory: dict | None = None) -> list[PlannedToolCall]:
        heuristic_calls = self.router.route(message)
        if heuristic_calls and not (len(heuristic_calls) == 1 and heuristic_calls[0].tool_name == 'generate_report'):
            return heuristic_calls

        if not self.is_available:
            metrics.record_llm_fallback(reason='llm_unavailable')
            raise RuntimeError('OPENAI_API_KEY is not configured for the OpenAI planner')

        tracer = get_tracer(__name__)
        with tracer.start_as_current_span('planner.plan') as span:
            span.set_attribute('planner.mode', 'openai')
            try:
                response = await self.llm_service.responses_create(
                    input_items=self._build_input(message, memory),
                    tools=self._build_tools(),
                    instructions=SYSTEM_PROMPT,
                    parallel_tool_calls=True,
                )
                planned_calls = self._parse_planned_calls(response)
                if planned_calls:
                    return self._merge_with_heuristic(message, planned_calls)

                metrics.record_llm_fallback(reason='empty_plan')
                return heuristic_calls or self.router.route(message)
            except Exception as exc:
                span.record_exception(exc)
                if self.settings.agent_runtime_mode == 'openai':
                    raise
                metrics.record_llm_fallback(reason='planner_exception')
                logger.exception('OpenAI planner failed, falling back to heuristic router')
                return heuristic_calls or self.router.route(message)

    def _build_input(self, message: str, memory: dict | None) -> list[dict]:
        input_items: list[dict] = []
        if memory:
            summary = str(memory.get('summary') or '').strip()
            if summary:
                input_items.append({'role': 'system', 'content': f'Conversation summary:\n{summary}'})
            for item in memory.get('messages', []):
                text = str(item.get('text') or '').strip()
                if not text:
                    continue
                role = item.get('role', 'user')
                input_items.append({'role': role, 'content': text})
        hints = self.router.route(message)
        if hints:
            input_items.append(
                {
                    'role': 'system',
                    'content': 'Heuristic tool hints (use as strong guidance, not final answer): '
                    + json.dumps([{'tool_name': item.tool_name, 'arguments': item.arguments} for item in hints], ensure_ascii=False),
                }
            )
        input_items.append({'role': 'user', 'content': message})
        return input_items

    def _build_tools(self) -> list[dict]:
        return [
            {
                'type': 'function',
                'name': schema.name,
                'description': schema.description,
                'parameters': self._normalize_parameters(schema.parameters),
            }
            for schema in self.tool_schemas
        ]

    def _normalize_parameters(self, parameters: dict) -> dict:
        normalized = dict(parameters)
        if normalized.get('type') == 'object':
            normalized.setdefault('properties', {})
            normalized.setdefault('required', [])
            normalized['additionalProperties'] = False
        return normalized

    def _parse_planned_calls(self, response: dict) -> list[PlannedToolCall]:
        planned_calls: list[PlannedToolCall] = []
        for item in response.get('output', []):
            if item.get('type') != 'function_call':
                continue
            try:
                arguments = json.loads(item.get('arguments') or '{}')
            except json.JSONDecodeError:
                arguments = {}
            planned_calls.append(PlannedToolCall(tool_name=item['name'], arguments=arguments))
        return planned_calls

    def _merge_with_heuristic(self, message: str, planned_calls: list[PlannedToolCall]) -> list[PlannedToolCall]:
        heuristic_calls = {call.tool_name: call for call in self.router.route(message)}
        merged_calls: list[PlannedToolCall] = []

        for planned_call in planned_calls:
            heuristic_call = heuristic_calls.get(planned_call.tool_name)
            if not heuristic_call:
                merged_calls.append(planned_call)
                continue

            merged_arguments = dict(planned_call.arguments)
            for key, value in heuristic_call.arguments.items():
                merged_arguments[key] = value
            merged_calls.append(PlannedToolCall(tool_name=planned_call.tool_name, arguments=merged_arguments))

        return merged_calls


class OptionalQwenPlannerAgent:
    def __init__(self, settings: Settings, llm_service: LLMService | None = None) -> None:
        self.settings = settings
        self.llm_service = llm_service
        self.router = MessageRouter()
        self.is_available = False
        try:
            from qwen_agent.agents import Assistant  # type: ignore

            self._assistant_cls = Assistant
            self.is_available = True
        except Exception:
            self._assistant_cls = None

    async def plan(self, message: str, memory: dict | None = None) -> list[PlannedToolCall]:
        return self.router.route(message)
