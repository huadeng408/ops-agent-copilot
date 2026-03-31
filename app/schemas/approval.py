from datetime import datetime
from typing import Any

from pydantic import BaseModel, Field


class ApprovalApproveRequest(BaseModel):
    approver_user_id: int


class ApprovalRejectRequest(BaseModel):
    approver_user_id: int
    reason: str = Field(min_length=1, max_length=255)


class ApprovalResponse(BaseModel):
    approval_no: str
    idempotency_key: str | None = None
    status: str
    version: int | None = None
    execution_result: dict[str, Any] | None = None
    execution_error: str | None = None
    rejected_reason: str | None = None


class ApprovalDetail(BaseModel):
    approval_no: str
    idempotency_key: str
    session_id: str
    trace_id: str
    action_type: str
    target_type: str
    target_id: str
    payload: dict[str, Any]
    reason: str
    status: str
    version: int
    requested_by: int
    approved_by: int | None = None
    approved_at: datetime | None = None
    executed_at: datetime | None = None
    execution_result: dict[str, Any] | None = None
    execution_error: str | None = None
    rejected_reason: str | None = None
    created_at: datetime | None = None


class ApprovalListItem(BaseModel):
    approval_no: str
    idempotency_key: str
    session_id: str
    trace_id: str
    action_type: str
    target_id: str
    status: str
    version: int
    requested_by: int
    approved_by: int | None = None
    execution_error: str | None = None
    rejected_reason: str | None = None
    created_at: datetime
    approved_at: datetime | None = None
    executed_at: datetime | None = None


class ApprovalListResponse(BaseModel):
    items: list[ApprovalListItem]
