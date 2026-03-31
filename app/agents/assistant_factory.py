from app.agents.planner_agent import HeuristicPlannerAgent, OpenAIPlannerAgent, OptionalQwenPlannerAgent
from app.core.config import Settings
from app.schemas.tool import ToolSchema
from app.services.llm_service import LLMService


def create_planner(
    settings: Settings,
    llm_service: LLMService | None = None,
    tool_schemas: list[ToolSchema] | None = None,
):
    if settings.agent_runtime_mode == 'heuristic':
        return HeuristicPlannerAgent()

    if settings.agent_runtime_mode == 'openai':
        return OpenAIPlannerAgent(settings=settings, llm_service=llm_service, tool_schemas=tool_schemas or [])

    if settings.agent_runtime_mode == 'auto' and settings.has_real_openai_api_key:
        return OpenAIPlannerAgent(settings=settings, llm_service=llm_service, tool_schemas=tool_schemas or [])

    if settings.agent_runtime_mode in {'auto', 'qwen_agent'}:
        planner = OptionalQwenPlannerAgent(settings=settings, llm_service=llm_service)
        if planner.is_available:
            return planner

    return HeuristicPlannerAgent()
