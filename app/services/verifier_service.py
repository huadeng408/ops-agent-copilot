from app.core.exceptions import ValidationAppError
from app.core.observability import metrics
from app.core.security import ensure_can_submit_write
from app.db.models import User
from app.repositories.ticket_repo import TicketRepository
from app.schemas.tool import VerifierResult
from app.tools.sql_guard import SQLGuard


class VerifierService:
    def __init__(self, ticket_repo: TicketRepository) -> None:
        self.ticket_repo = ticket_repo

    def verify_sql(self, sql: str) -> VerifierResult:
        result = VerifierResult(**SQLGuard().validate(sql))
        if not result.passed:
            metrics.record_verifier_rejection(stage='sql')
        return result

    def verify_result_size(self, rows: list[dict]) -> VerifierResult:
        if len(rows) == 0:
            metrics.record_verifier_rejection(stage='result_size')
            return VerifierResult(passed=False, severity='warn', message='结果为空，请缩小范围或补充条件')
        if len(rows) > 200:
            metrics.record_verifier_rejection(stage='result_size')
            return VerifierResult(passed=False, severity='error', message='结果过大，请缩小时间范围或增加过滤条件')
        return VerifierResult(passed=True, severity='info', message='结果规模正常')

    async def verify_proposal(self, action_type: str, payload: dict, reason: str, current_user: User) -> VerifierResult:
        ensure_can_submit_write(current_user)
        if not reason.strip():
            metrics.record_verifier_rejection(stage='proposal')
            raise ValidationAppError('proposal reason 不能为空')

        ticket_no = payload.get('ticket_no')
        if not ticket_no:
            metrics.record_verifier_rejection(stage='proposal')
            raise ValidationAppError('proposal payload 缺少 ticket_no')

        await self.ticket_repo.get_ticket_by_no(ticket_no)

        if action_type == 'assign_ticket' and not payload.get('assignee_name'):
            metrics.record_verifier_rejection(stage='proposal')
            raise ValidationAppError('分派 proposal 缺少 assignee_name')
        if action_type == 'escalate_ticket' and payload.get('new_priority') not in {'P1', 'P2', 'P3'}:
            metrics.record_verifier_rejection(stage='proposal')
            raise ValidationAppError('优先级不合法')
        if action_type == 'add_ticket_comment' and not payload.get('comment_text'):
            metrics.record_verifier_rejection(stage='proposal')
            raise ValidationAppError('备注内容不能为空')

        return VerifierResult(passed=True, severity='info', message='proposal 校验通过')
