# 01-基础认知（第01-04节）

本章目标：建立“系统级认知”。你不仅要知道模块名，还要理解业务边界、运行机制、数据模型和 API 契约。

---

## 第01节：项目定位与业务边界

### 🎯 本节目标
学完后你可以在 3 分钟内完整回答：
- 这个项目解决什么业务问题；
- 为什么必须有审批流；
- 与普通 ChatBot 后端相比，它的“工程硬点”是什么。

### 📚 核心知识点（结合源码）
1. **系统定位不是问答机器人，而是“可执行运营助手”**  
核心价值在于：把分析能力和工单操作能力放在同一条治理链路上。

2. **双轨能力模型**  
- 只读轨：查询、分析、报表，可直接执行。  
- 写操作轨：先 proposal，再审批，再执行，全部可审计。

3. **关键业务角色**
- 发起人（提交自然语言指令）
- 审批人（对高风险操作做最终授权）
- 系统（执行动作并沉淀审计证据）

### 🔍 必看代码路径（精确到文件/函数）
- [app/main.py](D:/vscode/ops-agent-copilot/app/main.py) `create_app`
- [app/api/chat.py](D:/vscode/ops-agent-copilot/app/api/chat.py) `chat`
- [app/services/approval_service.py](D:/vscode/ops-agent-copilot/app/services/approval_service.py) `create_proposal`
- [app/api/approvals.py](D:/vscode/ops-agent-copilot/app/api/approvals.py) `approve_approval` / `reject_approval`

### 🧠 面试高频问题（至少3个）
1. 为什么写操作必须通过审批流，不能直接执行？
2. 这个系统的核心业务不变量是什么？
3. 如果去掉审计模块，对业务和工程分别有什么影响？

### ✅ 标准答案与展开
**Q1：为什么写操作必须通过审批流，不能直接执行？**  
标准回答：因为这是“高风险动作”，审批流是把不可逆风险前置治理。  
展开：运营系统的写操作（分派、升级、备注）会影响责任归属和 SLA 结果。若直接执行，错误指令会立刻落地且难以追责。审批流把风险拆成两个阶段：提案（可审阅）和执行（可授权），再配合状态机和审计日志，能同时满足安全、追责和可恢复。

**Q2：核心业务不变量是什么？**  
标准回答：未经审批的高风险写操作不得执行，且每次执行必须可追溯。  
展开：这条不变量在代码层通过 proposal 机制、审批接口权限校验、状态迁移校验和审计落库共同保证。面试中要强调“一个不变量由多层机制共同守护”，这比单点校验更稳。

**Q3：去掉审计模块会怎样？**  
标准回答：短期系统还能跑，但长期会失去生产可治理性。  
展开：没有审计，线上故障时无法按 trace 回放，审批争议时无法还原上下文，安全合规无法满足。工程角度，问题定位平均时长会显著上升，回归验证也缺证据链。

### ⚙️ 实战任务（必须动手）
1. 通过 `/docs` 走一遍：  
`chat(只读)` -> `chat(写操作 proposal)` -> `approvals/approve` -> `audit` 查询。
2. 输出一份 1 页文档：  
- 业务角色  
- 风险动作  
- 不变量  
- 核心证据（接口/表/日志）

### 🧪 验收标准
- 能画出“提案到执行”的流程图，不看代码也能复述。
- 能解释清楚审批流的业务价值，不仅是“安全”两个字。

### 🚀 进阶思考（用于拉开差距）
如果是 Go 微服务版，你会把审批流独立成单服务，还是保留在单体内？请从“一致性成本、部署复杂度、团队规模”三维度给出你的判断。

---

## 第02节：启动链路与运行时配置

### 🎯 本节目标
学完后你能从“配置 -> 依赖 -> 启动 -> 服务可用”四个层次定位启动问题。

### 📚 核心知识点（结合源码）
1. **配置优先级与来源**
- `.env` 基础配置
- 环境变量覆盖
- `auth.json` 作为 API Key 补充来源

2. **启动链路**
- 解析配置
- 初始化 DB engine/session
- 注册路由
- 应用生命周期（logging/telemetry）

3. **双运行模式的价值**
- Docker 全依赖模式：贴近生产。
- SQLite 快速模式：开发/演示/面试环境下稳定复现。

### 🔍 必看代码路径（精确到文件/函数）
- [scripts/run_api.py](D:/vscode/ops-agent-copilot/scripts/run_api.py) `main`
- [start.ps1](D:/vscode/ops-agent-copilot/start.ps1)
- [app/core/config.py](D:/vscode/ops-agent-copilot/app/core/config.py) `Settings` / `get_settings`
- [app/db/session.py](D:/vscode/ops-agent-copilot/app/db/session.py) `get_engine`

### 🧠 面试高频问题（至少3个）
1. 配置优先级怎么设计才不容易出事故？
2. 为什么 `OPENAI_AUTH_FILE` 只做补充，而非唯一来源？
3. 启动失败时你如何快速定位是“配置问题、依赖问题还是代码问题”？

### ✅ 标准答案与展开
**Q1：配置优先级如何设计？**  
标准回答：显式覆盖优先，默认值兜底，敏感配置外置。  
展开：一般是“环境变量 > .env > 默认值”。这样线上可以在不改镜像的情况下修复配置。敏感信息不应硬编码，必须外置，并可审计变更来源。

**Q2：为何 auth file 不是唯一来源？**  
标准回答：保证部署灵活性。  
展开：在容器、CI/CD、云平台中，环境变量更常见。`auth.json` 更适合本地开发和中转场景。双通道能兼容多环境，同时避免强绑定单一机制。

**Q3：启动失败如何定位？**  
标准回答：按层排查，先配置，再外部依赖，再应用。  
展开：先验证配置加载结果，再检查 DB/Redis/LLM 连通性，最后看应用启动日志与栈。这个顺序能最快缩小范围，避免“盲改代码”。

### ⚙️ 实战任务（必须动手）
1. 用两种模式各启动一次（Docker 与 SQLite）。
2. 制造 2 个启动故障：  
- 错误 `DATABASE_URL`  
- 错误 `OPENAI_BASE_URL`
3. 写一份“启动排障 runbook（最少 8 步）”。

### 🧪 验收标准
- 10 分钟内能定位并修复任意一类启动故障。
- 能说明“为什么先查配置，再查网络，再查代码”。

### 🚀 进阶思考（用于拉开差距）
Go 项目中你会如何实现“配置变更白名单 + 启动前配置自检（schema validation）”？

---

## 第03节：数据模型与表结构认知

### 🎯 本节目标
学完后你能说清表关系、索引意图，以及它们如何支撑可追溯执行。

### 📚 核心知识点（结合源码）
1. **核心实体**
- `approvals`：审批状态与执行结果
- `ticket_actions`：操作历史
- `audit_logs`：业务事件
- `tool_call_logs`：工具调用轨迹
- `agent_messages`：会话上下文

2. **关键索引**
- `approvals.idempotency_key`
- `approvals.trace_id`
- `audit_logs.trace_id/event_type`

3. **视图价值**
- 把分析场景常用查询结构化，提高复用性与可读性。

### 🔍 必看代码路径（精确到文件/函数）
- [app/db/models.py](D:/vscode/ops-agent-copilot/app/db/models.py)
- [app/db/migrations/versions/0001_init.py](D:/vscode/ops-agent-copilot/app/db/migrations/versions/0001_init.py) `upgrade`
- [app/db/migrations/versions/0002_approval_state_machine.py](D:/vscode/ops-agent-copilot/app/db/migrations/versions/0002_approval_state_machine.py) `upgrade`
- [scripts/init_demo_data.py](D:/vscode/ops-agent-copilot/scripts/init_demo_data.py) `reset_schema`

### 🧠 面试高频问题（至少3个）
1. 为什么 `approvals` 需要 `idempotency_key`？
2. 为什么既有 `tickets` 还要 `ticket_actions`？
3. 你如何判断一个索引是否真的有效？

### ✅ 标准答案与展开
**Q1：为什么要 `idempotency_key`？**  
标准回答：防止重复提案和重复执行。  
展开：在重试、网络抖动、并发请求下，同一业务意图可能多次到达。幂等键能把“业务重复”压缩成“状态复用”，避免出现多个审批单或重复落库。

**Q2：为什么保留 `ticket_actions`？**  
标准回答：主表记录当前态，动作表记录演进过程。  
展开：只看 `tickets` 只能看到“现在是什么”，看不到“怎么变成这样”。动作历史是审计与复盘基础，也是事故归因证据。

**Q3：索引有效性怎么判断？**  
标准回答：看查询计划 + 命中率 + 写放大成本。  
展开：索引不是越多越好。要看高频查询是否走索引、扫描行数是否显著下降、对写性能的影响是否可接受。

### ⚙️ 实战任务（必须动手）
1. 在 DB 中查一条已执行审批，沿外键追踪关联数据。
2. 比较有无索引时某查询的执行时间（可用小规模实验）。
3. 输出“核心表字段字典（含字段业务意义）”。

### 🧪 验收标准
- 能在白板上画出关键表关系图。
- 能解释每张核心表“为谁服务，解决什么问题”。

### 🚀 进阶思考（用于拉开差距）
如果要做多租户，你会把 `tenant_id` 放进哪些表，并如何改索引顺序？

---

## 第04节：API 契约与错误处理

### 🎯 本节目标
学完后你能输出一份“可落地”的 API 契约与错误规范，不再停留在口头描述。

### 📚 核心知识点（结合源码）
1. **路由层职责**
- 参数解析、依赖注入、异常映射。

2. **错误分层**
- `ValidationAppError`：输入/业务规则不满足
- `ConflictError`：状态冲突（如重复审批）
- 未知异常：500

3. **响应契约稳定性**
- 统一结构、固定字段语义、可向后兼容。

### 🔍 必看代码路径（精确到文件/函数）
- [app/api/chat.py](D:/vscode/ops-agent-copilot/app/api/chat.py) `chat`
- [app/api/approvals.py](D:/vscode/ops-agent-copilot/app/api/approvals.py)
- [app/core/exceptions.py](D:/vscode/ops-agent-copilot/app/core/exceptions.py)
- [app/schemas/chat.py](D:/vscode/ops-agent-copilot/app/schemas/chat.py)

### 🧠 面试高频问题（至少3个）
1. 为什么 API 层需要显式做异常映射？
2. 400、409、500 的边界分别是什么？
3. 你如何做 API 兼容演进？

### ✅ 标准答案与展开
**Q1：为什么要异常映射？**  
标准回答：把内部错误语义稳定映射成外部协议语义。  
展开：如果不做映射，前端只能拿到杂乱报错文本，无法做可靠分支处理。稳定错误码是系统协作契约。

**Q2：400/409/500 边界？**  
标准回答：  
- 400：请求本身不合法（参数或业务校验失败）  
- 409：资源状态冲突（例如已执行不可再拒绝）  
- 500：系统内部故障  
展开：边界清晰是可运维和可观测前提，尤其 409 能显著减少误报“系统故障”。

**Q3：API 兼容如何演进？**  
标准回答：新增字段不破坏旧字段语义；删除/重命名走版本化。  
展开：接口演进遵循“非破坏优先”，并通过 contract test 和文档变更记录控制风险。

### ⚙️ 实战任务（必须动手）
1. 设计一份错误码表（10 条以内即可）。
2. 给 `chat` 增加 `trace_id` 透传演示（响应字段已含，可补说明文档）。
3. 写一个最小 contract test（验证字段存在和类型）。

### 🧪 验收标准
- 你能对任意报错快速判断是 4xx 还是 5xx。
- 你能说明“这次改动会不会破坏前端”。

### 🚀 进阶思考（用于拉开差距）
如果在 Go 中采用 `errors.Is/As + 错误码中间件`，你会如何组织错误模型避免业务层直接依赖 HTTP？

