from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.exceptions import NotFoundError
from app.db.session import get_db_session
from app.repositories.ticket_repo import TicketRepository


router = APIRouter(tags=['tickets'])


@router.get('/tickets/{ticket_no}')
async def get_ticket_detail(
    ticket_no: str,
    comments_limit: int = Query(default=10, ge=1, le=50),
    actions_limit: int = Query(default=10, ge=1, le=50),
    session: AsyncSession = Depends(get_db_session),
) -> dict:
    repo = TicketRepository(session)
    try:
        detail = await repo.get_ticket_detail(ticket_no)
        comments = await repo.get_ticket_comments(ticket_no, limit=comments_limit)
        actions = await repo.get_recent_ticket_actions(ticket_no, limit=actions_limit)
    except NotFoundError as exc:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail=str(exc)) from exc

    return {
        'ticket': detail,
        'comments': comments,
        'actions': actions,
    }
