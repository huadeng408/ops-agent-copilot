# ops-agent-copilot

企业运营 Copilot / 工单执行 Agent。  
支持自然语言查询运营指标、工单超 SLA 归因、日报生成，以及工单分派 / 备注 / 升级等写操作，并通过审批流、状态机、SQL Guard、Verifier、审计日志将大模型能力约束在可控边界内。

## 项目亮点

- **单 Agent + 确定性工作流**：LLM 负责理解和规划，业务系统负责执行、审批、审计，避免多 Agent 失控。
- **写操作必须审批**：所有分派、备注、升级动作先生成 proposal，再进入人工审批，审批通过后才真正落库。
- **强约束状态机**：支持 `pending -> approved/rejected -> executed` 流转，并处理重复审批、并发审批、重复执行。
- **SQL 安全治理**：只允许白名单视图、只允许 `SELECT`、限制 `LIMIT`，阻断危险 SQL。
- **全链路审计与可观测**：内置 Prometheus、Grafana、Jaeger，记录 chat、tool call、approval、write execution、LLM fallback。
- **真实压测数据**：既跑了本地快路径基线，也跑了 docker-compose + MySQL + Redis + Kimi 模型路径的真实压测。

## 核心能力

### 1. 运营指标查询

- 最近 7 天退款率异常类目
- 区域 / 类目退款率趋势
- 高退款率类目 TopN

### 2. 工单分析

- 超 SLA 工单列表
- 按根因 / 优先级 / 类目分组分析
- 工单详情与最近操作记录

### 3. 报表生成

- 运营日报
- 退款异常与高优工单摘要
- 异常归因提示

### 4. 工单执行

- 分派工单
- 添加备注
- 升级优先级

### 5. 治理与风控

- 审批前置
- SQL Guard
- Verifier
- 审计日志
- 工具调用日志
- 幂等执行

## 系统架构

```text
User
  -> POST /api/v1/chat
  -> AgentService
      -> session + trace_id + memory
      -> planner (router / LLM)
      -> ToolRegistry
          -> readonly tools
          -> write proposal tools
      -> Verifier
      -> approval_required / completed

Approver
  -> POST /api/v1/approvals/{approval_no}/approve|reject
  -> ApprovalStateMachine
  -> execute write action
  -> ticket_actions / audit_logs / tool_call_logs
```

## 技术栈

- **Backend**: Python 3.11, FastAPI, SQLAlchemy 2.x Async, Pydantic v2
- **Storage**: MySQL 8, Redis
- **LLM / Agent**: OpenAI-compatible API, tool calling, planner + router hybrid
- **Infra / Observability**: Docker Compose, Prometheus, Grafana, Jaeger, OpenTelemetry
- **Engineering**: Alembic, pytest, httpx

## 项目结构

```text
ops-agent-copilot/
├─ app/
│  ├─ api/               # chat / approvals / audit / admin / metrics
│  ├─ agents/            # prompts / planner / router / assistant factory
│  ├─ core/              # config / logging / observability / exceptions
│  ├─ db/                # models / session / alembic migrations
│  ├─ repositories/      # metric / ticket / approval / audit / session
│  ├─ schemas/           # request / response schemas
│  ├─ services/          # agent / approval / verifier / audit / llm / report
│  ├─ tools/             # readonly tools / write tools / sql guard
│  └─ jobs/              # daily report job
├─ scripts/              # init_demo_data / run_api / run_eval / run_load_test
├─ eval/                 # dataset / rubric / benchmark artifacts
├─ tests/                # chat / tools / approval / sql guard / llm tests
├─ docs/                 # spec / perf / relay / visualize / interview notes
├─ ops/                  # prometheus / grafana provisioning
├─ docker-compose.yml
├─ start.ps1
└─ README.md
```

## 快速开始

### 1. 填写模型配置

项目当前默认走国内 OpenAI-compatible 路线：

```env
OPENAI_BASE_URL=https://api.moonshot.cn/v1
OPENAI_MODEL=kimi-k2-0905-preview
OPENAI_API_KEY=sk-xxx
OPENAI_AUTH_FILE=auth.json
```

需要填写的文件：

- `.env`
- `auth.json`

### 2. 一键启动

```powershell
Copy-Item .env.example .env
.\start.ps1
```

`start.ps1` 默认会执行：

- 创建 / 修复 `.venv`
- 安装依赖
- 启动 MySQL / Redis / Prometheus / Grafana / Jaeger
- 执行 Alembic 迁移
- 初始化 demo 数据
- 检查 LLM 连通性
- 启动 API 服务

常用参数：

```powershell
.\start.ps1 -Port 18001
.\start.ps1 -SkipDocker
.\start.ps1 -SkipSeed
.\start.ps1 -SkipMigrate
.\start.ps1 -SkipLLMCheck
.\start.ps1 -NoReload
```

## 访问入口

- Swagger: `http://127.0.0.1:18000/docs`
- Admin Page: `http://127.0.0.1:18000/admin`
- Metrics: `http://127.0.0.1:18000/metrics`
- Prometheus: `http://127.0.0.1:19090`
- Grafana: `http://127.0.0.1:13000`
- Jaeger: `http://127.0.0.1:16686`

## API 示例

### 查询超 SLA 工单

```json
POST /api/v1/chat
{
  "session_id": "demo_sla",
  "user_id": 1,
  "message": "北京区昨天超 SLA 的工单按原因分类"
}
```

### 触发写操作审批

```json
POST /api/v1/chat
{
  "session_id": "demo_write",
  "user_id": 1,
  "message": "把 T202603280012 分派给王磊"
}
```

如果命中写操作，会返回：

```json
{
  "status": "approval_required",
  "approval": {
    "approval_no": "APR202603290001"
  }
}
```

之后由审批接口执行：

```json
POST /api/v1/approvals/{approval_no}/approve
{
  "approver_user_id": 2
}
```

## 压测结果

### 本地快路径基线

- `heuristic + sqlite`，100 RPS，30s：成功率 `99.30%`，`p95 2.27s`
- `heuristic + sqlite`，200 RPS，30s：成功率 `99.42%`，`p95 3.36s`

### 真实 Kimi 模型路径

- `docker-compose + MySQL + Redis + Prometheus + Grafana + kimi-k2-0905-preview`
- 100 RPS，10s：成功率 `97.4%`，`p95 17.59s`
- 200 RPS，10s：成功率 `96.7%`，`p95 34.49s`

结论：

- 系统治理链路稳定，失败率较低。
- 主要瓶颈不是数据库，而是同步远程 LLM 规划带来的高延迟和低吞吐。
- 真实生产场景应进一步引入异步任务、缓存、限流和快慢路径分流。

## 文档索引

- [实现规格](docs/IMPLEMENTATION_SPEC.md)
- [运行 / 调试 / 可视化](docs/RUN_DEBUG_VISUALIZE.md)
- [模型接入说明](docs/LLM_PROVIDER_SETUP.md)
- [OpenAI-compatible 接入说明](docs/OPENAI_RELAY.md)
- [性能报告](docs/PERF_REPORT_20260331.md)
- [状态机与可观测性说明](docs/STATE_MACHINE_OBSERVABILITY.md)
- [面试讲稿](docs/INTERVIEW_TALK.md)

## 简历表述参考

> 构建企业运营 Copilot / 工单执行 Agent，支持自然语言查询运营指标、超 SLA 归因、日报生成与写操作审批执行，打通请求理解、工具编排、审批、执行与审计的完整链路；设计 SQL Guard、Verifier、审批状态机与全链路可观测体系，在 docker-compose 真实环境下完成 100/200 RPS 压测并定位同步 LLM 规划瓶颈。
