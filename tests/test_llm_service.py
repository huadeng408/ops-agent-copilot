from types import SimpleNamespace

import pytest

from app.services.llm_service import LLMService


@pytest.mark.asyncio
async def test_responses_create_falls_back_to_chat_completions_on_404():
    calls: list[tuple[str, dict]] = []

    def fake_post_json(endpoint: str, payload: dict) -> dict:
        calls.append((endpoint, payload))
        if endpoint == '/responses':
            raise RuntimeError(
                'OpenAI API error 404: {"code":5,"error":"url.not_found","message":"没找到对象","method":"POST","scode":"0x5","status":false,"ua":"","url":"/v1/responses"}'
            )
        if endpoint == '/chat/completions':
            return {
                'choices': [
                    {
                        'message': {
                            'tool_calls': [
                                {
                                    'function': {
                                        'name': 'query_refund_metrics',
                                        'arguments': '{"start_date":"2026-03-01","end_date":"2026-03-07"}',
                                    }
                                }
                            ]
                        }
                    }
                ]
            }
        raise AssertionError(f'unexpected endpoint: {endpoint}')

    service = LLMService(
        SimpleNamespace(
            openai_model='kimi-k2-0905-preview',
            openai_base_url='https://api.moonshot.cn/v1',
            openai_api_key='sk-test',
        )
    )
    service._post_json = fake_post_json  # type: ignore[method-assign]

    result = await service.responses_create(
        input_items=[{'role': 'user', 'content': 'summary'}],
        tools=[
            {
                'type': 'function',
                'name': 'query_refund_metrics',
                'description': 'query refund metrics',
                'parameters': {'type': 'object', 'properties': {}},
            }
        ],
        instructions='SYSTEM',
        parallel_tool_calls=True,
    )

    assert [endpoint for endpoint, _ in calls] == ['/responses', '/chat/completions']
    assert result['output'][0]['type'] == 'function_call'
    assert result['output'][0]['name'] == 'query_refund_metrics'
    assert result['output'][0]['arguments'] == '{"start_date":"2026-03-01","end_date":"2026-03-07"}'
