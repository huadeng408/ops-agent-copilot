import pytest

from app.repositories.ticket_repo import TicketRepository


@pytest.mark.asyncio
async def test_ticket_detail_repository(db_session):
    repo = TicketRepository(db_session)
    detail = await repo.get_ticket_detail('T202603280012')
    assert detail['ticket_no'] == 'T202603280012'
    assert detail['region'] == '北京'
