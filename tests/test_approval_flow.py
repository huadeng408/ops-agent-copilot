import pytest
from sqlalchemy import func, select

from app.db.models import Approval, Ticket, TicketAction, User


@pytest.mark.asyncio
async def test_write_requires_approval(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_write', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    assert response.status_code == 200
    body = response.json()
    assert body['status'] == 'approval_required'
    assert body['approval']['action_type'] == 'assign_ticket'


@pytest.mark.asyncio
async def test_repeated_pending_write_request_reuses_same_approval(client):
    first = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_reuse', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    second = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_reuse', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )

    assert first.status_code == 200
    assert second.status_code == 200
    assert first.json()['approval']['approval_no'] == second.json()['approval']['approval_no']


@pytest.mark.asyncio
async def test_approval_execute_assign_ticket(client, db_session):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_assign', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    approval_no = response.json()['approval']['approval_no']
    approve_response = await client.post(
        f'/api/v1/approvals/{approval_no}/approve',
        json={'approver_user_id': 2},
    )
    assert approve_response.status_code == 200
    body = approve_response.json()
    assert body['status'] == 'executed'
    assert body['execution_result']['success'] is True

    ticket = (await db_session.execute(select(Ticket).where(Ticket.ticket_no == 'T202603280012'))).scalar_one()
    assignee = (await db_session.execute(select(User).where(User.id == ticket.assignee_id))).scalar_one()
    approval = (await db_session.execute(select(Approval).where(Approval.approval_no == approval_no))).scalar_one()

    assert assignee.display_name == '王磊'
    assert approval.status == 'executed'
    assert approval.execution_result is not None
    assert approval.version >= 3


@pytest.mark.asyncio
async def test_repeated_approve_is_idempotent_and_does_not_duplicate_execution(client, db_session):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_idempotent_approve', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    approval_no = response.json()['approval']['approval_no']

    first = await client.post(f'/api/v1/approvals/{approval_no}/approve', json={'approver_user_id': 2})
    second = await client.post(f'/api/v1/approvals/{approval_no}/approve', json={'approver_user_id': 2})

    assert first.status_code == 200
    assert second.status_code == 200
    assert first.json()['status'] == 'executed'
    assert second.json()['status'] == 'executed'
    assert first.json()['execution_result'] == second.json()['execution_result']

    action_count = (
        await db_session.execute(
            select(func.count(TicketAction.id)).join(Approval, TicketAction.approval_id == Approval.id).where(Approval.approval_no == approval_no)
        )
    ).scalar_one()
    assert action_count == 1


@pytest.mark.asyncio
async def test_reject_approval_no_db_change(client, db_session):
    original_ticket = (await db_session.execute(select(Ticket).where(Ticket.ticket_no == 'T202603280012'))).scalar_one()
    original_assignee_id = original_ticket.assignee_id
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_reject', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    approval_no = response.json()['approval']['approval_no']
    reject_response = await client.post(
        f'/api/v1/approvals/{approval_no}/reject',
        json={'approver_user_id': 2, 'reason': '分派目标不正确'},
    )
    assert reject_response.status_code == 200
    refreshed = (await db_session.execute(select(Ticket).where(Ticket.ticket_no == 'T202603280012'))).scalar_one()
    assert refreshed.assignee_id == original_assignee_id
    approval = (await db_session.execute(select(Approval).where(Approval.approval_no == approval_no))).scalar_one()
    assert approval.status == 'rejected'


@pytest.mark.asyncio
async def test_rejected_approval_cannot_be_approved(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_reject_then_approve', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    approval_no = response.json()['approval']['approval_no']

    reject_response = await client.post(
        f'/api/v1/approvals/{approval_no}/reject',
        json={'approver_user_id': 2, 'reason': '分派目标不正确'},
    )
    assert reject_response.status_code == 200

    approve_response = await client.post(
        f'/api/v1/approvals/{approval_no}/approve',
        json={'approver_user_id': 2},
    )
    assert approve_response.status_code == 400
    assert '已被拒绝' in approve_response.json()['detail']
