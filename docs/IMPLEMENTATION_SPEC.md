# Implementation Spec

## Summary

`ops-agent-copilot` is now a Go-dominant online service for an operations Copilot / Agent backend.

Current online responsibilities handled by Go:

- `POST /api/v1/chat` request orchestration
- planner routing
- readonly tool execution
- write proposal creation
- approval execution
- audit and tool-call logging
- cache access
- Prometheus metrics
- OTLP / Jaeger tracing

Python remains in the repository only for offline evaluation and load-testing scripts.

## Request flow

1. `POST /api/v1/chat` receives user input and injects a `trace_id`.
2. Session state and recent messages are loaded through the Go repositories and memory service.
3. `PlannerService` decides whether to use the heuristic router or the optional Kimi / Ollama planner path.
4. `ToolRegistry` executes readonly tools directly or converts write intent into `propose_*` calls.
5. Proposal tools create approval records with `pending` status instead of mutating business state.
6. `POST /api/v1/approvals/{approval_no}/approve` re-validates the payload and executes the deterministic business action.
7. Execution writes `ticket_actions`, `audit_logs`, and `tool_call_logs`.
8. `GET /api/v1/audit` and `GET /api/v1/tickets/{ticket_no}` can reconstruct the full business trace.

## Safety controls

- `SQLGuard` only allows readonly `SELECT` against whitelisted views with a bounded `LIMIT`.
- `VerifierService` validates proposal completeness, ticket existence, assignee validity, and write payload correctness.
- All dangerous actions are forced through `proposal -> approval -> execution`.
- The online service does not grant the model direct write access to the database.

## Approval guarantees

- `pending -> approved -> executed`
- `pending -> rejected`
- duplicate proposal suppression through `idempotency_key`
- approval concurrency control through optimistic locking on `version`
- duplicate business execution prevention through unique execution constraints

## Current online coverage

- `/healthz`
- `/docs`
- `/admin`
- `/metrics`
- `/api/v1/chat`
- `/api/v1/approvals`
- `/api/v1/approvals/{approval_no}`
- `/api/v1/approvals/{approval_no}/approve`
- `/api/v1/approvals/{approval_no}/reject`
- `/api/v1/audit`
- `/api/v1/tickets/{ticket_no}`

## Core capability set

Readonly tools:

- refund metrics query
- refund anomaly discovery
- SLA-breached ticket query
- ticket detail and comment lookup
- recent release lookup
- guarded readonly SQL
- daily report generation
- operational anomaly correlation

Write proposal tools:

- assign ticket
- add ticket comment
- escalate ticket priority
