from datetime import UTC, date, datetime, time

from sqlalchemy import asc, desc, func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import aliased

from app.core.exceptions import NotFoundError
from app.db.models import Ticket, TicketAction, TicketComment, User


class TicketRepository:
    def __init__(self, session: AsyncSession) -> None:
        self.session = session

    async def list_sla_breached_tickets(
        self,
        target_date: date,
        region: str | None = None,
        group_by: str | None = None,
    ) -> list[dict]:
        day_end = datetime.combine(target_date, time.max)
        assignee = aliased(User)
        filters = [Ticket.sla_deadline <= day_end, Ticket.status.notin_(['closed', 'resolved'])]
        if region:
            filters.append(Ticket.region == region)

        if group_by:
            field_map = {
                'root_cause': Ticket.root_cause,
                'priority': Ticket.priority,
                'category': Ticket.category,
                'assignee_name': assignee.display_name,
            }
            field = field_map[group_by]
            stmt = (
                select(field.label('group_key'), func.count(Ticket.id).label('ticket_count'))
                .select_from(Ticket)
                .outerjoin(assignee, Ticket.assignee_id == assignee.id)
                .where(*filters)
                .group_by(field)
                .order_by(desc('ticket_count'), asc('group_key'))
            )
            result = await self.session.execute(stmt)
            return [
                {'group_key': row.group_key or '未归类', 'ticket_count': int(row.ticket_count)}
                for row in result.all()
            ]

        stmt = (
            select(Ticket, assignee.display_name)
            .outerjoin(assignee, Ticket.assignee_id == assignee.id)
            .where(*filters)
            .order_by(Ticket.sla_deadline.asc())
            .limit(50)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'ticket_no': ticket.ticket_no,
                'region': ticket.region,
                'category': ticket.category,
                'status': ticket.status,
                'priority': ticket.priority,
                'root_cause': ticket.root_cause,
                'assignee_name': assignee_name,
                'sla_deadline': ticket.sla_deadline.isoformat(),
            }
            for ticket, assignee_name in result.all()
        ]

    async def get_ticket_by_no(self, ticket_no: str) -> Ticket:
        result = await self.session.execute(select(Ticket).where(Ticket.ticket_no == ticket_no))
        ticket = result.scalar_one_or_none()
        if ticket is None:
            raise NotFoundError(f'工单不存在: {ticket_no}')
        return ticket

    async def get_ticket_detail(self, ticket_no: str) -> dict:
        assignee = aliased(User)
        reporter = aliased(User)
        stmt = (
            select(Ticket, assignee.display_name, reporter.display_name)
            .outerjoin(assignee, Ticket.assignee_id == assignee.id)
            .outerjoin(reporter, Ticket.reporter_id == reporter.id)
            .where(Ticket.ticket_no == ticket_no)
        )
        result = await self.session.execute(stmt)
        row = result.one_or_none()
        if row is None:
            raise NotFoundError(f'工单不存在: {ticket_no}')
        ticket, assignee_name, reporter_name = row
        return {
            'ticket_no': ticket.ticket_no,
            'region': ticket.region,
            'category': ticket.category,
            'title': ticket.title,
            'description': ticket.description,
            'status': ticket.status,
            'priority': ticket.priority,
            'root_cause': ticket.root_cause,
            'assignee_name': assignee_name,
            'reporter_name': reporter_name,
            'sla_deadline': ticket.sla_deadline.isoformat(),
            'created_at': ticket.created_at.isoformat(),
            'updated_at': ticket.updated_at.isoformat(),
            'resolved_at': ticket.resolved_at.isoformat() if ticket.resolved_at else None,
        }

    async def get_ticket_comments(self, ticket_no: str, limit: int = 10) -> list[dict]:
        ticket = await self.get_ticket_by_no(ticket_no)
        author = aliased(User)
        stmt = (
            select(TicketComment, author.display_name)
            .outerjoin(author, TicketComment.created_by == author.id)
            .where(TicketComment.ticket_id == ticket.id)
            .order_by(TicketComment.created_at.desc())
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'comment_text': comment.comment_text,
                'created_by': author_name,
                'created_at': comment.created_at.isoformat(),
            }
            for comment, author_name in result.all()
        ]

    async def get_recent_ticket_actions(self, ticket_no: str, limit: int = 10) -> list[dict]:
        ticket = await self.get_ticket_by_no(ticket_no)
        stmt = (
            select(TicketAction)
            .where(TicketAction.ticket_id == ticket.id)
            .order_by(TicketAction.created_at.desc())
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'action_type': row.action_type,
                'old_value': row.old_value,
                'new_value': row.new_value,
                'trace_id': row.trace_id,
                'created_at': row.created_at.isoformat(),
            }
            for row in result.scalars().all()
        ]

    async def assign_ticket(
        self,
        ticket_no: str,
        assignee: User,
        operator_id: int,
        approval_id: int,
        trace_id: str,
    ) -> dict:
        ticket = await self.get_ticket_by_no(ticket_no)
        old_value = {'assignee_id': ticket.assignee_id}
        ticket.assignee_id = assignee.id
        ticket.updated_at = datetime.now(UTC).replace(tzinfo=None)
        self.session.add(
            TicketAction(
                ticket_id=ticket.id,
                action_type='assign_ticket',
                old_value=old_value,
                new_value={'assignee_id': assignee.id, 'assignee_name': assignee.display_name},
                operator_id=operator_id,
                approval_id=approval_id,
                trace_id=trace_id,
            )
        )
        await self.session.flush()
        return {'ticket_no': ticket.ticket_no, 'assignee_name': assignee.display_name}

    async def add_ticket_comment(
        self,
        ticket_no: str,
        comment_text: str,
        operator_id: int,
        approval_id: int,
        trace_id: str,
    ) -> dict:
        ticket = await self.get_ticket_by_no(ticket_no)
        self.session.add(TicketComment(ticket_id=ticket.id, comment_text=comment_text, created_by=operator_id))
        self.session.add(
            TicketAction(
                ticket_id=ticket.id,
                action_type='add_ticket_comment',
                old_value=None,
                new_value={'comment_text': comment_text},
                operator_id=operator_id,
                approval_id=approval_id,
                trace_id=trace_id,
            )
        )
        await self.session.flush()
        return {'ticket_no': ticket.ticket_no, 'comment_text': comment_text}

    async def escalate_ticket(
        self,
        ticket_no: str,
        new_priority: str,
        operator_id: int,
        approval_id: int,
        trace_id: str,
    ) -> dict:
        ticket = await self.get_ticket_by_no(ticket_no)
        old_priority = ticket.priority
        ticket.priority = new_priority
        ticket.updated_at = datetime.now(UTC).replace(tzinfo=None)
        self.session.add(
            TicketAction(
                ticket_id=ticket.id,
                action_type='escalate_ticket',
                old_value={'priority': old_priority},
                new_value={'priority': new_priority},
                operator_id=operator_id,
                approval_id=approval_id,
                trace_id=trace_id,
            )
        )
        await self.session.flush()
        return {'ticket_no': ticket.ticket_no, 'old_priority': old_priority, 'new_priority': new_priority}

    async def get_high_priority_open_tickets(self, limit: int = 10) -> list[dict]:
        stmt = (
            select(Ticket)
            .where(Ticket.priority.in_(['P1', 'P2']), Ticket.status.notin_(['closed', 'resolved']))
            .order_by(asc(Ticket.priority), Ticket.sla_deadline.asc())
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'ticket_no': row.ticket_no,
                'priority': row.priority,
                'region': row.region,
                'category': row.category,
                'status': row.status,
                'root_cause': row.root_cause,
            }
            for row in result.scalars().all()
        ]

    async def list_sla_breach_samples(
        self,
        target_date: date,
        region: str | None = None,
        categories: list[str] | None = None,
        limit: int = 10,
    ) -> list[dict]:
        day_end = datetime.combine(target_date, time.max)
        assignee = aliased(User)
        filters = [Ticket.sla_deadline <= day_end, Ticket.status.notin_(['closed', 'resolved'])]
        if region:
            filters.append(Ticket.region == region)
        if categories:
            filters.append(Ticket.category.in_(categories))

        stmt = (
            select(Ticket, assignee.display_name)
            .outerjoin(assignee, Ticket.assignee_id == assignee.id)
            .where(*filters)
            .order_by(Ticket.priority.asc(), Ticket.sla_deadline.asc())
            .limit(limit)
        )
        result = await self.session.execute(stmt)
        return [
            {
                'ticket_no': ticket.ticket_no,
                'region': ticket.region,
                'category': ticket.category,
                'priority': ticket.priority,
                'status': ticket.status,
                'root_cause': ticket.root_cause,
                'assignee_name': assignee_name,
                'sla_deadline': ticket.sla_deadline.isoformat(),
            }
            for ticket, assignee_name in result.all()
        ]
