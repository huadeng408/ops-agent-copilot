from typing import Any

from pydantic import BaseModel, Field


class ChatRequest(BaseModel):
    session_id: str = Field(min_length=3, max_length=64)
    user_id: int
    message: str = Field(min_length=1, max_length=4000)


class ToolCallSummary(BaseModel):
    tool_name: str
    success: bool
    tool_type: str | None = None
    latency_ms: int | None = None


class ApprovalBrief(BaseModel):
    approval_no: str
    action_type: str
    target_id: str
    payload: dict[str, Any]


class ChatResponse(BaseModel):
    trace_id: str
    session_id: str
    status: str
    answer: str
    tool_calls: list[ToolCallSummary] = Field(default_factory=list)
    approval: ApprovalBrief | None = None
