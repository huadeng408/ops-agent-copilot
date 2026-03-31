import pytest


@pytest.mark.asyncio
async def test_chat_metric_query(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_metric', 'user_id': 1, 'message': '最近7天北京区退款率最高的类目是什么？'},
    )
    assert response.status_code == 200
    body = response.json()
    assert body['status'] == 'completed'
    assert any(item['tool_name'] == 'query_refund_metrics' for item in body['tool_calls'])
    assert '退款率' in body['answer']


@pytest.mark.asyncio
async def test_chat_ticket_analysis(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_ticket', 'user_id': 1, 'message': '北京区昨天超SLA的工单按原因分类'},
    )
    assert response.status_code == 200
    body = response.json()
    assert body['status'] == 'completed'
    assert any(item['tool_name'] == 'list_sla_breached_tickets' for item in body['tool_calls'])
    assert '超 SLA 工单分类结果' in body['answer']


@pytest.mark.asyncio
async def test_chat_operational_anomaly_analysis(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_anomaly', 'user_id': 1, 'message': '北京区昨天退款率异常和超SLA工单做一下归因分析'},
    )
    assert response.status_code == 200
    body = response.json()
    assert body['status'] == 'completed'
    assert any(item['tool_name'] == 'analyze_operational_anomaly' for item in body['tool_calls'])
    assert '异常归因分析' in body['answer']
    assert '半自动归因结论' in body['answer']


@pytest.mark.asyncio
async def test_audit_log_created(client):
    response = await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_audit', 'user_id': 1, 'message': '查一下T202603280012的详情'},
    )
    assert response.status_code == 200
    trace_id = response.json()['trace_id']
    audit_response = await client.get('/api/v1/audit', params={'trace_id': trace_id})
    assert audit_response.status_code == 200
    events = [item['event_type'] for item in audit_response.json()['logs']]
    assert 'chat_received' in events
    assert 'response_returned' in events


@pytest.mark.asyncio
async def test_metrics_endpoint_exposes_operational_metrics(client):
    await client.post(
        '/api/v1/chat',
        json={'session_id': 'sess_metrics', 'user_id': 1, 'message': '北京区昨天超SLA的工单按原因分类'},
    )
    response = await client.get('/metrics')
    assert response.status_code == 200
    assert 'ops_agent_chat_requests_total' in response.text
    assert 'ops_agent_tool_latency_ms' in response.text
