from app.core.exceptions import PermissionDeniedError
from app.db.models import User


APPROVER_ROLES = {"admin", "approver"}
WRITE_ROLES = {"admin", "approver", "ops", "support", "manager"}


def ensure_can_submit_write(user: User) -> None:
    if user.role not in WRITE_ROLES:
        raise PermissionDeniedError("当前用户没有提交写操作申请的权限")


def ensure_can_approve(user: User) -> None:
    if user.role not in APPROVER_ROLES:
        raise PermissionDeniedError("当前用户没有审批权限")
