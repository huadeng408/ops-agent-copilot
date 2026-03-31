import asyncio
from datetime import datetime, timedelta
from decimal import Decimal
import random

from sqlalchemy import delete, text

from app.db.base import Base
from app.db.models import (
    AgentMessage,
    AgentSession,
    Approval,
    AuditLog,
    MetricRefundDaily,
    Release,
    Ticket,
    TicketAction,
    TicketComment,
    ToolCallLog,
    User,
)
from app.db.session import get_engine, get_session_factory


VIEW_SQLS = [
    """
    CREATE VIEW v_refund_metrics_daily AS
    SELECT dt, region, category, orders_cnt, refund_orders_cnt, refund_rate, gmv
    FROM metric_refund_daily
    """,
    """
    CREATE VIEW v_ticket_sla AS
    SELECT
      t.ticket_no,
      t.region,
      t.category,
      t.status,
      t.priority,
      t.root_cause,
      u.display_name AS assignee_name,
      t.sla_deadline,
      t.created_at,
      t.updated_at,
      CASE
        WHEN t.status NOT IN ('closed', 'resolved') AND CURRENT_TIMESTAMP > t.sla_deadline THEN 1
        ELSE 0
      END AS is_sla_breached
    FROM tickets t
    LEFT JOIN users u ON t.assignee_id = u.id
    """,
    """
    CREATE VIEW v_ticket_detail AS
    SELECT
      t.ticket_no,
      t.region,
      t.category,
      t.title,
      t.description,
      t.status,
      t.priority,
      t.root_cause,
      u.display_name AS assignee_name,
      t.sla_deadline,
      t.created_at,
      t.updated_at,
      t.resolved_at
    FROM tickets t
    LEFT JOIN users u ON t.assignee_id = u.id
    """,
    """
    CREATE VIEW v_recent_releases AS
    SELECT service_name, release_version, release_time, operator_name, change_summary
    FROM releases
    """,
]

ALEMBIC_HEAD = '0002_approval_state_machine'


async def reset_schema() -> None:
    engine = get_engine()
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        await conn.execute(
            text(
                """
                CREATE TABLE IF NOT EXISTS alembic_version (
                  version_num VARCHAR(32) NOT NULL
                )
                """
            )
        )
        await conn.execute(text('DELETE FROM alembic_version'))
        await conn.execute(text('INSERT INTO alembic_version (version_num) VALUES (:version_num)'), {'version_num': ALEMBIC_HEAD})
        for sql in [
            'DROP VIEW IF EXISTS v_recent_releases',
            'DROP VIEW IF EXISTS v_ticket_detail',
            'DROP VIEW IF EXISTS v_ticket_sla',
            'DROP VIEW IF EXISTS v_refund_metrics_daily',
        ]:
            await conn.execute(text(sql))
        for sql in VIEW_SQLS:
            await conn.execute(text(sql))


async def seed_users(session) -> list[User]:
    users = [
        User(username='admin', display_name='管理员', role='admin'),
        User(username='approver', display_name='审批人', role='approver'),
        User(username='wanglei', display_name='王磊', role='ops'),
        User(username='zhaomin', display_name='赵敏', role='ops'),
        User(username='lina', display_name='李娜', role='support'),
        User(username='chenjie', display_name='陈杰', role='support'),
        User(username='sunyu', display_name='孙宇', role='ops'),
        User(username='duty_manager', display_name='值班负责人', role='manager'),
    ]
    session.add_all(users)
    await session.flush()
    return users


async def seed_metrics(session) -> None:
    rng = random.Random(7)
    today = datetime.now().date()
    regions = ['北京', '上海', '广州']
    categories = ['生鲜', '餐饮', '酒店', '到店综合']
    rows: list[MetricRefundDaily] = []
    anomaly_day = today - timedelta(days=2)
    for offset in range(59, -1, -1):
        dt = today - timedelta(days=offset)
        for region in regions:
            for category in categories:
                orders_cnt = 180 + rng.randint(0, 120)
                base_rate = 0.018 + (regions.index(region) * 0.004) + (categories.index(category) * 0.003)
                if dt == anomaly_day and region == '北京' and category == '生鲜':
                    base_rate = 0.112
                if dt == today and region == '北京' and category == '生鲜':
                    base_rate = 0.085
                refund_rate = round(base_rate + rng.uniform(0.0, 0.01), 4)
                refund_orders_cnt = max(1, int(orders_cnt * refund_rate))
                gmv = Decimal(str(round(orders_cnt * (40 + rng.uniform(10, 80)), 2)))
                rows.append(
                    MetricRefundDaily(
                        dt=dt,
                        region=region,
                        category=category,
                        orders_cnt=orders_cnt,
                        refund_orders_cnt=refund_orders_cnt,
                        refund_rate=Decimal(str(refund_rate)),
                        gmv=gmv,
                    )
                )
    session.add_all(rows)
    await session.flush()


async def seed_releases(session) -> list[Release]:
    today = datetime.now()
    releases = []
    for i in range(10):
        release_time = today - timedelta(days=i * 2, hours=2)
        summary = '修复履约状态同步' if i == 0 else f'常规发布批次 {i + 1}'
        releases.append(
            Release(
                service_name='ops-ticket-service' if i % 2 == 0 else 'refund-analytics',
                release_version=f'v1.0.{20 + i}',
                release_time=release_time,
                operator_name='审批人',
                change_summary=summary,
            )
        )
    session.add_all(releases)
    await session.flush()
    return releases


async def seed_tickets(session, users: list[User]) -> None:
    rng = random.Random(13)
    today = datetime.now()
    yesterday = (today - timedelta(days=1)).date()
    root_causes = ['商家超时', '配送异常', '系统发布故障', '风控误拦截']
    statuses = ['open', 'processing', 'resolved', 'closed']
    categories = ['生鲜', '餐饮', '酒店', '到店综合']
    regions = ['北京', '上海', '广州']
    assignees = users[2:7]
    reporters = users[2:7]

    tickets: list[Ticket] = []
    for i in range(80):
        created_at = today - timedelta(days=rng.randint(0, 14), hours=rng.randint(0, 20))
        region = regions[i % len(regions)]
        category = categories[i % len(categories)]
        root_cause = root_causes[i % len(root_causes)]
        status = statuses[i % len(statuses)]
        priority = ['P1', 'P2', 'P3'][i % 3]
        ticket_no = f"T{created_at.strftime('%Y%m%d')}{i + 1:04d}"
        sla_deadline = created_at + timedelta(hours=8)
        if i < 16:
            region = '北京'
            status = 'open'
            priority = 'P1' if i % 2 == 0 else 'P2'
            root_cause = '系统发布故障' if i < 8 else root_causes[i % len(root_causes)]
            created_at = datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=9 + (i % 8))
            sla_deadline = created_at + timedelta(hours=4)
            ticket_no = f'T{yesterday.strftime("%Y%m%d")}{i + 1:04d}'
        ticket = Ticket(
            ticket_no=ticket_no,
            region=region,
            category=category,
            title=f'{region}{category}运营工单 {i + 1}',
            description=f'{root_cause} 导致的运营异常，需要跟进处理。',
            status=status,
            priority=priority,
            root_cause=root_cause,
            assignee_id=assignees[i % len(assignees)].id,
            reporter_id=reporters[(i + 1) % len(reporters)].id,
            sla_deadline=sla_deadline,
            created_at=created_at,
            updated_at=created_at + timedelta(hours=1),
            resolved_at=None if status in {'open', 'processing'} else created_at + timedelta(hours=5),
        )
        tickets.append(ticket)

    target_ticket = Ticket(
        ticket_no='T202603280012',
        region='北京',
        category='生鲜',
        title='北京生鲜履约退款异常',
        description='3 月 28 日上午发布后退款率和超 SLA 工单明显上升。',
        status='open',
        priority='P2',
        root_cause='系统发布故障',
        assignee_id=users[4].id,
        reporter_id=users[7].id,
        sla_deadline=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=15),
        created_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=10, minutes=30),
        updated_at=datetime.combine(yesterday, datetime.min.time()) + timedelta(hours=12),
        resolved_at=None,
    )
    tickets[11] = target_ticket
    session.add_all(tickets)
    await session.flush()

    target = next(ticket for ticket in tickets if ticket.ticket_no == 'T202603280012')
    comments = [
        TicketComment(ticket_id=target.id, comment_text='已联系商家待回执', created_by=users[4].id),
        TicketComment(ticket_id=target.id, comment_text='确认与上午发布版本时间接近', created_by=users[7].id),
    ]
    actions = [
        TicketAction(
            ticket_id=target.id,
            action_type='sync_context',
            old_value=None,
            new_value={'note': '系统自动补充上下文'},
            operator_id=users[0].id,
            approval_id=None,
            trace_id='seed_trace_ticket_001',
        )
    ]
    session.add_all(comments + actions)
    await session.flush()


async def clear_data(session) -> None:
    for model in [ToolCallLog, AuditLog, TicketAction, TicketComment, Approval, AgentMessage, AgentSession, Ticket, Release, MetricRefundDaily, User]:
        await session.execute(delete(model))


async def main() -> None:
    await reset_schema()
    session_factory = get_session_factory()
    async with session_factory() as session:
        await clear_data(session)
        users = await seed_users(session)
        await seed_metrics(session)
        await seed_releases(session)
        await seed_tickets(session, users)
        await session.commit()
    print('Demo data initialized successfully.')


if __name__ == '__main__':
    asyncio.run(main())

