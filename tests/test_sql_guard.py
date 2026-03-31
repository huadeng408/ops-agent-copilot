from app.tools.sql_guard import SQLGuard


def test_sql_guard_reject_update():
    result = SQLGuard().validate('UPDATE tickets SET priority = "P1" WHERE id = 1')
    assert result['passed'] is False
    assert 'SELECT' in result['message'] or '禁用' in result['message']


def test_sql_guard_allow_select_view_with_limit():
    result = SQLGuard().validate('SELECT * FROM v_ticket_detail LIMIT 20')
    assert result['passed'] is True
