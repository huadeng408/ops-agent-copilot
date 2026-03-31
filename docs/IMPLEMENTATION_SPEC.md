# Implementation Spec

## Summary

`ops-agent-copilot` implements a single-agent operations copilot built on FastAPI, SQLAlchemy, MySQL/SQLite-compatible models, approval-gated write actions, and audit/tool-call logging.

## Request Flow

1. `POST /api/v1/chat` receives user input and creates a `trace_id`.
2. Session and messages are persisted to `agent_sessions` and `agent_messages`.
3. A planner routes the request to readonly tools or write `propose_*` tools.
4. `ToolRegistry` executes tools through a unified audit/logging layer.
5. Write proposals are verified and stored in `approvals` with `pending` status.
6. `POST /api/v1/approvals/{approval_no}/approve` executes the approved business action.
7. Execution writes `ticket_actions`, `audit_logs`, and `tool_call_logs`.

## Safety Controls

- `SQLGuard` only allows `SELECT` against whitelisted views with `LIMIT <= 200`.
- Write actions can only be performed by parameterized business executors.
- All dangerous actions go through proposal + approval.
- `VerifierService` validates proposal completeness and target existence.

## Current MVP Coverage

- `/healthz`
- `/api/v1/chat`
- `/api/v1/approvals/{approval_no}/approve`
- `/api/v1/approvals/{approval_no}/reject`
- `/api/v1/approvals/{approval_no}`
- `/api/v1/audit`
- Readonly tools: metrics, SLA tickets, ticket detail/comments, recent releases, guarded SQL
- Write proposal tools: assign/comment/escalate
- Demo data seeding and offline eval script
