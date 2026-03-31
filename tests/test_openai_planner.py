from types import SimpleNamespace

import pytest

from app.agents.planner_agent import OpenAIPlannerAgent
from app.schemas.tool import ToolSchema


class FakeLLMService:
    async def responses_create(self, **kwargs):
        return {
            'output': [
                {
                    'type': 'function_call',
                    'name': 'propose_assign_ticket',
                    'arguments': '{"ticket_no":"T202603280012","assignee_name":"??","reason":"???????"}',
                }
            ]
        }


@pytest.mark.asyncio
async def test_openai_planner_parses_function_calls():
    planner = OpenAIPlannerAgent(
        settings=SimpleNamespace(agent_runtime_mode='openai', has_real_openai_api_key=True),
        llm_service=FakeLLMService(),
        tool_schemas=[
            ToolSchema(
                name='propose_assign_ticket',
                description='??????????????',
                parameters={
                    'type': 'object',
                    'properties': {
                        'ticket_no': {'type': 'string'},
                        'assignee_name': {'type': 'string'},
                        'reason': {'type': 'string'},
                    },
                    'required': ['ticket_no', 'assignee_name', 'reason'],
                },
            )
        ],
    )

    planned_calls = await planner.plan('? T202603280012 ?????', {'messages': [{'role': 'user', 'text': '? T202603280012 ?????'}]})

    assert len(planned_calls) == 1
    assert planned_calls[0].tool_name == 'propose_assign_ticket'
    assert planned_calls[0].arguments['ticket_no'] == 'T202603280012'
