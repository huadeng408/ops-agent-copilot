import asyncio
import http.client
import json
import ssl
from urllib.parse import urlparse

from app.core.config import Settings
from app.core.observability import get_tracer, metrics


class LLMService:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings

    async def chat_completion(self, messages: list[dict]) -> dict:
        payload = {'model': self.settings.openai_model, 'messages': messages}
        tracer = get_tracer(__name__)
        with tracer.start_as_current_span('llm.chat_completion') as span:
            span.set_attribute('llm.model', self.settings.openai_model)
            span.set_attribute('llm.endpoint', '/chat/completions')
            try:
                result = await asyncio.to_thread(self._post_json, '/chat/completions', payload)
                metrics.record_llm_request(endpoint='/chat/completions', success=True)
                return result
            except Exception:
                metrics.record_llm_request(endpoint='/chat/completions', success=False)
                raise

    async def responses_create(
        self,
        *,
        input_items: list[dict],
        tools: list[dict] | None = None,
        instructions: str | None = None,
        parallel_tool_calls: bool | None = None,
    ) -> dict:
        payload: dict = {'model': self.settings.openai_model, 'input': input_items}
        if instructions:
            payload['instructions'] = instructions
        if tools:
            payload['tools'] = tools
        if parallel_tool_calls is not None:
            payload['parallel_tool_calls'] = parallel_tool_calls

        tracer = get_tracer(__name__)
        with tracer.start_as_current_span('llm.responses_create') as span:
            span.set_attribute('llm.model', self.settings.openai_model)
            span.set_attribute('llm.endpoint', '/responses')
            span.set_attribute('llm.tool_count', len(tools or []))
            try:
                result = await asyncio.to_thread(self._post_json, '/responses', payload)
                metrics.record_llm_request(endpoint='/responses', success=True)
                return result
            except Exception as exc:
                metrics.record_llm_request(endpoint='/responses', success=False)
                if self._should_fallback_to_chat_completions(exc):
                    fallback_result = await asyncio.to_thread(
                        self._post_json,
                        '/chat/completions',
                        self._build_chat_completions_payload(input_items=input_items, tools=tools, instructions=instructions, parallel_tool_calls=parallel_tool_calls),
                    )
                    metrics.record_llm_request(endpoint='/chat/completions', success=True)
                    return self._convert_chat_completion_response(fallback_result)
                raise

    def _post_json(self, endpoint: str, payload: dict) -> dict:
        parsed = urlparse(self.settings.openai_base_url.rstrip('/'))
        scheme = parsed.scheme.lower()
        host = parsed.hostname
        if not host:
            raise RuntimeError(f'Invalid OPENAI_BASE_URL: {self.settings.openai_base_url}')

        port = parsed.port or (443 if scheme == 'https' else 80)
        base_path = parsed.path.rstrip('/')
        path = f'{base_path}{endpoint}' if base_path else endpoint
        body = json.dumps(payload)
        headers = {
            'Authorization': f'Bearer {self.settings.openai_api_key}',
            'Content-Type': 'application/json',
            'Accept-Encoding': 'identity',
            'Connection': 'close',
        }

        connection: http.client.HTTPConnection
        if scheme == 'https':
            connection = http.client.HTTPSConnection(host, port, timeout=60, context=ssl.create_default_context())
        elif scheme == 'http':
            connection = http.client.HTTPConnection(host, port, timeout=60)
        else:
            raise RuntimeError(f'Unsupported OPENAI_BASE_URL scheme: {parsed.scheme}')

        try:
            connection.request('POST', path, body=body, headers=headers)
            response = connection.getresponse()
            try:
                raw = response.read()
            except http.client.IncompleteRead as exc:
                raw = exc.partial

            text = raw.decode('utf-8', errors='replace')
            if response.status >= 400:
                raise RuntimeError(f'OpenAI API error {response.status}: {text}')
            return json.loads(text)
        finally:
            connection.close()

    def _build_chat_completions_payload(
        self,
        *,
        input_items: list[dict],
        tools: list[dict] | None,
        instructions: str | None,
        parallel_tool_calls: bool | None,
    ) -> dict:
        messages: list[dict] = []
        if instructions:
            messages.append({'role': 'system', 'content': instructions})

        for item in input_items:
            role = str(item.get('role') or 'user')
            content = item.get('content')
            if content is None:
                content = item.get('text')
            messages.append({'role': role, 'content': content})

        payload: dict = {'model': self.settings.openai_model, 'messages': messages}
        if tools:
            payload['tools'] = [self._convert_tool_schema(tool) for tool in tools]
            payload['tool_choice'] = 'auto'
        if parallel_tool_calls is not None:
            payload['parallel_tool_calls'] = parallel_tool_calls
        return payload

    def _convert_tool_schema(self, tool: dict) -> dict:
        if tool.get('function'):
            return tool
        return {
            'type': 'function',
            'function': {
                'name': tool.get('name'),
                'description': tool.get('description'),
                'parameters': tool.get('parameters', {}),
            },
        }

    def _convert_chat_completion_response(self, response: dict) -> dict:
        output: list[dict] = []
        for choice in response.get('choices', []):
            message = choice.get('message') or {}
            for tool_call in message.get('tool_calls', []) or []:
                function = tool_call.get('function') or {}
                output.append(
                    {
                        'type': 'function_call',
                        'name': function.get('name'),
                        'arguments': function.get('arguments') or '{}',
                    }
                )
        return {'output': output, 'raw': response}

    def _should_fallback_to_chat_completions(self, exc: Exception) -> bool:
        message = str(exc).lower()
        return (
            '/responses' in message
            and (
                '404' in message
                or 'url.not_found' in message
                or 'not found' in message
                or 'not_found' in message
            )
        )
