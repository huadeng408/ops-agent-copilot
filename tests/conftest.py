import os
from datetime import datetime, timedelta
from decimal import Decimal
from pathlib import Path

import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import async_sessionmaker

TEST_DB_PATH = Path(__file__).resolve().parent / 'test_app.sqlite3'
os.environ['DATABASE_URL'] = f"sqlite+aiosqlite:///{TEST_DB_PATH.as_posix()}"
os.environ['AGENT_RUNTIME_MODE'] = 'heuristic'
os.environ['OTEL_ENABLED'] = 'false'

from app.core.config import get_settings

get_settings.cache_clear()

from app.db.base import Base
from app.db.models import MetricRefundDaily, Release, Ticket, TicketAction, TicketComment, User
from app.db.session import get_engine, get_session_factory
from app.main import create_app


async def seed_test_data(session) -> None:
    users = [
        User(username='admin', display_name='管理员', role='admin'),
        User(username='approver', display_name='审批人', role='approver'),
        User(username='wanglei', display_name='王磊', role='ops'),
        User(username='lina', display_name='李娜', role='support'),
        User(username='duty', display_name='值班负责人', role='manager'),
    ]
    session.add_all(users)
    await session.flush()

    today = datetime.now().date()
    yesterday = today - timedelta(days=1)
    metrics = []
    for offset in range(6, -1, -1):
        dt = today - timedelta(days=offset)
        metrics.extend(
            [
                MetricRefundDaily(
                    dt=dt,
                    region='北京',
                    category='生鲜',
                    orders_cnt=200,
                    refund_orders_cnt=22 if offset <= 2 else 10,
                    refund_rate=Decimal('0.1100') if offset <= 2 else Decimal('0.0500'),
                    gmv=Decimal('12000.00'),
                ),
                MetricRefundDaily(
                    dt=dt,
                    region='北京',
                    category='餐饮',
                    orders_cnt=220,
                    refund_orders_cnt=8,
                    refund_rate=Decimal('0.0360'),
                    gmv=Decimal('15000.00'),
                ),
            ]
        )
    session.add_all(metrics)
    session.add(
        Release(
            service_name='ops-ticket-service',
            release_version='v1.0.21',
            release_time=datetime.now() - timedelta(days=1, hours=2),
            operator_name='审批人',
            change_summary='修复履约状态同步',
        )
    )
    ticket = Ticket(
        ticket_no='T202603280012',
        region='北京',
        category='生鲜',
        title='北京生鲜履约退款异常',
        description='发布后退款率和超 SLA 工单上升。',
        status='open',
        priority='P2',
        root_cause='系统发布故障',
        assignee_id=users[3].id,
        reporter_id=users[4].id,
        sla_deadline=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=15),
        created_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=10),
        updated_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=11),
        resolved_at=None,
    )
    other_ticket = Ticket(
        ticket_no='T202603280013',
        region='北京',
        category='餐饮',
        title='北京餐饮配送异常',
        description='配送异常导致超 SLA。',
        status='open',
        priority='P1',
        root_cause='配送异常',
        assignee_id=users[2].id,
        reporter_id=users[4].id,
        sla_deadline=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=12),
        created_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=8),
        updated_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=9),
        resolved_at=None,
    )
    session.add_all([ticket, other_ticket])
    await session.flush()
    session.add(TicketComment(ticket_id=ticket.id, comment_text='已联系商家待回执', created_by=users[3].id))
    session.add(
        TicketAction(
            ticket_id=ticket.id,
            action_type='sync_context',
            old_value=None,
            new_value={'note': 'seed action'},
            operator_id=users[0].id,
            approval_id=None,
            trace_id='seed_trace',
        )
    )
    await session.commit()


@pytest_asyncio.fixture
async def test_db():
    engine = get_engine()
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
        await conn.run_sync(Base.metadata.create_all)
    session_factory = get_session_factory()
    async with session_factory() as session:
        await seed_test_data(session)
    yield session_factory
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)


@pytest_asyncio.fixture
async def client(test_db):
    app = create_app()
    async with AsyncClient(transport=ASGITransport(app=app), base_url='http://testserver') as async_client:
        yield async_client


@pytest_asyncio.fixture
async def db_session(test_db):
    async with test_db() as session:
        yield session
