# 04-数据流与调用链（第19-22节）

本章目标：把“能跑通流程”升级为“能解释系统行为”。重点是时序、事务、一致性与可回放。

---

## 第19节：Chat 请求端到端时序

### 🎯 本节目标
你能够按时间顺序讲清一次 `chat` 请求的每个阶段、每个状态和每个关键数据写入点。

### 📚 核心知识点
- 时序分段：接入 -> 编排 -> 决策 -> 执行 -> 持久化 -> 回包。
- 关键阶段耗时决定用户体验和容量上限。
- 请求链路中最容易丢证据的环节是“异常分支”。

### 🔍 必看代码路径
- [app/api/chat.py](D:/vscode/ops-agent-copilot/app/api/chat.py) `chat`
- [app/services/agent_service.py](D:/vscode/ops-agent-copilot/app/services/agent_service.py) `handle_chat`
- [app/services/tool_registry.py](D:/vscode/ops-agent-copilot/app/services/tool_registry.py) `invoke`
- [app/core/observability.py](D:/vscode/ops-agent-copilot/app/core/observability.py)

### 🧠 面试高频问题
1. 一次请求中的关键耗时点有哪些？
2. 为什么要同时记录业务状态和 HTTP 状态？
3. 你如何证明 trace_id 贯穿全链路？

### ✅ 标准答案与展开
Q1：关键耗时通常在路由决策（含 LLM）、工具查询、数据库提交。  
Q2：HTTP 200 不能代表业务成功，`approval_required` 与 `completed` 是不同语义。  
Q3：通过审计日志、工具日志、响应体三处都带同一 trace_id 来验证贯穿。

### ⚙️ 实战任务
- 输出一次请求的 timeline（至少 6 个阶段）。
- 将阶段耗时写入日志，找出最长阶段并提出优化假设。

### 🚀 进阶思考
如何做“慢请求剖析采样”，避免全量 tracing 成本过高？

---

## 第20节：写操作链路（Proposal -> Approve -> Execute）

### 🎯 本节目标
你能把写链路解释成“受控事务流程”，并清楚每个状态转换的业务意义。

### 📚 核心知识点
- proposal 不是执行，它是“待授权命令”。
- approve 后才会触发真实动作执行。
- reject 与 execution_failed 是两类不同失败，语义必须分开。

### 🔍 必看代码路径
- [app/tools/write_tools.py](D:/vscode/ops-agent-copilot/app/tools/write_tools.py)
- [app/services/approval_service.py](D:/vscode/ops-agent-copilot/app/services/approval_service.py)
- [app/api/approvals.py](D:/vscode/ops-agent-copilot/app/api/approvals.py)

### 🧠 面试高频问题
1. 为什么 `approved` 不是终态？
2. proposal 与 approve 之间如何保证一致性？
3. reject 后是否允许重新发起？

### ✅ 标准答案与展开
Q1：`approved` 表示授权通过，不代表动作已成功执行；执行结果要独立建模。  
Q2：通过审批单主键、幂等键、版本号与状态机约束共同保证。  
Q3：允许，但应生成新提案并保留历史，不能重用已拒绝审批直接改状态。

### ⚙️ 实战任务
- 完成一次 approve 流，并核对 `tickets/ticket_actions/approvals/audit_logs`。
- 完成一次 reject 流，并核对 `rejected_reason` 与审计事件。

### 🚀 进阶思考
如果审批后执行由异步 worker 完成，如何向前端反馈“执行中”状态？

---

## 第21节：审计回放链路（按 trace_id 复盘）

### 🎯 本节目标
你能用 trace_id 从日志中复原一次业务事实，并用它支撑面试中的证据表达。

### 📚 核心知识点
- 回放的核心是事件顺序，而不是单条日志内容。
- 审计数据必须有最小闭环字段：时间、主体、动作、上下文、结果。
- 工具日志和审计日志结合，才能解释“为什么会失败”。

### 🔍 必看代码路径
- [app/api/audit.py](D:/vscode/ops-agent-copilot/app/api/audit.py) `list_audit_logs`
- [app/repositories/audit_repo.py](D:/vscode/ops-agent-copilot/app/repositories/audit_repo.py)
- [app/api/admin.py](D:/vscode/ops-agent-copilot/app/api/admin.py)

### 🧠 面试高频问题
1. trace 回放能解决哪些实际问题？
2. 审计日志和应用日志如何配合？
3. 审计查询慢时该怎么做？

### ✅ 标准答案与展开
Q1：定位故障、责任追溯、合规审计、回归验证四类问题。  
Q2：应用日志用于细节排障，审计日志用于业务事实还原，两者互补。  
Q3：索引优化、按时间分区、冷热分层、异步归档是常见方案。

### ⚙️ 实战任务
- 按 trace_id 回放一次完整流程，写成复盘报告。
- 设计“最近 N 条关键事件”快速查询接口草案。

### 🚀 进阶思考
如何做审计事件脱敏，既满足排障又满足合规？

---

## 第22节：核心数据流一致性检查

### 🎯 本节目标
你能够识别系统一致性风险，设计自动巡检和补偿机制。

### 📚 核心知识点
- 一致性不只靠事务，还依赖状态机和事件写入时序。
- 常见风险：部分成功、重复执行、审计缺失、跨表不一致。
- 巡检是线上稳定性的“最后安全网”。

### 🔍 必看代码路径
- [app/api/chat.py](D:/vscode/ops-agent-copilot/app/api/chat.py) `commit/rollback`
- [app/services/approval_service.py](D:/vscode/ops-agent-copilot/app/services/approval_service.py)
- [app/repositories/ticket_repo.py](D:/vscode/ops-agent-copilot/app/repositories/ticket_repo.py)
- [StudyStepByStep/assets/04-数据流图.md](D:/vscode/ops-agent-copilot/StudyStepByStep/assets/04-数据流图.md)

### 🧠 面试高频问题
1. 哪些场景会出现“状态对不上”？
2. 你如何做一致性巡检？
3. 补偿策略如何避免二次伤害？

### ✅ 标准答案与展开
Q1：网络重试、并发冲突、中途异常、非原子跨系统写入都可能造成不一致。  
Q2：定时任务校验关键关系，如 `executed approvals` 是否存在对应 `ticket_actions`。  
Q3：补偿必须幂等、可回滚、可审计，并提供人工兜底开关。

### ⚙️ 实战任务
- 写一条 SQL 找出“审批执行成功但无动作记录”的异常数据。
- 设计补偿流程：检测 -> 标记 -> 执行 -> 审计 -> 复核。

### 🚀 进阶思考
如果系统拆分为多服务，如何用 Outbox/InBox 降低一致性风险？

