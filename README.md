# ops-agent-copilot

企业运营 Copilot / 数据分析与工单执行 Agent（FastAPI + SQLAlchemy + 审批流 + 审计）。

本 README 已按真实启动结果重写。  
我在 `2026-03-31` 本地实际跑通了 API、审批流、审计查询与管理页（`SQLite + heuristic` 模式）。

## 项目流程（先看这个）

1. 用户调用 `POST /api/v1/chat` 发送自然语言请求。
2. `MessageRouter` 将请求路由到工具调用（只读分析、工单查询、写操作 proposal）。
3. 只读工具直接执行并返回结果。
4. 写操作工具不会直接落库，而是先创建审批单（`approval_required`）。
5. 审批人调用 `POST /api/v1/approvals/{approval_no}/approve|reject` 决定是否执行。
6. 全流程写入审计日志与工具调用日志，可通过 `GET /api/v1/audit` 和 `/admin` 查看。
7. 指标通过 `GET /metrics` 暴露，可接入 Prometheus/Grafana。

---

## 1. 本地运行

### 方式 A：完整依赖（MySQL/Redis/可观测组件，推荐）

前提：

- 已安装 Python（本机实测 `3.12.4` 可运行）
- Docker Desktop 已启动（必须）

步骤：

```powershell
Copy-Item .env.example .env
.\start.ps1
```

`start.ps1` 默认会做：

- 自动创建/修复 `.venv`
- 自动安装 `requirements.txt`
- `docker compose up -d` 拉起 MySQL / Redis / Adminer / Prometheus / Jaeger / Grafana
- `alembic upgrade head`
- `scripts.init_demo_data`
- 启动 API：`http://127.0.0.1:18000`

常用参数：

```powershell
.\start.ps1 -Port 18001
.\start.ps1 -SkipDocker
.\start.ps1 -SkipSeed
.\start.ps1 -SkipMigrate
.\start.ps1 -NoReload
```

### 方式 B：无 Docker 快速运行（SQLite，已实测）

当 Docker Desktop 未启动时，可以直接用 SQLite 启动 API：

```powershell
Copy-Item .env.example .env

$env:DATABASE_URL = 'sqlite+aiosqlite:///./readme_real.sqlite3'
$env:AGENT_RUNTIME_MODE = 'heuristic'
$env:OTEL_ENABLED = 'false'
$env:NO_PROXY = '127.0.0.1,localhost'

.\.venv\Scripts\python.exe -m pip install -r requirements.txt
.\.venv\Scripts\python.exe -m alembic upgrade head
.\.venv\Scripts\python.exe -m scripts.init_demo_data
.\.venv\Scripts\python.exe -m scripts.run_api --host 127.0.0.1 --port 18000
```

说明：

- 这个模式同样支持 chat、审批流、审计、`/admin`、`/metrics`。
- 若要启用在线 planner，请在 `.env` 或 `auth.json` 中配置有效 API Key。

---

## 2. 访问入口

API 启动后可访问：

- Swagger: <http://127.0.0.1:18000/docs>
- ReDoc: <http://127.0.0.1:18000/redoc>
- Health: <http://127.0.0.1:18000/healthz>
- Admin 管理页: <http://127.0.0.1:18000/admin>
- Prometheus Metrics: <http://127.0.0.1:18000/metrics>

若开启 Docker 观测栈，还可访问：

- Adminer: <http://127.0.0.1:18081>
- Prometheus UI: <http://127.0.0.1:19090>
- Jaeger UI: <http://127.0.0.1:16686>
- Grafana: <http://127.0.0.1:13000>（默认账号密码 `admin/admin`）

---

## 3. 可视化

### 3.1 API 可视化

- 直接在 Swagger 调试：
- `POST /api/v1/chat`
- `GET /api/v1/approvals`
- `POST /api/v1/approvals/{approval_no}/approve`
- `POST /api/v1/approvals/{approval_no}/reject`
- `GET /api/v1/audit`
- `GET /api/v1/tickets/{ticket_no}`

### 3.2 管理页可视化

`/admin` 页面可以：

- 查看并处理审批单（approve/reject）
- 按 `trace_id` / `event_type` 过滤审计日志
- 点击工单查看详情、评论、操作历史

### 3.3 数据库可视化（Adminer）

连接参数（docker compose 默认）：

- System: `MySQL`
- Server: `mysql`
- Username: `root`
- Password: `123456`
- Database: `ops_agent`

建议重点查看表：

- `tickets`
- `ticket_comments`
- `ticket_actions`
- `approvals`
- `audit_logs`
- `tool_call_logs`
- `agent_sessions`
- `agent_messages`

---

## 4. 调试

### VS Code 启动配置

见 [`.vscode/launch.json`](.vscode/launch.json)，可直接使用：

- `API: run local server`
- `Script: init demo data`
- `Script: run eval`
- `Tests: pytest`

### 推荐断点

- `app/services/agent_service.py`
- `app/agents/planner_agent.py`
- `app/services/llm_service.py`
- `app/services/tool_registry.py`
- `app/services/approval_service.py`

### 实用命令

```powershell
.\.venv\Scripts\python.exe -m pytest -q
.\.venv\Scripts\python.exe -m scripts.run_eval
.\.venv\Scripts\python.exe -m scripts.run_eval --base-url http://127.0.0.1:18000
.\.venv\Scripts\python.exe -m scripts.run_load_test --base-url http://127.0.0.1:18000 --rps 50 --duration 20
```

### 排障建议（真实踩坑）

- 如果 `docker version` 报错 pipe/engine 不存在，说明 Docker Desktop 引擎未启动。
- 本机有代理时，访问本地 API 可能出现 `502`；建议设置：
  - `$env:NO_PROXY='127.0.0.1,localhost'`
  - Python `httpx` 场景可加 `trust_env=False`
- `scripts/smoke_test.py` 默认请求 `http://127.0.0.1:8000`，如果你把 API 跑在 `18000`，需要同步调整。

---

## 5. 示例请求（可直接复制）

以下示例均在 `http://127.0.0.1:18000` 实测通过。

### 5.1 退款率异常查询

```powershell
$payload = '{"session_id":"demo_metric","user_id":1,"message":"\u6700\u8fd17\u5929\u9000\u6b3e\u7387\u5f02\u5e38\u7684\u7c7b\u76ee\u6709\u54ea\u4e9b\uff1f"}'
Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18000/api/v1/chat' -ContentType 'application/json; charset=utf-8' -Body $payload
```

### 5.2 超 SLA 分类分析

```powershell
$payload = '{"session_id":"demo_sla","user_id":1,"message":"\u5317\u4eac\u533a\u6628\u5929\u8d85SLA\u7684\u5de5\u5355\u6309\u539f\u56e0\u5206\u7c7b"}'
Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18000/api/v1/chat' -ContentType 'application/json; charset=utf-8' -Body $payload
```

### 5.3 指定工单详情 + 操作记录

```powershell
$payload = '{"session_id":"demo_ticket","user_id":1,"message":"\u67e5\u4e00\u4e0b T202603280012 \u7684\u8be6\u60c5\u548c\u6700\u8fd1\u64cd\u4f5c\u8bb0\u5f55"}'
Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18000/api/v1/chat' -ContentType 'application/json; charset=utf-8' -Body $payload
```

### 5.4 写操作审批流（proposal -> approve）

```powershell
$proposalPayload = '{"session_id":"demo_write","user_id":1,"message":"\u628aT202603280012\u5206\u6d3e\u7ed9\u738b\u78ca"}'
$proposal = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18000/api/v1/chat' -ContentType 'application/json; charset=utf-8' -Body $proposalPayload
$approvalNo = $proposal.approval.approval_no

$approvePayload = '{"approver_user_id":2}'
Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:18000/api/v1/approvals/$approvalNo/approve" -ContentType 'application/json; charset=utf-8' -Body $approvePayload
```

### 5.5 审计日志查询

```powershell
Invoke-RestMethod -Method Get -Uri 'http://127.0.0.1:18000/api/v1/audit?limit=20'
```

---

## 补充文档

- OpenAI 中转接入说明：[`docs/OPENAI_RELAY.md`](docs/OPENAI_RELAY.md)
- 运行/调试/可视化摘要：[`docs/RUN_DEBUG_VISUALIZE.md`](docs/RUN_DEBUG_VISUALIZE.md)

## 国内模型接入

如果你要走国内 OpenAI-compatible 模型路线，当前默认已经改成：

```env
OPENAI_BASE_URL=https://api.moonshot.cn/v1
OPENAI_MODEL=kimi-k2-0905-preview
OPENAI_API_KEY=sk-xxx
OPENAI_AUTH_FILE=auth.json
```

你只需要把下面两个地方换成自己的值：

- [`/.env`](/D:/vscode/ops-agent-copilot/.env)
- [`/auth.json`](/D:/vscode/ops-agent-copilot/auth.json)

如果你的模型服务不支持 `/responses`，项目会自动降级到 `/chat/completions`，保证 `/api/v1/chat` 仍然可用。
