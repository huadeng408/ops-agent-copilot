from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_approval_service, get_user
from app.core.exceptions import AppError
from app.db.models import Approval
from app.db.session import get_db_session
from app.repositories.approval_repo import ApprovalRepository
from app.schemas.approval import (
    ApprovalApproveRequest,
    ApprovalDetail,
    ApprovalListItem,
    ApprovalListResponse,
    ApprovalRejectRequest,
    ApprovalResponse,
)
from app.services.approval_service import ApprovalService


router = APIRouter(tags=['approvals'])


@router.get('/approvals', response_model=ApprovalListResponse)
async def list_approvals(
    status: str | None = Query(default=None),
    limit: int = Query(default=20, ge=1, le=100),
    session: AsyncSession = Depends(get_db_session),
) -> ApprovalListResponse:
    repo = ApprovalRepository(session)
    approvals = await repo.list_recent(status=status, limit=limit)
    return ApprovalListResponse(items=[ApprovalListItem.model_validate(item, from_attributes=True) for item in approvals])


@router.post('/approvals/{approval_no}/approve', response_model=ApprovalResponse)
async def approve_approval(
    approval_no: str,
    payload: ApprovalApproveRequest,
    session: AsyncSession = Depends(get_db_session),
    approval_service: ApprovalService = Depends(get_approval_service),
) -> ApprovalResponse:
    try:
        approver = await get_user(payload.approver_user_id, session)
        approval, result = await approval_service.approve(approval_no, approver)
        await session.commit()
        return ApprovalResponse(
            approval_no=approval.approval_no,
            idempotency_key=approval.idempotency_key,
            status=approval.status,
            version=approval.version,
            execution_result={'success': True, **result},
            execution_error=approval.execution_error,
        )
    except AppError as exc:
        await session.rollback()
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc


@router.post('/approvals/{approval_no}/reject', response_model=ApprovalResponse)
async def reject_approval(
    approval_no: str,
    payload: ApprovalRejectRequest,
    session: AsyncSession = Depends(get_db_session),
    approval_service: ApprovalService = Depends(get_approval_service),
) -> ApprovalResponse:
    try:
        approver = await get_user(payload.approver_user_id, session)
        approval = await approval_service.reject(approval_no, approver, payload.reason)
        await session.commit()
        return ApprovalResponse(
            approval_no=approval.approval_no,
            idempotency_key=approval.idempotency_key,
            status=approval.status,
            version=approval.version,
            rejected_reason=approval.rejected_reason,
        )
    except AppError as exc:
        await session.rollback()
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(exc)) from exc


@router.get('/approvals/{approval_no}', response_model=ApprovalDetail)
async def get_approval_detail(
    approval_no: str,
    session: AsyncSession = Depends(get_db_session),
) -> ApprovalDetail:
    result = await session.execute(select(Approval).where(Approval.approval_no == approval_no))
    approval = result.scalar_one_or_none()
    if approval is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail='approval not found')
    return ApprovalDetail.model_validate(approval, from_attributes=True)
