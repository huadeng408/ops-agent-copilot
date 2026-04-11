# Run, Debug, Visualize

## One-command startup

If Docker Desktop is already running:

```powershell
.\start.ps1 -SkipLLMCheck
```

Real behavior of the current startup flow:

- checks `go`
- starts `mysql`, `redis`, `prometheus`, `grafana`, `jaeger`, `adminer`
- optionally checks remote LLM connectivity
- builds the Go server to `.tmp/ops-agent-go.exe`
- seeds demo data through `go run ./cmd/seed`
- starts the Go API on `http://127.0.0.1:18000`

Useful options:

```powershell
.\start.ps1 -Port 18001
.\start.ps1 -SkipDocker
.\start.ps1 -SkipSeed
.\start.ps1 -SkipLLMCheck
.\start.ps1 -SkipBuild
```

Notes:

- `-NoReload` is now a compatibility no-op because the online service is no longer Python reload mode.
- `-SkipMigrate` is now a compatibility no-op because schema bootstrap is handled by Go startup and seeding.
- `-SkipInstall` is now a compatibility no-op for the online path. Python is only needed for offline eval and load test scripts.

## Manual startup

```powershell
docker compose up -d
go run ./cmd/seed
$env:HOST='127.0.0.1'
$env:PORT='18000'
$env:AGENT_RUNTIME_MODE='heuristic'
$env:OTEL_ENABLED='true'
$env:OTEL_EXPORTER_OTLP_ENDPOINT='http://127.0.0.1:4318'
go run ./cmd/server
```

## Stable interview demo

Run the closed-loop interview demo:

```powershell
.\scripts\interview_demo.ps1
```

The script covers:

- readonly chat query
- proposal creation for a write action
- approval execution
- audit lookup
- ticket detail verification

## Debug locally

Use VS Code launch configs or attach to the Go process.

Good breakpoint locations:

- `cmd/server/main.go`
- `internal/app/http.go`
- `internal/app/agent.go`
- `internal/app/service_support.go`
- `internal/app/tool_registry.go`
- `internal/app/approval.go`
- `internal/app/telemetry.go`

If you want to debug the planner decision:

- set `AGENT_RUNTIME_MODE=heuristic` to inspect deterministic routing
- set `AGENT_RUNTIME_MODE=auto` plus a valid Kimi or Ollama config to inspect planner fallback from LLM to heuristic routing

## Visualize data

Current entry points:

- Admin page: `http://127.0.0.1:18000/admin`
- Docs page: `http://127.0.0.1:18000/docs`
- Health: `http://127.0.0.1:18000/healthz`
- Metrics: `http://127.0.0.1:18000/metrics`
- Prometheus: `http://127.0.0.1:19090`
- Grafana: `http://127.0.0.1:13000`
- Jaeger: `http://127.0.0.1:16686`
- Adminer: `http://127.0.0.1:18081`

Adminer connection:

- system: `MySQL`
- server: `mysql`
- user: `root`
- password: `123456`
- database: `ops_agent`

Useful tables:

- `tickets`
- `ticket_comments`
- `ticket_actions`
- `approvals`
- `audit_logs`
- `tool_call_logs`

## What to show in an interview

If the interviewer asks for a quick run-through, prioritize these pages:

1. `/docs` to show the online API surface is real.
2. `/admin` to show approvals and audit logs are queryable.
3. `/metrics` to show Prometheus metrics exist.
4. Jaeger to show traces exist when `OTEL_ENABLED=true`.
