revision = '0001_init'
down_revision = None
branch_labels = None
depends_on = None

from alembic import op

from app.db.base import Base
from app.db import models  # noqa: F401


VIEWS = [
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


DROP_VIEWS = [
    'DROP VIEW IF EXISTS v_recent_releases',
    'DROP VIEW IF EXISTS v_ticket_detail',
    'DROP VIEW IF EXISTS v_ticket_sla',
    'DROP VIEW IF EXISTS v_refund_metrics_daily',
]


def upgrade() -> None:
    bind = op.get_bind()
    Base.metadata.create_all(bind)
    for view_sql in VIEWS:
        op.execute(view_sql)


def downgrade() -> None:
    bind = op.get_bind()
    for sql in DROP_VIEWS:
        op.execute(sql)
    Base.metadata.drop_all(bind)
