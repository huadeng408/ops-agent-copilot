from app.schemas.tool import ToolSchema
from app.tools.base import BaseTool, ToolContext, ToolResult


class ProposalTool(BaseTool):
    tool_type = 'write'


class ProposeAssignTicketTool(ProposalTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='propose_assign_ticket',
            description='提出工单分派建议，不直接执行',
            parameters={
                'type': 'object',
                'properties': {
                    'ticket_no': {'type': 'string'},
                    'assignee_name': {'type': 'string'},
                    'reason': {'type': 'string'},
                },
                'required': ['ticket_no', 'assignee_name', 'reason'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        return ToolResult(
            data={
                'action_type': 'assign_ticket',
                'target_type': 'ticket',
                'target_id': arguments['ticket_no'],
                'payload': arguments,
                'reason': arguments['reason'],
            },
            message='已生成分派 proposal',
            requires_approval=True,
        )


class ProposeAddTicketCommentTool(ProposalTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='propose_add_ticket_comment',
            description='提出工单备注写入建议，不直接执行',
            parameters={
                'type': 'object',
                'properties': {
                    'ticket_no': {'type': 'string'},
                    'comment_text': {'type': 'string'},
                    'reason': {'type': 'string'},
                },
                'required': ['ticket_no', 'comment_text', 'reason'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        return ToolResult(
            data={
                'action_type': 'add_ticket_comment',
                'target_type': 'ticket',
                'target_id': arguments['ticket_no'],
                'payload': arguments,
                'reason': arguments['reason'],
            },
            message='已生成备注 proposal',
            requires_approval=True,
        )


class ProposeEscalateTicketTool(ProposalTool):
    @property
    def schema(self) -> ToolSchema:
        return ToolSchema(
            name='propose_escalate_ticket',
            description='提出升级工单优先级建议，不直接执行',
            parameters={
                'type': 'object',
                'properties': {
                    'ticket_no': {'type': 'string'},
                    'new_priority': {'type': 'string', 'enum': ['P1', 'P2', 'P3']},
                    'reason': {'type': 'string'},
                },
                'required': ['ticket_no', 'new_priority', 'reason'],
            },
        )

    async def execute(self, context: ToolContext, arguments: dict) -> ToolResult:
        return ToolResult(
            data={
                'action_type': 'escalate_ticket',
                'target_type': 'ticket',
                'target_id': arguments['ticket_no'],
                'payload': arguments,
                'reason': arguments['reason'],
            },
            message='已生成升级 proposal',
            requires_approval=True,
        )
