# State Machine, Eval, and Observability

## Approval state machine

Approval lifecycle uses a constrained state machine:

- `pending -> approved -> executed`
- `pending -> rejected`
- `approved -> execution_failed`

Key guarantees:

- `approvals.idempotency_key` suppresses duplicate pending proposals
- `approvals.version` supports optimistic locking during concurrent approval
- `ticket_actions.approval_id` prevents duplicate business execution
- repeated `approve` on an already executed approval returns the same persisted result
- rejected approvals cannot be approved later

Related Go files:

- `internal/app/approval.go`
- `internal/app/repository_approval.go`
- `internal/app/repository_ticket.go`
- `internal/app/schema.go`

## Live evaluation

Supported path for the current system:

```powershell
python -m scripts.run_eval --base-url http://127.0.0.1:18000
```

This evaluates the live Go API directly and is the only supported evaluation path after the Python online stack cleanup.

Reported metrics:

- `tool_selection_accuracy`
- `task_success_rate`
- `dangerous_action_interception_rate`
- `approval_required_precision`
- `chat_p95_latency_ms`

## Load testing

Run a 100 RPS chat load test for 20 seconds:

```powershell
python -m scripts.run_load_test --base-url http://127.0.0.1:18000 --rps 100 --duration 20 --concurrency 50
```

Run a 200 RPS chat load test:

```powershell
python -m scripts.run_load_test --base-url http://127.0.0.1:18000 --rps 200 --duration 20 --concurrency 100
```

The report includes:

- achieved RPS
- error rate
- average latency
- p50 / p95 latency
- status code histogram

Important interpretation rule:

- heuristic / deterministic path benchmark results can support the Go online baseline story
- synchronous remote LLM planner results should be presented separately because they are the current bottleneck

## Metrics and tracing

Start the observability stack:

```powershell
docker compose up -d prometheus grafana jaeger
```

Application side:

```env
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
```

Useful endpoints:

- App metrics: `http://127.0.0.1:18000/metrics`
- Prometheus: `http://127.0.0.1:19090`
- Grafana: `http://127.0.0.1:13000`
- Jaeger: `http://127.0.0.1:16686`

Grafana default credentials:

- username: `admin`
- password: `admin`

Tracked business metrics include:

- `ops_agent_chat_latency_ms`
- `ops_agent_tool_latency_ms`
- `ops_agent_approval_turnaround_seconds`
- `ops_agent_verifier_rejections_total`
- `ops_agent_llm_fallback_total`
- `ops_agent_planner_requests_total`
- `ops_agent_planner_latency_ms`

## Operational anomaly analysis

Representative prompt:

```text
北京区昨天退款率异常和超SLA工单做一下归因分析
```

The tool correlates:

- refund spikes against recent baseline
- SLA breach distribution by root cause and category
- nearby release records in the anomaly window

This is intentionally designed as a semi-automated analysis chain instead of free-form LLM narration only.
