from __future__ import annotations

from typing import Any

import httpx

from .config import RuntimeConfig


class BackendServiceError(RuntimeError):
    def __init__(self, status_code: int, detail: str) -> None:
        super().__init__(detail)
        self.status_code = status_code
        self.detail = detail


class BackendClient:
    def __init__(self, config: RuntimeConfig) -> None:
        headers: dict[str, str] = {}
        if config.internal_api_key:
            headers["X-Internal-API-Key"] = config.internal_api_key
        self._client = httpx.Client(
            base_url=config.ops_agent_base_url.rstrip("/"),
            headers=headers,
            timeout=30.0,
            trust_env=False,
        )

    def _post_json(self, path: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        response = self._client.post(path, json=payload)
        if response.is_error:
            detail = response.text.strip()
            try:
                body = response.json()
                detail = str(body.get("detail") or detail)
            except Exception:
                pass
            raise BackendServiceError(response.status_code, detail or f"backend request failed: {path}")
        return response.json()

    def invoke_tool(
        self,
        *,
        trace_id: str,
        session_id: str,
        user_id: int,
        tool_name: str,
        arguments: dict[str, Any],
    ) -> dict[str, Any]:
        return self._post_json(
            "/internal/v1/tool-invoke",
            {
                "trace_id": trace_id,
                "session_id": session_id,
                "user_id": user_id,
                "tool_name": tool_name,
                "arguments": arguments,
            },
        )

    def create_proposal(
        self,
        *,
        trace_id: str,
        session_id: str,
        user_id: int,
        action_type: str,
        target_type: str,
        target_id: str,
        payload: dict[str, Any],
        reason: str,
    ) -> dict[str, Any]:
        return self._post_json(
            "/internal/v1/proposals",
            {
                "trace_id": trace_id,
                "session_id": session_id,
                "user_id": user_id,
                "action_type": action_type,
                "target_type": target_type,
                "target_id": target_id,
                "payload": payload,
                "reason": reason,
            },
        )

    def generate_daily_report(self) -> dict[str, Any]:
        return self._post_json("/internal/v1/reports/daily")
