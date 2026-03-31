from datetime import date, datetime
from decimal import Decimal

from sqlalchemy import Boolean, Date, DateTime, ForeignKey, Index, Integer, JSON, Numeric, String, Text, UniqueConstraint, func
from sqlalchemy.orm import Mapped, mapped_column, relationship
from sqlalchemy.types import BigInteger

from app.db.base import Base


ID_TYPE = BigInteger().with_variant(Integer, 'sqlite')


class User(Base):
    __tablename__ = 'users'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    username: Mapped[str] = mapped_column(String(64), nullable=False, unique=True)
    display_name: Mapped[str] = mapped_column(String(64), nullable=False)
    role: Mapped[str] = mapped_column(String(32), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())


class AgentSession(Base):
    __tablename__ = 'agent_sessions'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    session_id: Mapped[str] = mapped_column(String(64), nullable=False, unique=True)
    user_id: Mapped[int] = mapped_column(ForeignKey('users.id'), nullable=False)
    summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now(), onupdate=func.now())

    user: Mapped[User] = relationship()

    __table_args__ = (Index('idx_agent_sessions_user_id', 'user_id'),)


class AgentMessage(Base):
    __tablename__ = 'agent_messages'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    session_id: Mapped[str] = mapped_column(String(64), nullable=False)
    role: Mapped[str] = mapped_column(String(16), nullable=False)
    content: Mapped[dict] = mapped_column(JSON, nullable=False)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        Index('idx_agent_messages_session_id', 'session_id'),
        Index('idx_agent_messages_trace_id', 'trace_id'),
    )


class MetricRefundDaily(Base):
    __tablename__ = 'metric_refund_daily'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    dt: Mapped[date] = mapped_column(Date, nullable=False)
    region: Mapped[str] = mapped_column(String(32), nullable=False)
    category: Mapped[str] = mapped_column(String(64), nullable=False)
    orders_cnt: Mapped[int] = mapped_column(Integer, nullable=False)
    refund_orders_cnt: Mapped[int] = mapped_column(Integer, nullable=False)
    refund_rate: Mapped[Decimal] = mapped_column(Numeric(8, 4), nullable=False)
    gmv: Mapped[Decimal] = mapped_column(Numeric(18, 2), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        UniqueConstraint('dt', 'region', 'category', name='uk_metric_refund_daily'),
        Index('idx_metric_refund_daily_region_dt', 'region', 'dt'),
        Index('idx_metric_refund_daily_category_dt', 'category', 'dt'),
    )


class Release(Base):
    __tablename__ = 'releases'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    service_name: Mapped[str] = mapped_column(String(64), nullable=False)
    release_version: Mapped[str] = mapped_column(String(64), nullable=False)
    release_time: Mapped[datetime] = mapped_column(DateTime, nullable=False)
    operator_name: Mapped[str] = mapped_column(String(64), nullable=False)
    change_summary: Mapped[str] = mapped_column(String(255), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (Index('idx_releases_service_time', 'service_name', 'release_time'),)


class Ticket(Base):
    __tablename__ = 'tickets'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    ticket_no: Mapped[str] = mapped_column(String(32), nullable=False, unique=True)
    region: Mapped[str] = mapped_column(String(32), nullable=False)
    category: Mapped[str] = mapped_column(String(64), nullable=False)
    title: Mapped[str] = mapped_column(String(255), nullable=False)
    description: Mapped[str] = mapped_column(Text, nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False)
    priority: Mapped[str] = mapped_column(String(16), nullable=False)
    root_cause: Mapped[str | None] = mapped_column(String(64), nullable=True)
    assignee_id: Mapped[int | None] = mapped_column(ForeignKey('users.id'), nullable=True)
    reporter_id: Mapped[int | None] = mapped_column(ForeignKey('users.id'), nullable=True)
    sla_deadline: Mapped[datetime] = mapped_column(DateTime, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now(), onupdate=func.now())
    resolved_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)

    assignee: Mapped[User | None] = relationship(foreign_keys=[assignee_id])
    reporter: Mapped[User | None] = relationship(foreign_keys=[reporter_id])

    __table_args__ = (
        Index('idx_tickets_region_status', 'region', 'status'),
        Index('idx_tickets_priority', 'priority'),
        Index('idx_tickets_sla_deadline', 'sla_deadline'),
    )


class TicketComment(Base):
    __tablename__ = 'ticket_comments'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    ticket_id: Mapped[int] = mapped_column(ForeignKey('tickets.id'), nullable=False)
    comment_text: Mapped[str] = mapped_column(Text, nullable=False)
    created_by: Mapped[int] = mapped_column(ForeignKey('users.id'), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (Index('idx_ticket_comments_ticket_id', 'ticket_id'),)


class TicketAction(Base):
    __tablename__ = 'ticket_actions'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    ticket_id: Mapped[int] = mapped_column(ForeignKey('tickets.id'), nullable=False)
    action_type: Mapped[str] = mapped_column(String(32), nullable=False)
    old_value: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    new_value: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    operator_id: Mapped[int] = mapped_column(ForeignKey('users.id'), nullable=False)
    approval_id: Mapped[int | None] = mapped_column(ForeignKey('approvals.id'), nullable=True)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        Index('idx_ticket_actions_ticket_id', 'ticket_id'),
        Index('idx_ticket_actions_trace_id', 'trace_id'),
        UniqueConstraint('approval_id', name='uk_ticket_actions_approval_id'),
    )


class Approval(Base):
    __tablename__ = 'approvals'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    approval_no: Mapped[str] = mapped_column(String(32), nullable=False, unique=True)
    idempotency_key: Mapped[str] = mapped_column(String(96), nullable=False, unique=True)
    session_id: Mapped[str] = mapped_column(String(64), nullable=False)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    action_type: Mapped[str] = mapped_column(String(32), nullable=False)
    target_type: Mapped[str] = mapped_column(String(32), nullable=False)
    target_id: Mapped[str] = mapped_column(String(64), nullable=False)
    payload: Mapped[dict] = mapped_column(JSON, nullable=False)
    reason: Mapped[str] = mapped_column(Text, nullable=False)
    status: Mapped[str] = mapped_column(String(16), nullable=False)
    requested_by: Mapped[int] = mapped_column(ForeignKey('users.id'), nullable=False)
    approved_by: Mapped[int | None] = mapped_column(ForeignKey('users.id'), nullable=True)
    approved_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    executed_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    execution_result: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    execution_error: Mapped[str | None] = mapped_column(String(255), nullable=True)
    version: Mapped[int] = mapped_column(Integer, nullable=False, default=1, server_default='1')
    rejected_reason: Mapped[str | None] = mapped_column(String(255), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        Index('idx_approvals_idempotency_key', 'idempotency_key'),
        Index('idx_approvals_session_id', 'session_id'),
        Index('idx_approvals_trace_id', 'trace_id'),
        Index('idx_approvals_status', 'status'),
    )

    __mapper_args__ = {'version_id_col': version}


class AuditLog(Base):
    __tablename__ = 'audit_logs'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    session_id: Mapped[str | None] = mapped_column(String(64), nullable=True)
    user_id: Mapped[int | None] = mapped_column(ForeignKey('users.id'), nullable=True)
    event_type: Mapped[str] = mapped_column(String(64), nullable=False)
    event_data: Mapped[dict] = mapped_column(JSON, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        Index('idx_audit_logs_trace_id', 'trace_id'),
        Index('idx_audit_logs_event_type', 'event_type'),
        Index('idx_audit_logs_created_at', 'created_at'),
    )


class ToolCallLog(Base):
    __tablename__ = 'tool_call_logs'

    id: Mapped[int] = mapped_column(ID_TYPE, primary_key=True, autoincrement=True)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    session_id: Mapped[str] = mapped_column(String(64), nullable=False)
    tool_name: Mapped[str] = mapped_column(String(64), nullable=False)
    tool_type: Mapped[str] = mapped_column(String(16), nullable=False)
    input_payload: Mapped[dict] = mapped_column(JSON, nullable=False)
    output_payload: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    success: Mapped[bool] = mapped_column(Boolean, nullable=False)
    error_message: Mapped[str | None] = mapped_column(String(255), nullable=True)
    latency_ms: Mapped[int] = mapped_column(Integer, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, nullable=False, server_default=func.now())

    __table_args__ = (
        Index('idx_tool_call_logs_trace_id', 'trace_id'),
        Index('idx_tool_call_logs_session_id', 'session_id'),
        Index('idx_tool_call_logs_tool_name', 'tool_name'),
    )
