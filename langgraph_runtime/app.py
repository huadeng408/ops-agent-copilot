from __future__ import annotations

from typing import Any

import httpx
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from .backend import BackendServiceError
from .config import RuntimeConfig
from .graph import build_graph


class ChatRequest(BaseModel):
    trace_id: str
    session_id: str
    user_id: int
    message: str
    memory: dict[str, Any] = Field(default_factory=dict)


class ToolCallSummary(BaseModel):
    tool_name: str
    success: bool
    tool_type: str = ""
    latency_ms: int = 0


class ApprovalBrief(BaseModel):
    approval_no: str
    action_type: str
    target_id: str
    payload: dict[str, Any] = Field(default_factory=dict)


class ChatResponse(BaseModel):
    status: str
    answer: str
    planning_source: str = ""
    planner_latency_ms: int = 0
    plan_cache_hit: bool = False
    tool_calls: list[ToolCallSummary] = Field(default_factory=list)
    approval: ApprovalBrief | None = None
    planned_calls: list[dict[str, Any]] = Field(default_factory=list)


config = RuntimeConfig.from_env()
graph = build_graph(config)
app = FastAPI(title="ops-agent-copilot-langgraph", version="1.0.0")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/chat", response_model=ChatResponse)
def chat(request: ChatRequest) -> ChatResponse:
    try:
        state = graph.invoke(
            {
                "trace_id": request.trace_id,
                "session_id": request.session_id,
                "user_id": request.user_id,
                "message": request.message,
                "memory": request.memory,
            }
        )
    except httpx.HTTPStatusError as exc:
        detail = exc.response.text or str(exc)
        raise HTTPException(status_code=exc.response.status_code, detail=detail) from exc
    except httpx.HTTPError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    except BackendServiceError as exc:
        raise HTTPException(status_code=502, detail=exc.detail) from exc
    except Exception as exc:  # pragma: no cover - defensive catch for runtime issues
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    return ChatResponse(
        status=str(state.get("status") or "completed"),
        answer=str(state.get("answer") or "已处理请求。"),
        planning_source=str(state.get("planning_source") or ""),
        planner_latency_ms=int(state.get("planner_latency_ms") or 0),
        plan_cache_hit=bool(state.get("plan_cache_hit")),
        tool_calls=[ToolCallSummary(**item) for item in state.get("tool_calls") or []],
        approval=ApprovalBrief(**state["approval"]) if state.get("approval") else None,
        planned_calls=list(state.get("planned_calls") or []),
    )
