from typing import Any

from pydantic import BaseModel


class ToolSchema(BaseModel):
    name: str
    description: str
    parameters: dict[str, Any]


class ToolExecutionRecord(BaseModel):
    tool_name: str
    tool_type: str
    success: bool
    latency_ms: int
    output: dict[str, Any] | None = None
    error_message: str | None = None


class VerifierResult(BaseModel):
    passed: bool
    severity: str
    message: str
