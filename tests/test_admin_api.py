import pytest


@pytest.mark.asyncio
async def test_list_approvals_endpoint(client):
    create_response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_admin_approvals', 'user_id': 1, 'message': '把 T202603280012 分派给王磊'},
    )
    assert create_response.status_code == 200

    response = await client.get('/api/v1/approvals', params={'status': 'pending', 'limit': 10})
    assert response.status_code == 200
    body = response.json()
    assert body['items']
    assert any(item['status'] == 'pending' for item in body['items'])


@pytest.mark.asyncio
async def test_admin_page_loads(client):
    response = await client.get('/admin')
    assert response.status_code == 200
    assert 'ops-agent-copilot' in response.text
    assert '工单详情' in response.text
    assert 'event-type-select' in response.text


@pytest.mark.asyncio
async def test_recent_audit_endpoint_supports_event_type_filter(client):
    await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_admin_audit', 'user_id': 1, 'message': '查一下 T202603280012 的详情'},
    )

    response = await client.get('/api/v1/audit', params={'limit': 20, 'event_type': 'tool_called'})
    assert response.status_code == 200
    body = response.json()
    assert body['event_type'] == 'tool_called'
    assert body['available_event_types']
    assert body['logs']
    assert all(item['event_type'] == 'tool_called' for item in body['logs'])


@pytest.mark.asyncio
async def test_ticket_detail_endpoint_returns_comments_and_actions(client):
    response = await client.get('/api/v1/tickets/T202603280012')
    assert response.status_code == 200
    body = response.json()
    assert body['ticket']['ticket_no'] == 'T202603280012'
    assert body['comments']
    assert body['actions']
