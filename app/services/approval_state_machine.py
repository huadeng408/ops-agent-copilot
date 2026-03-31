from app.core.exceptions import ConflictError
from app.db.models import Approval


APPROVAL_STATUS_PENDING = 'pending'
APPROVAL_STATUS_APPROVED = 'approved'
APPROVAL_STATUS_REJECTED = 'rejected'
APPROVAL_STATUS_EXECUTED = 'executed'
APPROVAL_STATUS_EXECUTION_FAILED = 'execution_failed'

APPROVAL_FINAL_STATUSES = {
    APPROVAL_STATUS_REJECTED,
    APPROVAL_STATUS_EXECUTED,
}

APPROVAL_ALLOWED_TRANSITIONS: dict[str, set[str]] = {
    APPROVAL_STATUS_PENDING: {APPROVAL_STATUS_APPROVED, APPROVAL_STATUS_REJECTED},
    APPROVAL_STATUS_APPROVED: {APPROVAL_STATUS_EXECUTED, APPROVAL_STATUS_EXECUTION_FAILED},
    APPROVAL_STATUS_REJECTED: set(),
    APPROVAL_STATUS_EXECUTED: set(),
    APPROVAL_STATUS_EXECUTION_FAILED: set(),
}


def ensure_transition(approval: Approval, next_status: str) -> None:
    allowed = APPROVAL_ALLOWED_TRANSITIONS.get(approval.status, set())
    if next_status not in allowed:
        raise ConflictError(f'审批单状态不允许从 {approval.status} 迁移到 {next_status}')
