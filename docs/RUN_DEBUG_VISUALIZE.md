# Run, Debug, Visualize

## One-command startup

If Docker Desktop is already running:

```powershell
.\start.ps1
```

Default behavior:

- auto-create or repair `.venv` when needed
- auto-install `requirements.txt` dependencies when needed
- start `mysql`, `redis`, `adminer`
- run Alembic migrations
- seed demo data
- start FastAPI on `http://127.0.0.1:18000`

Useful options:

```powershell
.\start.ps1 -Port 18001
.\start.ps1 -SkipSeed
.\start.ps1 -SkipDocker
.\\start.ps1 -SkipInstall
.\start.ps1 -NoReload
```

## Virtual environment

Activation is optional because `start.ps1` uses `.venv\Scripts\python.exe` directly.

If you still want to activate it manually in PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\.venv\Scripts\Activate.ps1
```

## Manual startup

```bash
pip install -r requirements.txt
docker compose up -d
python -m alembic upgrade head
python -m scripts.init_demo_data
python -m scripts.run_api --host 127.0.0.1 --port 18000 --reload
```

## Debug locally

Use VS Code launch configs in `.vscode/launch.json`.

Best places for breakpoints:

- `app/services/agent_service.py`
- `app/agents/planner_agent.py`
- `app/services/llm_service.py`
- `app/services/tool_registry.py`

## Visualize data

- Admin page: `http://127.0.0.1:18000/admin`
- Swagger UI: `http://127.0.0.1:18000/docs`
- ReDoc: `http://127.0.0.1:18000/redoc`
- Adminer: `http://127.0.0.1:18081`

Adminer connection:

- system: `MySQL`
- server: `mysql`
- user: `root`
- password: `123456`
- database: `ops_agent`

Useful tables:

- `tickets`
- `ticket_actions`
- `approvals`
- `audit_logs`
- `tool_call_logs`
