# State Machine, Eval, and Observability

## Approval state machine

Approval lifecycle now uses a constrained state machine:

- `pending -> approved -> executed`
- `pending -> rejected`
- `approved -> execution_failed`

Key guarantees:

- `approvals.idempotency_key` is unique and reused for duplicate pending proposals
- `approvals.version` enables optimistic locking
- `ticket_actions.approval_id` is unique, preventing duplicate write execution
- repeated `approve` on an already executed approval returns the same persisted `execution_result`
- rejected approvals cannot be approved later

Related files:

- `app/services/approval_service.py`
- `app/services/approval_state_machine.py`
- `app/db/models.py`
- `app/db/migrations/versions/0002_approval_state_machine.py`

## Offline and live evaluation

Offline evaluation:

```powershell
python -m scripts.run_eval
```

Live API evaluation:

```powershell
python -m scripts.run_eval --base-url http://127.0.0.1:18000
```

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

## Metrics and tracing

Start the observability stack:

```powershell
docker compose up -d prometheus grafana jaeger
```

Application side:

```env
METRICS_ENABLED=true
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

Tracked business metrics:

- `ops_agent_chat_latency_ms`
- `ops_agent_tool_latency_ms`
- `ops_agent_approval_turnaround_seconds`
- `ops_agent_verifier_rejections_total`
- `ops_agent_llm_fallback_total`

## Operational anomaly analysis

New tool:

- `analyze_operational_anomaly`

Example prompt:

```text
北京区昨天退款率异常和超SLA工单做一下归因分析
```

The tool correlates:

- refund spikes vs. 7-day baseline
- SLA breach distribution by root cause and category
- nearby releases in the anomaly time window

This produces a semi-automated correlation summary instead of only raw query output.
